package com.bowerbird.mobile

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.graphics.ImageFormat
import android.graphics.SurfaceTexture
import android.hardware.camera2.CameraCaptureSession
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraDevice
import android.hardware.camera2.CameraManager
import android.hardware.camera2.CaptureRequest
import android.media.ImageReader
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.HandlerThread
import android.provider.Settings
import android.util.Size
import android.view.Gravity
import android.view.Surface
import android.view.TextureView
import android.view.View
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Offline QR scanner built on Camera2 + ZXing core. Supports runtime CAMERA
 * permission handling with retry/settings fallback, torch toggle, and front/back
 * camera switching without requiring Google Play Services.
 */
class QrScanActivity : Activity() {
    private lateinit var textureView: TextureView
    private lateinit var statusText: TextView
    private lateinit var torchBtn: Button
    private lateinit var switchBtn: Button
    private lateinit var errorBox: LinearLayout
    private lateinit var errorText: TextView
    private lateinit var settingsBtn: Button

    private var cameraThread: HandlerThread? = null
    private var cameraHandler: Handler? = null
    private var cameraDevice: CameraDevice? = null
    private var captureSession: CameraCaptureSession? = null
    private var imageReader: ImageReader? = null

    private var lensFacing = CameraCharacteristics.LENS_FACING_BACK
    private var torchEnabled = false
    private var hasFlash = false
    private val found = AtomicBoolean(false)
    private var lastAnalyzeMs = 0L

    private val reader = MultiFormatReader().apply {
        setHints(
            mapOf(
                DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
                DecodeHintType.TRY_HARDER to true
            )
        )
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        buildUi()
        ensurePermissionAndStart()
    }

    private fun buildUi() {
        val root = FrameLayout(this).apply { setBackgroundColor(Color.BLACK) }

        textureView = TextureView(this)
        root.addView(
            textureView,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        // Viewfinder frame
        val frame = View(this).apply {
            setBackgroundColor(Color.TRANSPARENT)
            foreground = android.graphics.drawable.GradientDrawable().apply {
                setStroke(6, Color.parseColor("#60A5FA"))
                cornerRadius = 32f
                setColor(Color.TRANSPARENT)
            }
        }
        val boxSize = (resources.displayMetrics.density * 240).toInt()
        root.addView(
            frame,
            FrameLayout.LayoutParams(boxSize, boxSize, Gravity.CENTER)
        )

        // Top bar
        val topBar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(32, 96, 32, 24)
            setBackgroundColor(Color.parseColor("#88000000"))
        }
        val cancelBtn = Button(this).apply {
            text = "关闭"
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.TRANSPARENT)
            setOnClickListener {
                setResult(RESULT_CANCELED)
                finish()
            }
        }
        val title = TextView(this).apply {
            text = "扫码配对"
            textSize = 18f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER
        }
        torchBtn = Button(this).apply {
            text = "闪光灯"
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.TRANSPARENT)
            setOnClickListener { toggleTorch() }
        }
        switchBtn = Button(this).apply {
            text = "切镜头"
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.TRANSPARENT)
            setOnClickListener { switchCamera() }
        }
        topBar.addView(cancelBtn)
        topBar.addView(title, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
        topBar.addView(torchBtn)
        topBar.addView(switchBtn)
        root.addView(
            topBar,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.TOP
            )
        )

        // Bottom hint
        statusText = TextView(this).apply {
            text = "将电脑端显示的配对二维码放入框内"
            textSize = 14f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER
            setPadding(48, 28, 48, 64)
            setBackgroundColor(Color.parseColor("#88000000"))
        }
        root.addView(
            statusText,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM
            )
        )

        // Error / permission overlay
        errorBox = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setPadding(64, 64, 64, 64)
            setBackgroundColor(Color.parseColor("#CC000000"))
            visibility = View.GONE
        }
        errorText = TextView(this).apply {
            textSize = 15f
            setTextColor(Color.WHITE)
            gravity = Gravity.CENTER
        }
        val retryBtn = Button(this).apply {
            text = "重试"
            setOnClickListener { ensurePermissionAndStart() }
        }
        settingsBtn = Button(this).apply {
            text = "打开系统设置"
            setOnClickListener {
                startActivity(
                    Intent(
                        Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.parse("package:$packageName")
                    )
                )
            }
        }
        errorBox.addView(errorText)
        errorBox.addView(retryBtn)
        errorBox.addView(settingsBtn)
        root.addView(
            errorBox,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        setContentView(root)
    }

    private fun ensurePermissionAndStart() {
        errorBox.visibility = View.GONE
        if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            startPreview()
        } else {
            requestPermissions(arrayOf(Manifest.permission.CAMERA), REQ_CAMERA)
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == REQ_CAMERA) {
            if (grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
                startPreview()
            } else {
                showError("未授予相机权限，请允许 Bowerbird 访问相机后重试", showSettings = true)
            }
        }
    }

    override fun onResume() {
        super.onResume()
        if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED &&
            cameraDevice == null
        ) {
            startPreview()
        }
    }

    override fun onPause() {
        closeCamera()
        super.onPause()
    }

    private fun startPreview() {
        errorBox.visibility = View.GONE
        startCameraThread()
        if (textureView.isAvailable) {
            openCamera(textureView.width, textureView.height)
        } else {
            textureView.surfaceTextureListener = object : TextureView.SurfaceTextureListener {
                override fun onSurfaceTextureAvailable(surface: SurfaceTexture, width: Int, height: Int) {
                    openCamera(width, height)
                }
                override fun onSurfaceTextureSizeChanged(surface: SurfaceTexture, width: Int, height: Int) {}
                override fun onSurfaceTextureDestroyed(surface: SurfaceTexture): Boolean = true
                override fun onSurfaceTextureUpdated(surface: SurfaceTexture) {}
            }
        }
    }

    private fun openCamera(viewW: Int, viewH: Int) {
        val mgr = getSystemService(Context.CAMERA_SERVICE) as CameraManager
        val camId = chooseCamera(mgr) ?: run {
            showError("未找到可用摄像头", showSettings = false)
            return
        }
        val chars = mgr.getCameraCharacteristics(camId)
        hasFlash = chars.get(CameraCharacteristics.FLASH_INFO_AVAILABLE) == true
        runOnUiThread {
            torchBtn.visibility = if (hasFlash) View.VISIBLE else View.GONE
        }

        val map = chars.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP)
        val previewSize = chooseSize(map?.getOutputSizes(SurfaceTexture::class.java), viewW, viewH)
        val analyzeSize = chooseAnalyzeSize(map?.getOutputSizes(ImageFormat.YUV_420_888))

        imageReader?.close()
        imageReader = ImageReader.newInstance(analyzeSize.width, analyzeSize.height, ImageFormat.YUV_420_888, 2).apply {
            setOnImageAvailableListener({ ir ->
                val img = ir.acquireLatestImage() ?: return@setOnImageAvailableListener
                try {
                    val now = System.currentTimeMillis()
                    if (found.get() || now - lastAnalyzeMs < 150) return@setOnImageAvailableListener
                    lastAnalyzeMs = now
                    val plane = img.planes[0]
                    val buf = plane.buffer
                    val data = ByteArray(buf.remaining())
                    buf.get(data)
                    val w = img.width
                    val h = img.height
                    val rowStride = plane.rowStride
                    val yData = if (rowStride == w) {
                        data
                    } else {
                        ByteArray(w * h).also { compact ->
                            for (row in 0 until h) {
                                val srcOff = row * rowStride
                                val len = minOf(w, data.size - srcOff)
                                if (len > 0) System.arraycopy(data, srcOff, compact, row * w, len)
                            }
                        }
                    }
                    val source = PlanarYUVLuminanceSource(yData, w, h, 0, 0, w, h, false)
                    val bitmap = BinaryBitmap(HybridBinarizer(source))
                    val result = runCatching { reader.decodeWithState(bitmap) }.getOrNull()
                    reader.reset()
                    val text = result?.text?.trim()
                    if (!text.isNullOrEmpty() && found.compareAndSet(false, true)) {
                        runOnUiThread {
                            setResult(RESULT_OK, Intent().putExtra(EXTRA_PAYLOAD, text))
                            finish()
                        }
                    }
                } finally {
                    img.close()
                }
            }, cameraHandler)
        }

        if (checkSelfPermission(Manifest.permission.CAMERA) != PackageManager.PERMISSION_GRANTED) return
        try {
            mgr.openCamera(camId, object : CameraDevice.StateCallback() {
                override fun onOpened(camera: CameraDevice) {
                    cameraDevice = camera
                    createSession(camera, previewSize)
                }
                override fun onDisconnected(camera: CameraDevice) {
                    camera.close()
                    cameraDevice = null
                }
                override fun onError(camera: CameraDevice, error: Int) {
                    camera.close()
                    cameraDevice = null
                    showError("相机启动失败 (错误码 $error)", showSettings = false)
                }
            }, cameraHandler)
        } catch (e: Exception) {
            showError("无法打开相机: ${e.message}", showSettings = false)
        }
    }

    private fun createSession(camera: CameraDevice, previewSize: Size) {
        val st = textureView.surfaceTexture ?: return
        st.setDefaultBufferSize(previewSize.width, previewSize.height)
        val previewSurface = Surface(st)
        val analyzeSurface = imageReader?.surface ?: return
        @Suppress("DEPRECATION")
        camera.createCaptureSession(
            listOf(previewSurface, analyzeSurface),
            object : CameraCaptureSession.StateCallback() {
                override fun onConfigured(session: CameraCaptureSession) {
                    if (cameraDevice == null) return
                    captureSession = session
                    applyCaptureRequest(camera, session, previewSurface, analyzeSurface)
                }
                override fun onConfigureFailed(session: CameraCaptureSession) {
                    showError("相机预览配置失败", showSettings = false)
                }
            },
            cameraHandler
        )
    }

    private fun applyCaptureRequest(
        camera: CameraDevice,
        session: CameraCaptureSession,
        previewSurface: Surface,
        analyzeSurface: Surface
    ) {
        val builder = camera.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW).apply {
            addTarget(previewSurface)
            addTarget(analyzeSurface)
            set(CaptureRequest.CONTROL_AF_MODE, CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE)
            if (hasFlash && torchEnabled) {
                set(CaptureRequest.FLASH_MODE, CaptureRequest.FLASH_MODE_TORCH)
            } else {
                set(CaptureRequest.FLASH_MODE, CaptureRequest.FLASH_MODE_OFF)
            }
        }
        runCatching { session.setRepeatingRequest(builder.build(), null, cameraHandler) }
    }

    private fun toggleTorch() {
        if (!hasFlash) return
        torchEnabled = !torchEnabled
        torchBtn.text = if (torchEnabled) "关灯" else "闪光灯"
        val cam = cameraDevice ?: return
        val session = captureSession ?: return
        val st = textureView.surfaceTexture ?: return
        val analyzeSurface = imageReader?.surface ?: return
        applyCaptureRequest(cam, session, Surface(st), analyzeSurface)
    }

    private fun switchCamera() {
        lensFacing = if (lensFacing == CameraCharacteristics.LENS_FACING_BACK) {
            CameraCharacteristics.LENS_FACING_FRONT
        } else {
            CameraCharacteristics.LENS_FACING_BACK
        }
        torchEnabled = false
        torchBtn.text = "闪光灯"
        closeCamera()
        startPreview()
    }

    private fun chooseCamera(mgr: CameraManager): String? {
        for (id in mgr.cameraIdList) {
            val facing = mgr.getCameraCharacteristics(id).get(CameraCharacteristics.LENS_FACING)
            if (facing == lensFacing) return id
        }
        return mgr.cameraIdList.firstOrNull()
    }

    private fun chooseSize(choices: Array<Size>?, w: Int, h: Int): Size {
        val list = choices?.toList().orEmpty()
        return list.firstOrNull { it.width <= 1920 && it.height <= 1080 } ?: list.firstOrNull() ?: Size(1280, 720)
    }

    private fun chooseAnalyzeSize(choices: Array<Size>?): Size {
        val list = choices?.toList().orEmpty()
        return list.filter { it.width in 640..1280 && it.height in 480..960 }
            .minByOrNull { kotlin.math.abs(it.width - 960) }
            ?: list.firstOrNull()
            ?: Size(640, 480)
    }

    private fun closeCamera() {
        runCatching { captureSession?.close() }
        captureSession = null
        runCatching { cameraDevice?.close() }
        cameraDevice = null
        runCatching { imageReader?.close() }
        imageReader = null
        cameraThread?.quitSafely()
        cameraThread = null
        cameraHandler = null
    }

    private fun startCameraThread() {
        if (cameraThread != null) return
        cameraThread = HandlerThread("qr-camera").apply { start() }
        cameraHandler = Handler(cameraThread!!.looper)
    }

    private fun showError(msg: String, showSettings: Boolean) {
        runOnUiThread {
            errorText.text = msg
            settingsBtn.visibility = if (showSettings) View.VISIBLE else View.GONE
            errorBox.visibility = View.VISIBLE
        }
    }

    companion object {
        const val EXTRA_PAYLOAD = "payload"
        private const val REQ_CAMERA = 9001
    }
}
