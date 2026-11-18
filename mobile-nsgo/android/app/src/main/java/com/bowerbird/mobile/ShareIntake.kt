package com.bowerbird.mobile

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.OpenableColumns
import android.webkit.MimeTypeMap
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.UUID
import java.util.concurrent.Executors

/**
 * Receives ACTION_SEND / ACTION_SEND_MULTIPLE off the UI thread, copies streams
 * into the app's private storage and persists one JSON event per delivery.
 * The Go app calls share.takePending, persists what it needs and then share.ack.
 */
class ShareIntake(private val context: Context) {
    private val rootDir = File(context.filesDir, "share_inbox").apply { mkdirs() }
    private val pending = File(rootDir, "pending").apply { mkdirs() }
    private val prefs = context.getSharedPreferences("bowerbird_share_intake", Context.MODE_PRIVATE)
    private val executor = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())
    private val lock = Any()

    /** Handles intent asynchronously; invokes onReady(true) on main thread when an event was saved. */
    fun handleAsync(intent: Intent?, onReady: (Boolean) -> Unit = {}) {
        if (intent == null) {
            onReady(false)
            return
        }
        val action = intent.action
        if (action != Intent.ACTION_SEND && action != Intent.ACTION_SEND_MULTIPLE) {
            onReady(false)
            return
        }
        val uris = collectUris(intent, action)
        val text = intent.getCharSequenceExtra(Intent.EXTRA_TEXT)?.toString()
            ?: intent.getCharSequenceExtra(Intent.EXTRA_HTML_TEXT)?.toString()
        if (uris.isEmpty() && text.isNullOrBlank()) {
            onReady(false)
            return
        }
        val fingerprint = (action + "|" + (intent.type ?: "") + "|" +
            uris.joinToString("|") { it.toString() } + "#" + (text ?: "")).hashCode().toString()
        synchronized(lock) {
            val now = System.currentTimeMillis()
            val previous = prefs.getLong(fingerprint, 0)
            if (previous > 0 && now - previous < 15_000) {
                onReady(false)
                return
            }
            prefs.edit().putLong(fingerprint, now).apply()
            pruneFingerprints()
        }

        val subject = intent.getStringExtra(Intent.EXTRA_SUBJECT)
        val fallbackMime = intent.type ?: "*/*"
        executor.execute {
            val saved = runCatching { process(uris, text, subject, fallbackMime) }.getOrDefault(false)
            main.post { onReady(saved) }
        }
    }

    private fun process(uris: List<Uri>, text: String?, subject: String?, fallbackMime: String): Boolean {
        val now = System.currentTimeMillis()
        val id = UUID.randomUUID().toString()
        val dir = File(pending, id).apply { mkdirs() }
        val items = JSONArray()
        uris.forEachIndexed { index, uri ->
            val rawName = displayName(uri) ?: "shared-${index + 1}"
            var mime = context.contentResolver.getType(uri)
                ?: fallbackMime.takeIf { it != "*/*" }
                ?: "application/octet-stream"
            var safe = sanitizeFileName(rawName, "shared_$index")
            if (!safe.contains('.')) {
                extensionForMime(mime)?.let { safe = "$safe.$it" }
            }
            if (mime == "application/octet-stream") {
                val ext = safe.substringAfterLast('.', "")
                if (ext.isNotEmpty()) {
                    MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext.lowercase())?.let { mime = it }
                }
            }
            val target = File(dir, "${index}_$safe")
            val copied = runCatching {
                context.contentResolver.openInputStream(uri)?.use { input ->
                    target.outputStream().use { output -> input.copyTo(output, DEFAULT_BUFFER_SIZE) }
                }
                target.takeIf { it.exists() && it.length() > 0 }
            }.getOrNull()
            if (copied == null) {
                target.delete()
                return@forEachIndexed
            }
            items.put(JSONObject().apply {
                put("kind", "file")
                put("name", safe)
                put("path", copied.absolutePath)
                put("mimeType", mime)
                put("size", copied.length())
            })
        }
        if (!text.isNullOrBlank()) {
            items.put(JSONObject().apply {
                put("kind", "text")
                put("name", subject?.trim().orEmpty())
                put("text", text)
            })
        }
        if (items.length() == 0) {
            dir.deleteRecursively()
            return false
        }
        val event = JSONObject().apply {
            put("id", id)
            put("receivedAt", now)
            put("items", items)
        }
        synchronized(lock) {
            pending.mkdirs()
            val target = File(pending, "$id.json")
            val temp = File(pending, ".$id.tmp")
            temp.writeText(event.toString())
            if (!temp.renameTo(target)) {
                target.writeText(event.toString())
                temp.delete()
            }
        }
        return true
    }

    fun takePending(): List<Map<String, Any?>> {
        val out = mutableListOf<Map<String, Any?>>()
        synchronized(lock) {
            pending.listFiles { f -> f.isFile && f.name.endsWith(".json") }
                ?.sortedBy { it.lastModified() }
                ?.forEach { file ->
                    runCatching {
                        val obj = JSONObject(file.readText())
                        val id = obj.optString("id").ifBlank { obj.optString("eventId") }
                        val items = mutableListOf<Map<String, Any?>>()
                        val array = obj.optJSONArray("items") ?: JSONArray()
                        for (i in 0 until array.length()) {
                            val item = array.optJSONObject(i) ?: continue
                            items.add(
                                mapOf(
                                    "kind" to item.optString("kind"),
                                    "name" to item.optString("name"),
                                    "text" to item.optString("text"),
                                    "path" to item.optString("path"),
                                    "mimeType" to item.optString("mimeType"),
                                    "size" to item.optLong("size")
                                )
                            )
                        }
                        if (id.isNotBlank() && items.isNotEmpty()) {
                            out.add(
                                mapOf(
                                    "id" to id,
                                    "receivedAt" to obj.optLong("receivedAt"),
                                    "items" to items
                                )
                            )
                        }
                    }
                }
        }
        return out
    }

    fun ack(ids: List<String>) {
        synchronized(lock) {
            ids.forEach { id ->
                File(pending, "$id.json").delete()
                File(pending, id).deleteRecursively()
                File(rootDir, id).deleteRecursively()
            }
        }
    }

    private fun collectUris(intent: Intent, action: String): List<Uri> {
        val found = LinkedHashMap<String, Uri>()
        fun add(uri: Uri?) {
            if (uri != null && !uri.scheme.isNullOrEmpty()) {
                found[uri.toString()] = uri
            }
        }
        if (action == Intent.ACTION_SEND_MULTIPLE) {
            val streams = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri::class.java)
            } else {
                @Suppress("DEPRECATION")
                intent.getParcelableArrayListExtra<Uri>(Intent.EXTRA_STREAM)
            }
            streams?.forEach(::add)
        } else {
            val stream = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
            } else {
                @Suppress("DEPRECATION")
                intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM)
            }
            add(stream)
        }
        intent.clipData?.let { clip ->
            for (i in 0 until clip.itemCount) add(clip.getItemAt(i).uri)
        }
        add(intent.data)
        return found.values.toList()
    }

    private fun displayName(uri: Uri): String? {
        return runCatching {
            context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
                val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                if (index >= 0 && cursor.moveToFirst()) cursor.getString(index) else null
            }
        }.getOrNull() ?: uri.lastPathSegment?.substringAfterLast('/')
    }

    private fun sanitizeFileName(raw: String, fallback: String): String {
        var name = raw.replace(Regex("[\\\\/:*?\"<>|\\x00-\\x1F]"), "_").trim().trim('.')
        if (name.isEmpty()) name = fallback
        if (name.length > 100) {
            val dot = name.lastIndexOf('.')
            name = if (dot > 0 && name.length - dot <= 12) {
                name.substring(0, 88) + name.substring(dot)
            } else {
                name.substring(0, 100)
            }
        }
        return name
    }

    private fun extensionForMime(mimeType: String): String? {
        return when (mimeType.lowercase()) {
            "image/jpeg" -> "jpg"
            "image/png" -> "png"
            "image/gif" -> "gif"
            "image/webp" -> "webp"
            "video/mp4" -> "mp4"
            "video/quicktime" -> "mov"
            "video/webm" -> "webm"
            "text/plain" -> "txt"
            else -> MimeTypeMap.getSingleton().getExtensionFromMimeType(mimeType)
        }
    }

    private fun pruneFingerprints() {
        val all = prefs.all
        if (all.size <= 300) return
        val editor = prefs.edit()
        all.entries.sortedBy { (it.value as? Long) ?: 0L }.take(all.size - 300).forEach { editor.remove(it.key) }
        editor.apply()
    }
}
