package com.bowerbird.mobile

import android.Manifest
import android.app.Activity
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import android.window.BackEvent
import android.window.OnBackAnimationCallback
import android.window.OnBackInvokedCallback
import android.window.OnBackInvokedDispatcher
import androidx.annotation.RequiresApi
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
    private var pendingQrComponent: QrScannerComponent? = null
    private var detailBackCallback: Any? = null

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
        ensureNotificationChannel()
        app = NsgoView(this)
        app.setStorageDirectory(filesDir)
        app.setParams(mapOf(
            "deviceName" to (Build.MODEL ?: "Android"),
            "systemAccent" to systemAccentHex(),
        ))
        app.setErrorListener { message -> android.util.Log.e("bowerbird", message) }
        app.registerComponent("gallery_grid") { ctx, events -> GalleryGridComponent(ctx, events) }
        app.registerComponent("qr_scanner") { ctx, events ->
            QrScannerComponent(ctx, events) { comp ->
                pendingQrComponent = comp
                requestPermissions(arrayOf(Manifest.permission.CAMERA), REQ_INLINE_QR_PERM)
            }
        }
        installHandlers()
        setContentView(app)

        intake.handleAsync(intent) { saved ->
            if (saved) app.send("share.available", null)
        }
        intent?.data?.takeIf { it.scheme == "bowerbird" }?.let { app.send("openUrl", it.toString()) }
    }

    /**
     * Reads the platform's accent colour so the app's "follow system accent"
     * setting can use the same idea as the desktop client. Empty when the OS
     * does not expose one.
     */
    private fun systemAccentHex(): String {
        if (Build.VERSION.SDK_INT < 31) return ""
        return runCatching {
            val id = resources.getIdentifier("system_accent1_500", "color", "android")
            if (id == 0) return ""
            val color = resources.getColor(id, theme)
            String.format("#%06X", 0xFFFFFF and color)
        }.getOrDefault("")
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
        app.handle("notify.show") { data, reply ->
            val map = data as? Map<*, *>
            val title = (map?.get("title") as? String).orEmpty().ifBlank { getString(R.string.app_name) }
            val body = (map?.get("body") as? String).orEmpty()
            runOnUiThread {
                showSaveNotification(title, body)
                reply.resolve(null)
            }
        }
        app.handle("notify.requestPermission") { _, reply ->
            if (Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            ) {
                requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIFY_PERM)
            }
            reply.resolve(null)
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
        app.handle("detail.setPredictiveBack") { data, reply ->
            val enabled = (data as? Map<*, *>)?.get("enabled") == true
            setDetailPredictiveBack(enabled)
            reply.resolve(null)
        }
    }

    private fun setDetailPredictiveBack(enabled: Boolean) {
        if (Build.VERSION.SDK_INT < 33) return
        if (enabled) {
            if (detailBackCallback != null) return
            val callback = if (Build.VERSION.SDK_INT >= 34) {
                createDetailBackAnimationCallback()
            } else {
                OnBackInvokedCallback { app.send("detail.backInvoke", null) }
            }
            onBackInvokedDispatcher.registerOnBackInvokedCallback(
                OnBackInvokedDispatcher.PRIORITY_OVERLAY,
                callback
            )
            detailBackCallback = callback
        } else {
            unregisterDetailPredictiveBack()
        }
    }

    private fun unregisterDetailPredictiveBack() {
        if (Build.VERSION.SDK_INT < 33) return
        val callback = detailBackCallback as? OnBackInvokedCallback ?: return
        detailBackCallback = null
        runCatching { onBackInvokedDispatcher.unregisterOnBackInvokedCallback(callback) }
    }

    @RequiresApi(34)
    private fun createDetailBackAnimationCallback(): OnBackAnimationCallback {
        return object : OnBackAnimationCallback {
            override fun onBackStarted(backEvent: BackEvent) {
                app.send("detail.backStart", mapOf("progress" to backEvent.progress))
            }

            override fun onBackProgressed(backEvent: BackEvent) {
                app.send("detail.backProgress", mapOf("progress" to backEvent.progress))
            }

            override fun onBackCancelled() {
                app.send("detail.backCancel", null)
            }

            override fun onBackInvoked() {
                app.send("detail.backInvoke", null)
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
        if (request == REQ_INLINE_QR_PERM) {
            val granted = results.isNotEmpty() && results[0] == PackageManager.PERMISSION_GRANTED
            pendingQrComponent?.onPermissionResult(granted)
            pendingQrComponent = null
            return
        }
        app.permissionResult(request, permissions, results)
    }

    @Deprecated("The Activity API still routes the system back gesture here.")
    override fun onBackPressed() {
        if (!app.back()) super.onBackPressed()
    }

    override fun onDestroy() {
        unregisterDetailPredictiveBack()
        discovery.stop()
        ioExecutor.shutdownNow()
        app.stop()
        super.onDestroy()
    }

    private fun ensureNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = getSystemService(NotificationManager::class.java) ?: return
        if (manager.getNotificationChannel(SAVE_CHANNEL_ID) != null) return
        val channel = NotificationChannel(
            SAVE_CHANNEL_ID,
            "保存通知",
            NotificationManager.IMPORTANCE_DEFAULT
        ).apply {
            description = "分享保存到电脑后的结果通知"
        }
        manager.createNotificationChannel(channel)
    }

    @Suppress("DEPRECATION")
    private fun showSaveNotification(title: String, body: String) {
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        val manager = getSystemService(NotificationManager::class.java) ?: return
        val launchIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val piFlags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        } else {
            PendingIntent.FLAG_UPDATE_CURRENT
        }
        val pending = PendingIntent.getActivity(this, 0, launchIntent, piFlags)
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, SAVE_CHANNEL_ID)
        } else {
            Notification.Builder(this)
        }
        val notification = builder
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle(title)
            .setContentText(body)
            .setAutoCancel(true)
            .setContentIntent(pending)
            .build()
        manager.notify((System.currentTimeMillis() and 0x7FFFFFFF).toInt(), notification)
    }

    companion object {
        private const val REQ_QR_SCAN = 4091
        private const val REQ_INLINE_QR_PERM = 4092
        private const val REQ_NOTIFY_PERM = 4093
        private const val SAVE_CHANNEL_ID = "bowerbird_save"
    }
}
