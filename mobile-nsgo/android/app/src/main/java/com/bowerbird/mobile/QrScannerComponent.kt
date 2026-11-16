package com.bowerbird.mobile

import android.Manifest
import android.animation.ValueAnimator
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.BlurMaskFilter
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ImageFormat
import android.graphics.LinearGradient
import android.graphics.Matrix
import android.graphics.Outline
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.graphics.Shader
import android.graphics.SurfaceTexture
import android.view.animation.AccelerateDecelerateInterpolator
import android.hardware.camera2.CameraCaptureSession
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraDevice
import android.hardware.camera2.CameraManager
import android.hardware.camera2.CaptureRequest
import android.media.ImageReader
import android.net.Uri
import android.os.Handler
import android.os.HandlerThread
import android.provider.Settings
import android.util.Size
import android.view.Gravity
import android.view.Surface
import android.view.TextureView
import android.view.View
import android.view.ViewOutlineProvider
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
import dev.oneo.nsgo.NsgoComponent
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.math.abs
import kotlin.math.max

/**
 * Inline square Camera2 + ZXing QR scanner component embedded inside the welcome card.
 */
class QrScannerComponent(
    private val context: Context,
    private val events: NsgoComponent.Events,
    private val requestPermission: (QrScannerComponent) -> Unit
) : NsgoComponent {

    private val dp = context.resources.displayMetrics.density
    private fun dp(v: Float): Int = (v * dp + 0.5f).toInt()

    private val root = FrameLayout(context).apply {
        setBackgroundColor(Color.BLACK)
        clipToOutline = true
        outlineProvider = object : ViewOutlineProvider() {
            override fun getOutline(view: View, outline: Outline) {
                outline.setRoundRect(0, 0, view.width, view.height, dp(32f).toFloat())
            }
        }
    }

    private val textureView = TextureView(context)
    private val effectsView = ScannerEffectsView(context, dp)
    private val fallbackBox = LinearLayout(context).apply {
        orientation = LinearLayout.VERTICAL
        gravity = Gravity.CENTER
        setPadding(dp(20f), dp(20f), dp(20f), dp(20f))
        setBackgroundColor(Color.parseColor("#181A20"))
        visibility = View.GONE
    }
    private val fallbackText = TextView(context).apply {
        textSize = 14f
        setTextColor(Color.WHITE)
        gravity = Gravity.CENTER
    }
    private val grantBtn = Button(context).apply {
        text = "授予相机权限"
        setOnClickListener {
            if (context.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
                startPreview()
            } else {
                requestPermission(this@QrScannerComponent)
            }
        }
    }
    private val settingsBtn = Button(context).apply {
        text = "打开系统设置"
        visibility = View.GONE
        setOnClickListener {
            context.startActivity(
                Intent(
                    Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                    Uri.parse("package:${context.packageName}")
                ).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        }
    }

    private val reader = MultiFormatReader().apply {
        setHints(mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE)))
    }
    private val found = AtomicBoolean(false)
    private var lastAnalyzeMs = 0L
    private var active = false
    private var cameraThread: HandlerThread? = null
    private var cameraHandler: Handler? = null
    private var cameraDevice: CameraDevice? = null
    private var captureSession: CameraCaptureSession? = null
    private var imageReader: ImageReader? = null

    init {
        root.addView(
            textureView,
            FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT)
        )
        root.addView(
            effectsView,
            FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT)
        )
        fallbackBox.addView(fallbackText)
        fallbackBox.addView(grantBtn)
        fallbackBox.addView(settingsBtn)
        root.addView(
            fallbackBox,
            FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT)
        )
    }

    override fun view(): View = root

    override fun setProp(name: String, value: Any?) {
        when (name) {
            "active" -> {
                val nextActive = value as? Boolean ?: false
                if (nextActive == active) return
                active = nextActive
                if (active) {
                    found.set(false)
                    effectsView.startAnim()
                    ensurePermissionAndStart()
                } else {
                    effectsView.stopAnim()
                    closeCamera()
                }
            }
            "accent" -> {
                val color = (value as? Number)?.toInt()
                if (color != null && color != 0) {
                    effectsView.setAccentColor(color)
                }
            }
            "resetSeq" -> {
                val resetSeq = (value as? Number)?.toInt() ?: 0
                if (resetSeq != lastResetSeq) {
                    lastResetSeq = resetSeq
                    found.set(false)
                }
            }
        }
    }

    private var lastResetSeq = 0

    override fun destroy() {
        active = false
        effectsView.stopAnim()
        closeCamera()
    }

    fun onPermissionResult(granted: Boolean) {
        if (!active) return
        if (granted) {
            startPreview()
        } else {
            showFallback("需要相机权限才能扫描配对二维码", showSettings = true)
        }
    }

    private fun ensurePermissionAndStart() {
        fallbackBox.visibility = View.GONE
        if (context.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            startPreview()
        } else {
            requestPermission(this)
        }
    }

    private fun showFallback(msg: String, showSettings: Boolean) {
        root.post {
            fallbackText.text = msg
            settingsBtn.visibility = if (showSettings) View.VISIBLE else View.GONE
            fallbackBox.visibility = View.VISIBLE
        }
    }

    private fun startPreview() {
        fallbackBox.visibility = View.GONE
        startCameraThread()
        if (textureView.isAvailable && textureView.width > 0 && textureView.height > 0) {
            openCamera(textureView.width, textureView.height)
        } else {
            textureView.surfaceTextureListener = object : TextureView.SurfaceTextureListener {
                override fun onSurfaceTextureAvailable(surface: SurfaceTexture, width: Int, height: Int) {
                    if (active) openCamera(width, height)
                }
                override fun onSurfaceTextureSizeChanged(surface: SurfaceTexture, width: Int, height: Int) {
                    if (active) applyCenterCropTransform(width, height)
                }
                override fun onSurfaceTextureDestroyed(surface: SurfaceTexture): Boolean = true
                override fun onSurfaceTextureUpdated(surface: SurfaceTexture) {}
            }
        }
    }

    private var currentPreviewSize: Size? = null

    private fun applyCenterCropTransform(viewW: Int, viewH: Int) {
        val preview = currentPreviewSize ?: return
        if (viewW <= 0 || viewH <= 0) return
        // Camera sensor is usually landscape; portrait view swaps w/h.
        val bufW = preview.height.toFloat()
        val bufH = preview.width.toFloat()
        val scale = max(viewW / bufW, viewH / bufH)
        val sx = (bufW * scale) / viewW
        val sy = (bufH * scale) / viewH
        val matrix = Matrix()
        matrix.setScale(sx, sy, viewW / 2f, viewH / 2f)
        root.post { textureView.setTransform(matrix) }
    }

    @SuppressLint("MissingPermission")
    private fun openCamera(viewW: Int, viewH: Int) {
        if (cameraDevice != null) return
        val mgr = context.getSystemService(Context.CAMERA_SERVICE) as CameraManager
        val camId = chooseBackCamera(mgr) ?: run {
            showFallback("未找到可用后置摄像头", showSettings = false)
            return
        }
        val chars = mgr.getCameraCharacteristics(camId)
        val map = chars.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP)
        val previewSize = chooseSquareFriendlySize(map?.getOutputSizes(SurfaceTexture::class.java), viewW, viewH)
        currentPreviewSize = previewSize
        applyCenterCropTransform(viewW, viewH)

        val analyzeSize = chooseAnalyzeSize(map?.getOutputSizes(ImageFormat.YUV_420_888))
        imageReader?.close()
        imageReader = ImageReader.newInstance(analyzeSize.width, analyzeSize.height, ImageFormat.YUV_420_888, 2).apply {
            setOnImageAvailableListener({ ir ->
                val img = ir.acquireLatestImage() ?: return@setOnImageAvailableListener
                try {
                    val now = System.currentTimeMillis()
                    if (!active || found.get() || now - lastAnalyzeMs < 150) return@setOnImageAvailableListener
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
                                System.arraycopy(data, row * rowStride, compact, row * w, w)
                            }
                        }
                    }
                    val source = PlanarYUVLuminanceSource(yData, w, h, 0, 0, w, h, false)
                    val bitmap = BinaryBitmap(HybridBinarizer(source))
                    val result = runCatching { reader.decodeWithState(bitmap) }.getOrNull()
                    reader.reset()
                    if (result != null && found.compareAndSet(false, true)) {
                        val payload = result.text
                        root.post { events.emit("scanned", mapOf("payload" to payload)) }
                    }
                } finally {
                    img.close()
                }
            }, cameraHandler)
        }

        runCatching {
            mgr.openCamera(camId, object : CameraDevice.StateCallback() {
                override fun onOpened(camera: CameraDevice) {
                    if (!active) {
                        camera.close()
                        return
                    }
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
                    showFallback("无法打开相机 ($error)", showSettings = false)
                }
            }, cameraHandler)
        }.onFailure {
            showFallback("打开相机失败: ${it.message}", showSettings = true)
        }
    }

    private fun createSession(camera: CameraDevice, previewSize: Size) {
        val st = textureView.surfaceTexture ?: return
        st.setDefaultBufferSize(previewSize.width, previewSize.height)
        val previewSurface = Surface(st)
        val analyzeSurface = imageReader?.surface ?: return
        val targets = listOf(previewSurface, analyzeSurface)
        @Suppress("DEPRECATION")
        camera.createCaptureSession(targets, object : CameraCaptureSession.StateCallback() {
            override fun onConfigured(session: CameraCaptureSession) {
                if (!active) {
                    session.close()
                    return
                }
                captureSession = session
                val req = camera.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW).apply {
                    addTarget(previewSurface)
                    addTarget(analyzeSurface)
                    set(CaptureRequest.CONTROL_AF_MODE, CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE)
                }
                session.setRepeatingRequest(req.build(), null, cameraHandler)
            }

            override fun onConfigureFailed(session: CameraCaptureSession) {
                showFallback("相机预览初始化失败", showSettings = false)
            }
        }, cameraHandler)
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
        cameraThread = HandlerThread("bowerbird-inline-qr").apply { start() }
        cameraHandler = Handler(cameraThread!!.looper)
    }

    private fun chooseBackCamera(mgr: CameraManager): String? {
        return mgr.cameraIdList.firstOrNull { id ->
            mgr.getCameraCharacteristics(id).get(CameraCharacteristics.LENS_FACING) ==
                CameraCharacteristics.LENS_FACING_BACK
        } ?: mgr.cameraIdList.firstOrNull()
    }

    private fun chooseSquareFriendlySize(sizes: Array<Size>?, viewW: Int, viewH: Int): Size {
        if (sizes.isNullOrEmpty()) return Size(1080, 1080)
        val target = max(viewW, viewH).coerceIn(720, 1440)
        return sizes.minByOrNull { abs(it.height - target) + abs(it.width - it.height) } ?: sizes[0]
    }

    private fun chooseAnalyzeSize(sizes: Array<Size>?): Size {
        if (sizes.isNullOrEmpty()) return Size(1280, 720)
        return sizes.filter { it.width in 640..1920 && it.height in 480..1080 }
            .minByOrNull { abs(it.width - 1280) } ?: sizes[0]
    }

    /**
     * Cool glowing frame, corner brackets, and sweeping laser + gradient trail overlay.
     */
    private class ScannerEffectsView(context: Context, private val density: Float) : View(context) {
        private fun dp(v: Float): Float = v * density

        private var progress = 0f
        private var movingDown = true
        private var lastVal = 0f
        private var accentColor = Color.rgb(56, 189, 248)

        private fun withAlpha(color: Int, alpha: Int): Int {
            return (alpha.coerceIn(0, 255) shl 24) or (color and 0x00FFFFFF)
        }

        fun setAccentColor(color: Int) {
            accentColor = color
            framePaint.color = withAlpha(color, 130)
            cornerGlowPaint.color = withAlpha(color, 120)
            cornerPaint.color = withAlpha(color, 255)
            lineGlowPaint.color = withAlpha(color, 150)
            sparkPaint.color = withAlpha(color, 210)
            invalidate()
        }

        private val framePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = dp(2f)
            color = withAlpha(accentColor, 130)
        }

        private val cornerGlowPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = dp(6f)
            strokeCap = Paint.Cap.ROUND
            strokeJoin = Paint.Join.ROUND
            color = withAlpha(accentColor, 120)
            maskFilter = BlurMaskFilter(dp(6f), BlurMaskFilter.Blur.NORMAL)
        }

        private val cornerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = dp(3.2f)
            strokeCap = Paint.Cap.ROUND
            strokeJoin = Paint.Join.ROUND
            color = accentColor
        }

        private val lineGlowPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = dp(8f)
            strokeCap = Paint.Cap.ROUND
            color = withAlpha(accentColor, 150)
            maskFilter = BlurMaskFilter(dp(8f), BlurMaskFilter.Blur.NORMAL)
        }

        private val lineCorePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = dp(2.4f)
            strokeCap = Paint.Cap.ROUND
            color = Color.WHITE
        }

        private val trailPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.FILL
        }

        private val sparkPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.FILL
            color = withAlpha(accentColor, 210)
        }

        private val cornerPath = Path()
        private val frameRect = RectF()

        private val animator = ValueAnimator.ofFloat(0f, 1f).apply {
            duration = 2100L
            repeatCount = ValueAnimator.INFINITE
            repeatMode = ValueAnimator.REVERSE
            interpolator = AccelerateDecelerateInterpolator()
            addUpdateListener { anim ->
                val v = anim.animatedValue as Float
                movingDown = v >= lastVal
                lastVal = v
                progress = v
                invalidate()
            }
        }

        init {
            setLayerType(LAYER_TYPE_SOFTWARE, null)
        }

        fun startAnim() {
            if (!animator.isStarted) animator.start()
        }

        fun stopAnim() {
            animator.cancel()
        }

        override fun onDetachedFromWindow() {
            stopAnim()
            super.onDetachedFromWindow()
        }

        override fun onDraw(canvas: Canvas) {
            super.onDraw(canvas)
            val w = width.toFloat()
            val h = height.toFloat()
            if (w <= 0f || h <= 0f) return

            val inset = dp(18f)
            val left = inset
            val top = inset
            val right = w - inset
            val bottom = h - inset
            val r = dp(22f)

            // Outer subtle rounded frame
            frameRect.set(dp(2f), dp(2f), w - dp(2f), h - dp(2f))
            canvas.drawRoundRect(frameRect, dp(30f), dp(30f), framePaint)

            // 4 glowing corner brackets
            val arm = dp(24f)
            cornerPath.reset()
            // Top-left
            cornerPath.moveTo(left, top + arm)
            cornerPath.lineTo(left, top + r * 0.6f)
            cornerPath.quadTo(left, top, left + r * 0.6f, top)
            cornerPath.lineTo(left + arm, top)
            // Top-right
            cornerPath.moveTo(right - arm, top)
            cornerPath.lineTo(right - r * 0.6f, top)
            cornerPath.quadTo(right, top, right, top + r * 0.6f)
            cornerPath.lineTo(right, top + arm)
            // Bottom-right
            cornerPath.moveTo(right, bottom - arm)
            cornerPath.lineTo(right, bottom - r * 0.6f)
            cornerPath.quadTo(right, bottom, right - r * 0.6f, bottom)
            cornerPath.lineTo(right - arm, bottom)
            // Bottom-left
            cornerPath.moveTo(left + arm, bottom)
            cornerPath.lineTo(left + r * 0.6f, bottom)
            cornerPath.quadTo(left, bottom, left, bottom - r * 0.6f)
            cornerPath.lineTo(left, bottom - arm)

            canvas.drawPath(cornerPath, cornerGlowPaint)
            canvas.drawPath(cornerPath, cornerPaint)

            // Sweeping laser line + trailing gradient wake
            val scanTop = top + dp(10f)
            val scanBottom = bottom - dp(10f)
            val y = scanTop + (scanBottom - scanTop) * progress
            val lineLeft = left + dp(10f)
            val lineRight = right - dp(10f)

            val trailH = dp(44f)
            if (movingDown) {
                val tTop = max(scanTop, y - trailH)
                trailPaint.shader = LinearGradient(
                    0f, tTop, 0f, y,
                    withAlpha(accentColor, 0),
                    withAlpha(accentColor, 100),
                    Shader.TileMode.CLAMP
                )
                canvas.drawRect(lineLeft, tTop, lineRight, y, trailPaint)
            } else {
                val tBot = kotlin.math.min(scanBottom, y + trailH)
                trailPaint.shader = LinearGradient(
                    0f, y, 0f, tBot,
                    withAlpha(accentColor, 100),
                    withAlpha(accentColor, 0),
                    Shader.TileMode.CLAMP
                )
                canvas.drawRect(lineLeft, y, lineRight, tBot, trailPaint)
            }

            // Tight edge-to-edge sci-fi pixel matrix with multi-frequency flickering squares
            val spanX = lineRight - lineLeft
            val cell = dp(10f)
            val cols = (spanX / cell).toInt().coerceAtLeast(1)
            val actualCell = spanX / cols
            val rows = 4
            for (rIdx in 0 until rows) {
                val cellTop = if (movingDown) y - (rIdx + 1) * actualCell else y + rIdx * actualCell
                val cellBot = cellTop + actualCell
                if (cellTop < scanTop || cellBot > scanBottom) continue
                val rowFade = 1f - (rIdx.toFloat() / rows.toFloat())
                for (cIdx in 0 until cols) {
                    val phase = (cIdx * 13 + rIdx * 29) % 17
                    val freq = 14f + (phase % 5) * 7f
                    val band = 0.5f + 0.5f * kotlin.math.sin((progress * freq + cIdx * 1.7f + rIdx * 2.3f).toDouble()).toFloat()
                    val alpha = (210f * rowFade * band * band).toInt().coerceIn(8, 225)
                    sparkPaint.color = withAlpha(accentColor, alpha)
                    val cx = lineLeft + cIdx * actualCell
                    canvas.drawRect(cx, cellTop, cx + actualCell, cellBot, sparkPaint)
                }
            }

            canvas.drawLine(lineLeft, y, lineRight, y, lineGlowPaint)
            canvas.drawLine(lineLeft, y, lineRight, y, lineCorePaint)
        }
    }
}
