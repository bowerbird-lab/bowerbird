package com.bowerbird.mobile

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.ContentValues
import android.content.Context
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import java.io.File
import java.io.FileInputStream

/**
 * Native media & migration helpers:
 * - File URI clipboard via BowerbirdFileProvider (ClipData + FileProvider)
 * - Streaming MediaStore export to Pictures/Movies/Downloads/Bowerbird
 * - Discovery of legacy Flutter app data directories for one-time migration
 */
object MediaBridge {
    private const val MAX_CLIPBOARD_BYTES = 500_000_000L
    private const val CLIPBOARD_TTL_MS = 24L * 60L * 60L * 1000L

    fun copyFileToClipboard(context: Context, path: String, mimeType: String, displayName: String) {
        val src = File(path)
        require(src.isFile) { "File not found: $path" }
        require(src.length() <= MAX_CLIPBOARD_BYTES) { "File exceeds 500 MB clipboard limit" }

        val clipboardDir = File(context.cacheDir, "clipboard").apply { mkdirs() }.canonicalFile
        val canonicalSrc = src.canonicalFile
        val target = if (canonicalSrc.path.startsWith(clipboardDir.path + File.separator)) {
            canonicalSrc
        } else {
            val safeName = (displayName.ifBlank { canonicalSrc.name }).replace(Regex("[^A-Za-z0-9._-]"), "_")
            val dest = File(clipboardDir, safeName)
            canonicalSrc.inputStream().use { input ->
                dest.outputStream().use { output -> input.copyTo(output, 65536) }
            }
            dest.canonicalFile
        }

        val mime = mimeType.ifBlank { "application/octet-stream" }
        val label = displayName.ifBlank { target.name }
        val uri = BowerbirdFileProvider.uriForClipboardFile(context, target, label, mime)
        val clip = ClipData(ClipDescription(label, arrayOf(mime)), ClipData.Item(uri))
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(clip)

        val cutoff = System.currentTimeMillis() - CLIPBOARD_TTL_MS
        clipboardDir.listFiles()?.forEach { candidate ->
            if (candidate != target && candidate.isFile && candidate.lastModified() < cutoff) {
                candidate.delete()
            }
        }
    }

    fun saveToMediaStore(context: Context, path: String, filename: String, mimeType: String): String {
        val src = File(path)
        require(src.isFile) { "Source file not found: $path" }
        val resolver = context.contentResolver
        val mime = mimeType.ifBlank { "application/octet-stream" }
        val lowerMime = mime.lowercase()
        val isVideo = lowerMime.startsWith("video/")
        val isImage = lowerMime.startsWith("image/")

        val collection = when {
            isVideo -> if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                MediaStore.Video.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
            } else {
                MediaStore.Video.Media.EXTERNAL_CONTENT_URI
            }
            isImage -> if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
            } else {
                MediaStore.Images.Media.EXTERNAL_CONTENT_URI
            }
            else -> {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                    MediaStore.Downloads.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
                } else {
                    throw IllegalStateException("Non-media export requires SAF on Android < 10")
                }
            }
        }

        val relDir = when {
            isVideo -> Environment.DIRECTORY_MOVIES + "/Bowerbird"
            isImage -> Environment.DIRECTORY_PICTURES + "/Bowerbird"
            else -> Environment.DIRECTORY_DOWNLOADS + "/Bowerbird"
        }

        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, filename.ifBlank { src.name })
            put(MediaStore.MediaColumns.MIME_TYPE, mime)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                put(MediaStore.MediaColumns.RELATIVE_PATH, relDir)
                put(MediaStore.MediaColumns.IS_PENDING, 1)
            }
        }

        val uri = resolver.insert(collection, values)
            ?: throw IllegalStateException("Failed to create MediaStore entry")

        try {
            resolver.openOutputStream(uri)?.use { out ->
                FileInputStream(src).use { input ->
                    input.copyTo(out, 65536)
                }
                out.flush()
            } ?: throw IllegalStateException("Failed to open MediaStore output stream")

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                values.clear()
                values.put(MediaStore.MediaColumns.IS_PENDING, 0)
                resolver.update(uri, values, null, null)
            }
            return uri.toString()
        } catch (e: Throwable) {
            runCatching { resolver.delete(uri, null, null) }
            throw e
        }
    }

    fun scanLegacyFlutter(context: Context): Map<String, Any?> {
        val flutterDir = runCatching { context.getDir("flutter", Context.MODE_PRIVATE) }.getOrNull()
        val filesDir = context.filesDir
        val cacheDir = context.cacheDir

        val inboxCandidates = listOf(
            File(filesDir, "share_inbox/index.json"),
            File(cacheDir, "share_inbox/index.json")
        ).firstOrNull { it.isFile }

        val shareTagsCandidates = buildList {
            if (flutterDir != null) add(File(flutterDir, "share_inbox/share_tags.json"))
            add(File(filesDir, "share_inbox/share_tags.json"))
        }.firstOrNull { it.isFile }

        val securePrefs = context.getSharedPreferences("FlutterSecureStorage", Context.MODE_PRIVATE)
        val hasSecureKeys = securePrefs.all.keys.any { it.contains("bowerbird") }

        return mapOf(
            "flutterDir" to flutterDir?.absolutePath,
            "filesDir" to filesDir.absolutePath,
            "cacheDir" to cacheDir.absolutePath,
            "legacyInboxPath" to inboxCandidates?.absolutePath,
            "legacyShareTagsPath" to shareTagsCandidates?.absolutePath,
            "hasLegacySecure" to hasSecureKeys
        )
    }
}
