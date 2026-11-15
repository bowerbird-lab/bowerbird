package com.bowerbird.mobile

import android.app.Activity
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import dev.oneo.nsgo.NsgoView
import java.util.concurrent.Executors

/**
 * The Kotlin host around the NSGo client. It owns the process-level platform
 * capabilities (Keystore, NSD, QR scanner, FileProvider clipboard, MediaStore,
 * share intake, deep links) and runs one NsgoView.
 */
class MainActivity : Activity() {
    private lateinit var app: NsgoView
    private lateinit var secure: SecurePrefs
    private lateinit var discovery: Discovery
    private lateinit var intake: ShareIntake
    private val ioExecutor = Executors.newCachedThreadPool()
    private var scanReply: NsgoView.HostReply? = null

    override fun onCreate(state: Bundle?) {
        val night = resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK
        if (night != Configuration.UI_MODE_NIGHT_YES) {
            setTheme(android.R.style.Theme_DeviceDefault_Light_NoActionBar)
        }
        super.onCreate(state)
        configureEdgeToEdge()

        secure = SecurePrefs(this)
        discovery = Discovery(this)
        intake = ShareIntake(this)
        app = NsgoView(this)
        app.setStorageDirectory(filesDir)
        app.setParams(mapOf("deviceName" to (Build.MODEL ?: "Android")))
        app.setErrorListener { message -> android.util.Log.e("bowerbird", message) }
        app.registerComponent("gallery_grid") { ctx, events -> GalleryGridComponent(ctx, events) }
        installHandlers()
        setContentView(app)

        intake.handleAsync(intent) { saved ->
            if (saved) app.send("share.available", null)
        }
        intent?.data?.takeIf { it.scheme == "bowerbird" }?.let { app.send("openUrl", it.toString()) }
    }

    @Suppress("DEPRECATION")
    private fun configureEdgeToEdge() {
        val lp = window.attributes
        lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
        window.attributes = lp
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
        window.statusBarColor = Color.TRANSPARENT
        window.navigationBarColor = Color.TRANSPARENT
        if (Build.VERSION.SDK_INT >= 30) {
            window.setDecorFitsSystemWindows(false)
        }
    }

    private fun installHandlers() {
        app.handle("secure.get") { data, reply ->
            val key = (data as? Map<*, *>)?.get("key") as? String
            if (key == null) reply.reject("key is required") else reply.resolve(mapOf("value" to secure.get(key)))
        }
        app.handle("secure.set") { data, reply ->
            val map = data as? Map<*, *>
            val key = map?.get("key") as? String
            val value = map?.get("value") as? String
            if (key == null || value == null) {
                reply.reject("key and value are required")
            } else {
                secure.set(key, value)
                reply.resolve(null)
            }
        }
        app.handle("secure.delete") { data, reply ->
            val key = (data as? Map<*, *>)?.get("key") as? String
            if (key == null) reply.reject("key is required") else {
                secure.delete(key)
                reply.resolve(null)
            }
        }
        app.handle("discovery.browse") { _, reply -> discovery.browse(reply) }
        app.handle("discovery.stop") { _, reply ->
            discovery.stop()
            reply.resolve(null)
        }
        app.handle("share.takePending") { _, reply ->
            ioExecutor.execute {
                val events = intake.takePending()
                runOnUiThread { reply.resolve(events) }
            }
        }
        app.handle("share.ack") { data, reply ->
            val ids = ((data as? Map<*, *>)?.get("ids") as? List<*>)?.mapNotNull { it as? String } ?: emptyList()
            ioExecutor.execute {
                intake.ack(ids)
                runOnUiThread { reply.resolve(null) }
            }
        }
        app.handle("scanner.scan") { _, reply ->
            if (scanReply != null) {
                reply.reject("scanner already active")
                return@handle
            }
            scanReply = reply
            @Suppress("DEPRECATION")
            startActivityForResult(Intent(this, QrScanActivity::class.java), REQ_QR_SCAN)
        }
        app.handle("clipboard.copyFile") { data, reply ->
            val map = data as? Map<*, *>
            val path = map?.get("path") as? String
            val mimeType = (map?.get("mimeType") as? String).orEmpty()
            val displayName = (map?.get("displayName") as? String).orEmpty()
            if (path.isNullOrBlank()) {
                reply.reject("path is required")
                return@handle
            }
            ioExecutor.execute {
                runCatching {
                    MediaBridge.copyFileToClipboard(this, path, mimeType, displayName)
                }.onSuccess {
                    runOnUiThread { reply.resolve(mapOf("ok" to true)) }
                }.onFailure { err ->
                    runOnUiThread { reply.reject(err.message ?: "copy failed") }
                }
            }
        }
        app.handle("media.saveToGallery") { data, reply ->
            val map = data as? Map<*, *>
            val path = map?.get("path") as? String
            val name = (map?.get("name") as? String).orEmpty()
            val mimeType = (map?.get("mimeType") as? String).orEmpty()
            if (path.isNullOrBlank()) {
                reply.reject("path is required")
                return@handle
            }
            ioExecutor.execute {
                runCatching {
                    MediaBridge.saveToMediaStore(this, path, name, mimeType)
                }.onSuccess { uri ->
                    runOnUiThread { reply.resolve(mapOf("uri" to uri)) }
                }.onFailure { err ->
                    runOnUiThread { reply.reject(err.message ?: "save failed") }
                }
            }
        }
        app.handle("migration.scanLegacy") { _, reply ->
            ioExecutor.execute {
                val info = MediaBridge.scanLegacyFlutter(this)
                runOnUiThread { reply.resolve(info) }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intake.handleAsync(intent) { saved ->
            if (saved) app.send("share.available", null)
        }
        intent.data?.takeIf { it.scheme == "bowerbird" }?.let { app.send("openUrl", it.toString()) }
    }

    override fun onActivityResult(request: Int, result: Int, data: Intent?) {
        super.onActivityResult(request, result, data)
        if (request == REQ_QR_SCAN) {
            val reply = scanReply
            scanReply = null
            if (reply != null) {
                if (result == RESULT_OK) {
                    val payload = data?.getStringExtra(QrScanActivity.EXTRA_PAYLOAD).orEmpty()
                    reply.resolve(mapOf("payload" to payload, "cancelled" to false))
                } else {
                    reply.resolve(mapOf("payload" to "", "cancelled" to true))
                }
            }
            return
        }
        app.activityResult(request, result, data)
    }

    override fun onRequestPermissionsResult(request: Int, permissions: Array<String>, results: IntArray) {
        super.onRequestPermissionsResult(request, permissions, results)
        app.permissionResult(request, permissions, results)
    }

    @Deprecated("The Activity API still routes the system back gesture here.")
    override fun onBackPressed() {
        if (!app.back()) super.onBackPressed()
    }

    override fun onDestroy() {
        discovery.stop()
        ioExecutor.shutdownNow()
        app.stop()
        super.onDestroy()
    }

    companion object {
        private const val REQ_QR_SCAN = 4091
    }
}
