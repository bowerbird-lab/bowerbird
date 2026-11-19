package com.bowerbird.mobile

import android.animation.Animator
import android.animation.AnimatorListenerAdapter
import android.animation.ValueAnimator
import android.annotation.SuppressLint
import android.content.Context
import android.graphics.BitmapShader
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Matrix
import android.graphics.Paint
import android.graphics.RectF
import android.graphics.Shader
import android.graphics.Typeface
import android.graphics.drawable.ColorDrawable
import android.graphics.drawable.GradientDrawable
import android.text.TextUtils
import android.util.TypedValue
import android.view.Gravity
import android.view.MotionEvent
import android.view.ScaleGestureDetector
import android.view.View
import android.view.ViewGroup
import android.view.ViewParent
import android.view.animation.DecelerateInterpolator
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout
import dev.oneo.nsgo.NsgoComponent
import dev.oneo.nsgo.NsgoView
import kotlin.math.abs
import kotlin.math.hypot

/**
 * Native 120fps gallery grid backed by dual-layer Android RecyclerViews
 * (GridLayoutManager for dense square grids + StaggeredGridLayoutManager for
 * aspect-ratio waterfall) plus a GPU-accelerated MorphOverlayView that
 * interpolates every visible tile's (x, y, width, height) in real time with
 * finger-span distance thresholds, matching Google Photos' seamless transitions.
 */
class GalleryGridComponent(
    private val context: Context,
    private val events: NsgoComponent.Events
) : NsgoComponent {

    data class GridEntry(
        val kind: String, // "header", "folder_header", "folder", or "item"
        val key: String,
        val title: String,
        val count: Int,
        val flatIndex: Int,
        val id: String,
        val thumbUrl: String,
        val width: Int,
        val height: Int,
        val badge: String,
        val shareMarked: Boolean,
        val selected: Boolean,
        val collapsed: Boolean = false,
        val thumbUrls: List<String> = emptyList()
    )

    data class LayoutConfig(
        val cols: Int,
        val isWaterfall: Boolean
    )

    /** Colours supplied by the Go theme; every tile, badge and header reads these. */
    private data class ThemeColors(
        val background: Int = Color.TRANSPARENT,
        val surface: Int = Color.WHITE,
        val surfaceAlt: Int = Color.parseColor("#F4F4F5"),
        val border: Int = Color.parseColor("#E4E4E7"),
        val text: Int = Color.parseColor("#18181B"),
        val textMuted: Int = Color.parseColor("#71717A"),
        val faint: Int = Color.parseColor("#9A9AA3"),
        val accent: Int = Color.parseColor("#2563EB"),
        val onAccent: Int = Color.WHITE,
        val badge: Int = Color.argb(158, 0, 0, 0),
        val onBadge: Int = Color.WHITE,
        val success: Int = Color.parseColor("#15803D"),
        val overlay: Int = Color.argb(89, 0, 0, 0)
    )

    private val density = context.resources.displayMetrics.density
    private val triggerSlopPx = TRIGGER_SLOP_DP * density
    private val stepDistancePx = STEP_DISTANCE_DP * density

    private var requestedCols = 3
    private var layoutMode = MODE_AUTO
    private var currentConfig = resolveConfig(layoutMode, requestedCols)

    private var isDark = false
    private var theme = ThemeColors()
    private var themeVersion = 0
    private var hasThemeColors = false
    private var lastScrollTopSeq = 0
    private var pendingRestorePos = -1
    private var pendingRestoreOffset = 0
    private var hasRestoredInitialScroll = false
    private var topInsetDp = 0
    private var bottomInsetDp = 24
    private var horizontalInsetDp = 8
    private var items: List<GridEntry> = emptyList()
    private var itemsVersion = 0
    private var emptyMessage = ""
    private var loadMoreTriggeredForSize = -1

    private val stableIds = HashMap<String, Long>(1024)
    private var nextStableId = 1L

    private val swipeRefresh = SwipeRefreshLayout(context)
    private val rootContainer = PinchHostFrameLayout(context)
    private val morphOverlay = MorphOverlayView(context)
    private val emptyView = TextView(context)

    private var primaryLayer = GridLayer(currentConfig)
    private var previewLayer = GridLayer(currentConfig)

    // Active pinch / morph state
    private var isPinching = false
    private var pinchBlockedUntilUp = false
    private var startSpanPx = 0f
    private var armSpanPx = 0f
    private var activeDir = 0 // -1 = spread/zoom-in (fewer cols), +1 = pinch/zoom-out (more cols)
    private var prevSignedDeltaPx = 0f
    private var smoothedVelocityPx = 0f
    private var targetConfig: LayoutConfig? = null
    private var currentProgress = 0f
    private var colRatio = 1f
    private var primaryAnchorX = 0f
    private var primaryAnchorY = 0f
    private var previewAnchorX = 0f
    private var previewAnchorY = 0f
    private var transitionAnimator: ValueAnimator? = null

    private val scaleDetector = ScaleGestureDetector(
        context,
        object : ScaleGestureDetector.SimpleOnScaleGestureListener() {
            override fun onScaleBegin(detector: ScaleGestureDetector): Boolean {
                if (items.isEmpty()) return false
                cancelTransitionAnimator()
                isPinching = true
                pinchBlockedUntilUp = true
                startSpanPx = detector.currentSpan
                armSpanPx = startSpanPx
                activeDir = 0
                prevSignedDeltaPx = 0f
                smoothedVelocityPx = 0f
                targetConfig = null
                currentProgress = 0f
                primaryAnchorX = detector.focusX
                primaryAnchorY = detector.focusY
                return true
            }

            override fun onScale(detector: ScaleGestureDetector): Boolean {
                if (!isPinching) return false
                val curSpan = detector.currentSpan
                if (!curSpan.isFinite() || curSpan <= 0f) return false

                if (activeDir == 0) {
                    val deltaPx = curSpan - startSpanPx
                    if (abs(deltaPx) < triggerSlopPx) {
                        return true
                    }
                    activeDir = if (deltaPx > 0f) -1 else 1
                    armSpanPx = curSpan
                    prevSignedDeltaPx = 0f
                    smoothedVelocityPx = 0f
                    prepareTransition(activeDir, detector.focusX, detector.focusY)
                    return true
                }

                val signedDeltaPx = if (activeDir == -1) (curSpan - armSpanPx) else (armSpanPx - curSpan)
                val stepDelta = signedDeltaPx - prevSignedDeltaPx
                prevSignedDeltaPx = signedDeltaPx
                smoothedVelocityPx = smoothedVelocityPx * 0.5f + stepDelta * 0.5f

                val target = targetConfig
                if (target == null) {
                    // Boundary rubber-band feedback; flip immediately if user reverses direction
                    if (signedDeltaPx < -triggerSlopPx * 0.6f) {
                        resetLayerTransforms(primaryLayer)
                        activeDir = -activeDir
                        armSpanPx = curSpan
                        prevSignedDeltaPx = 0f
                        prepareTransition(activeDir, detector.focusX, detector.focusY)
                    } else {
                        val norm = (signedDeltaPx / stepDistancePx).coerceIn(0f, 1.2f)
                        val s = if (activeDir == -1) 1f + norm * 0.055f else 1f - norm * 0.055f
                        primaryLayer.rv.pivotX = detector.focusX
                        primaryLayer.rv.pivotY = detector.focusY
                        primaryLayer.rv.scaleX = s
                        primaryLayer.rv.scaleY = s
                    }
                    return true
                }

                // Reversed past start point -> disengage current target cleanly
                if (signedDeltaPx < -dp(6).toFloat()) {
                    cancelMorphToPrimary()
                    startSpanPx = curSpan
                    armSpanPx = curSpan
                    activeDir = 0
                    prevSignedDeltaPx = 0f
                    return true
                }

                val progress = (signedDeltaPx / stepDistancePx).coerceIn(0f, 1f)
                currentProgress = progress
                applyTransitionFrame(progress)

                // Continuous multi-step pinch: promote target and chain into next step seamlessly
                if (progress >= 1f) {
                    val nextStep = resolveStepConfig(layoutMode, target, activeDir)
                    if (nextStep != null) {
                        promotePreviewToPrimary(target)
                        armSpanPx = curSpan
                        prevSignedDeltaPx = 0f
                        currentProgress = 0f
                        prepareTransition(activeDir, detector.focusX, detector.focusY)
                    }
                }
                return true
            }

            override fun onScaleEnd(detector: ScaleGestureDetector) {
                isPinching = false
                val target = targetConfig
                if (target == null) {
                    animateBoundaryRelease()
                    return
                }
                val flickCommit = smoothedVelocityPx > dp(2).toFloat()
                val flickCancel = smoothedVelocityPx < -dp(2).toFloat()
                val shouldCommit = when {
                    flickCancel && currentProgress < 0.75f -> false
                    flickCommit && currentProgress >= 0.18f -> true
                    else -> currentProgress >= 0.42f
                }
                animateTransitionSettle(shouldCommit, target)
            }
        }
    ).apply {
        isQuickScaleEnabled = false
    }

    init {
        previewLayer.rv.visibility = View.INVISIBLE
        morphOverlay.visibility = View.GONE

        emptyView.apply {
            gravity = Gravity.CENTER
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 14f)
            setPadding(dp(24), dp(48), dp(24), dp(48))
            visibility = View.GONE
        }

        rootContainer.addView(
            primaryLayer.rv,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )
        rootContainer.addView(
            previewLayer.rv,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )
        rootContainer.addView(
            morphOverlay,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )
        rootContainer.addView(
            emptyView,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        applyInsets()

        swipeRefresh.apply {
            isEnabled = false
            addView(
                rootContainer,
                ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT
                )
            )
        }
    }

    override fun view(): View = swipeRefresh

    override fun destroy() {
        cancelTransitionAnimator()
        morphOverlay.clearScene()
    }

    override fun setProp(name: String, value: Any?) {
        when (name) {
            "cols" -> {
                val next = ((value as? Number)?.toInt() ?: 3).coerceIn(2, 5)
                if (next != requestedCols) {
                    requestedCols = next
                    syncExternalLayoutConfig()
                }
            }
            "layoutMode" -> {
                val next = normalizeLayoutMode(value as? String)
                if (next != layoutMode) {
                    layoutMode = next
                    syncExternalLayoutConfig()
                }
            }
            "isDark" -> {
                val dark = value == true
                if (dark != isDark) {
                    isDark = dark
                    if (!hasThemeColors) {
                        theme = if (isDark) {
                            ThemeColors(
                                background = Color.parseColor("#141416"),
                                surface = Color.parseColor("#232327"),
                                surfaceAlt = Color.parseColor("#2C2C31"),
                                border = Color.parseColor("#3F3F46"),
                                text = Color.parseColor("#F4F4F5"),
                                textMuted = Color.parseColor("#A1A1AA"),
                                faint = Color.parseColor("#71717A"),
                                accent = Color.parseColor("#6EA8FF"),
                                onAccent = Color.parseColor("#0B1220"),
                                badge = Color.argb(158, 0, 0, 0),
                                onBadge = Color.WHITE,
                                success = Color.parseColor("#16A34A"),
                                overlay = Color.argb(89, 0, 0, 0)
                            )
                        } else {
                            ThemeColors()
                        }
                        applyTheme()
                    } else {
                        primaryLayer.adapter.notifyDataSetChanged()
                    }
                }
            }
            "theme" -> {
                val map = value as? Map<*, *> ?: return
                val next = ThemeColors(
                    background = colorOf(map, "background", theme.background),
                    surface = colorOf(map, "surface", theme.surface),
                    surfaceAlt = colorOf(map, "surfaceAlt", colorOf(map, "inset", theme.surfaceAlt)),
                    border = colorOf(map, "border", theme.border),
                    text = colorOf(map, "text", theme.text),
                    textMuted = colorOf(map, "textMuted", theme.textMuted),
                    faint = colorOf(map, "faint", theme.faint),
                    accent = colorOf(map, "accent", colorOf(map, "accentFill", theme.accent)),
                    onAccent = colorOf(map, "onAccent", theme.onAccent),
                    badge = colorOf(map, "badge", theme.badge),
                    onBadge = colorOf(map, "onBadge", theme.onBadge),
                    success = colorOf(map, "success", theme.success),
                    overlay = colorOf(map, "overlay", theme.overlay)
                )
                if (next != theme || !hasThemeColors) {
                    theme = next
                    hasThemeColors = true
                    applyTheme()
                }
            }
            "topInsetDp" -> {
                val next = ((value as? Number)?.toInt() ?: 0).coerceAtLeast(0)
                if (next != topInsetDp) {
                    topInsetDp = next
                    applyInsets()
                }
            }
            "horizontalInsetDp" -> {
                val next = ((value as? Number)?.toInt() ?: 8).coerceAtLeast(0)
                if (next != horizontalInsetDp) {
                    horizontalInsetDp = next
                    applyInsets()
                }
            }
            "bottomInsetDp" -> {
                val next = ((value as? Number)?.toInt() ?: 24).coerceAtLeast(16)
                if (next != bottomInsetDp) {
                    bottomInsetDp = next
                    applyInsets()
                }
            }
            "emptyText" -> {
                emptyMessage = (value as? String).orEmpty()
                emptyView.text = emptyMessage
                updateEmptyVisibility()
            }
            "scrollTopSeq" -> {
                val seq = (value as? Number)?.toInt() ?: 0
                if (seq != lastScrollTopSeq) {
                    lastScrollTopSeq = seq
                    pendingRestorePos = -1
                    pendingRestoreOffset = 0
                    cancelTransitionAndReset()
                    primaryLayer.rv.scrollToPosition(0)
                }
            }
            "scrollPos" -> {
                if (!hasRestoredInitialScroll) {
                    pendingRestorePos = (value as? Number)?.toInt() ?: -1
                }
            }
            "scrollOffset" -> {
                if (!hasRestoredInitialScroll) {
                    pendingRestoreOffset = (value as? Number)?.toInt() ?: 0
                }
            }
            "rows" -> {
                val rawList = value as? List<*> ?: emptyList<Any>()
                val parsed = ArrayList<GridEntry>(rawList.size)
                for (raw in rawList) {
                    val m = raw as? Map<*, *> ?: continue
                    val rawThumbs = m["thumbUrls"] as? List<*>
                    val thumbUrls = if (rawThumbs != null) {
                        rawThumbs.mapNotNull { it as? String }.filter { it.isNotEmpty() }.take(4)
                    } else {
                        emptyList()
                    }
                    parsed.add(
                        GridEntry(
                            kind = (m["kind"] as? String).orEmpty(),
                            key = (m["key"] as? String).orEmpty(),
                            title = (m["title"] as? String).orEmpty(),
                            count = (m["count"] as? Number)?.toInt() ?: 0,
                            flatIndex = (m["flatIndex"] as? Number)?.toInt() ?: 0,
                            id = (m["id"] as? String).orEmpty(),
                            thumbUrl = (m["thumbUrl"] as? String).orEmpty(),
                            width = (m["width"] as? Number)?.toInt() ?: 0,
                            height = (m["height"] as? Number)?.toInt() ?: 0,
                            badge = (m["badge"] as? String).orEmpty(),
                            shareMarked = m["shareMarked"] == true,
                            selected = m["selected"] == true,
                            collapsed = m["collapsed"] == true,
                            thumbUrls = thumbUrls
                        )
                    )
                }
                val old = items
                if (parsed == old) {
                    updateEmptyVisibility()
                    return
                }
                if (parsed.size != old.size) {
                    loadMoreTriggeredForSize = -1
                }
                items = parsed
                itemsVersion++
                val diff = DiffUtil.calculateDiff(object : DiffUtil.Callback() {
                    override fun getOldListSize(): Int = old.size
                    override fun getNewListSize(): Int = parsed.size
                    override fun areItemsTheSame(o: Int, n: Int): Boolean = old[o].key == parsed[n].key
                    override fun areContentsTheSame(o: Int, n: Int): Boolean = old[o] == parsed[n]
                }, false)
                diff.dispatchUpdatesTo(primaryLayer.adapter)
                if (previewLayer.rv.visibility == View.VISIBLE || morphOverlay.visibility == View.VISIBLE) {
                    diff.dispatchUpdatesTo(previewLayer.adapter)
                }
                if (!hasRestoredInitialScroll && pendingRestorePos >= 0 && items.isNotEmpty()) {
                    hasRestoredInitialScroll = true
                    val targetPos = pendingRestorePos.coerceIn(0, items.lastIndex)
                    val targetOffset = pendingRestoreOffset
                    pendingRestorePos = -1
                    pendingRestoreOffset = 0
                    primaryLayer.rv.post {
                        primaryLayer.scrollToPositionWithOffset(targetPos, targetOffset)
                    }
                }
                updateEmptyVisibility()
            }
        }
    }

    private fun syncExternalLayoutConfig() {
        val next = resolveConfig(layoutMode, requestedCols)
        if (next == currentConfig) return
        if (isPinching) return

        cancelTransitionAnimator()
        if (items.isNotEmpty() && rootContainer.width > 0 && rootContainer.height > 0) {
            val cx = rootContainer.width * 0.5f
            val cy = rootContainer.height * 0.45f
            val dir = if (next.cols <= currentConfig.cols) -1 else 1
            activeDir = dir
            prepareTransitionWithTarget(next, cx, cy)
            currentProgress = 0f
            applyTransitionFrame(0f)
            animateTransitionSettle(commit = true, target = next)
        } else {
            currentConfig = next
            primaryLayer.applyConfig(next, preserveAnchor = true)
        }
    }

    private fun prepareTransition(dir: Int, focusX: Float, focusY: Float) {
        val target = resolveStepConfig(layoutMode, currentConfig, dir)
        if (target == null) {
            targetConfig = null
            return
        }
        prepareTransitionWithTarget(target, focusX, focusY)
    }

    private fun prepareTransitionWithTarget(target: LayoutConfig, focusX: Float, focusY: Float) {
        targetConfig = target
        val w = rootContainer.width
        val h = rootContainer.height
        val curColW = colWidthPx(currentConfig).toFloat()
        val tgtColW = colWidthPx(target).toFloat()
        colRatio = (tgtColW / curColW).coerceIn(0.45f, 2.2f)

        val anchorView = findFocalChild(primaryLayer.rv, focusX, focusY)
        val anchorPos = if (anchorView != null) {
            primaryLayer.rv.getChildAdapterPosition(anchorView).takeIf { it in items.indices }
                ?: primaryLayer.firstVisiblePosition().coerceIn(0, items.lastIndex)
        } else {
            primaryLayer.firstVisiblePosition().coerceIn(0, items.lastIndex)
        }

        if (anchorView != null) {
            primaryAnchorX = (anchorView.left + anchorView.right) * 0.5f
            primaryAnchorY = (anchorView.top + anchorView.bottom) * 0.5f
        } else {
            primaryAnchorX = focusX
            primaryAnchorY = focusY
        }

        // Snapshot all currently visible rects in primaryLayer before switching layers
        val startItems = collectVisibleRects(primaryLayer.rv)

        // Synchronously lay out previewLayer at target configuration aligned around anchorPos
        previewLayer.configureForTransition(target)
        val targetTileH = tileHeightFor(items[anchorPos], previewLayer.parentWidthPx(target), target)
        val desiredOffset = (primaryAnchorY - targetTileH * 0.5f - previewLayer.rv.paddingTop).toInt()
        previewLayer.scrollToPositionWithOffset(anchorPos, desiredOffset)

        if (w > 0 && h > 0) {
            previewLayer.rv.requestLayout()
            previewLayer.rv.measure(
                View.MeasureSpec.makeMeasureSpec(w, View.MeasureSpec.EXACTLY),
                View.MeasureSpec.makeMeasureSpec(h, View.MeasureSpec.EXACTLY)
            )
            previewLayer.rv.layout(0, 0, w, h)
        }

        val previewChild = previewLayer.rv.findViewHolderForAdapterPosition(anchorPos)?.itemView
        if (previewChild != null) {
            previewAnchorX = (previewChild.left + previewChild.right) * 0.5f
            previewAnchorY = (previewChild.top + previewChild.bottom) * 0.5f
        } else {
            previewAnchorX = primaryAnchorX
            previewAnchorY = primaryAnchorY
        }

        val endItems = collectVisibleRects(previewLayer.rv)
        buildMorphScene(startItems, endItems)

        primaryLayer.rv.visibility = View.INVISIBLE
        previewLayer.rv.visibility = View.INVISIBLE
        morphOverlay.visibility = View.VISIBLE
        morphOverlay.bringToFront()
        emptyView.bringToFront()
        applyTransitionFrame(0f)
    }

    private fun collectVisibleRects(rv: RecyclerView): Map<String, Pair<GridEntry, RectF>> {
        val map = LinkedHashMap<String, Pair<GridEntry, RectF>>(rv.childCount * 2)
        for (i in 0 until rv.childCount) {
            val child = rv.getChildAt(i) ?: continue
            val pos = rv.getChildAdapterPosition(child)
            if (pos !in items.indices) continue
            val entry = items[pos]
            map[entry.key] = entry to RectF(
                child.left.toFloat(),
                child.top.toFloat(),
                child.right.toFloat(),
                child.bottom.toFloat()
            )
        }
        return map
    }

    private fun buildMorphScene(
        startMap: Map<String, Pair<GridEntry, RectF>>,
        endMap: Map<String, Pair<GridEntry, RectF>>
    ) {
        val morphTiles = ArrayList<MorphTile>(maxOf(startMap.size, endMap.size) + 8)
        val morphHeaders = ArrayList<MorphHeader>(8)
        val invRatio = if (colRatio > 0.001f) 1f / colRatio else 1f
        val shiftX = previewAnchorX - primaryAnchorX
        val shiftY = previewAnchorY - primaryAnchorY

        // 1. Entries present in startMap (either matched in endMap, or exiting viewport)
        for ((key, startPair) in startMap) {
            val (entry, fromRect) = startPair
            val endPair = endMap[key]
            if (entry.kind == "header" || entry.kind == "folder_header") {
                val fromH = fromRect.height()
                val toTop: Float
                val toAlpha: Float
                if (endPair != null) {
                    toTop = endPair.second.top
                    toAlpha = 1f
                } else {
                    toTop = scaleAroundY(fromRect.top, primaryAnchorY, colRatio) + shiftY
                    toAlpha = 0f
                }
                morphHeaders.add(
                    MorphHeader(
                        title = entry.title,
                        countText = if (entry.kind == "folder_header") "" else "${entry.count} 项",
                        left = fromRect.left,
                        right = fromRect.right,
                        height = fromH,
                        fromTop = fromRect.top,
                        toTop = toTop,
                        fromAlpha = 1f,
                        toAlpha = toAlpha
                    )
                )
            } else {
                val toRect: RectF
                val toAlpha: Float
                if (endPair != null) {
                    toRect = endPair.second
                    toAlpha = 1f
                } else {
                    toRect = scaleRectAround(fromRect, primaryAnchorX, primaryAnchorY, colRatio, shiftX, shiftY)
                    toAlpha = 0f
                }
                morphTiles.add(makeMorphTile(entry, fromRect, toRect, 1f, toAlpha))
            }
        }

        // 2. Entries only in endMap (entering viewport)
        for ((key, endPair) in endMap) {
            if (startMap.containsKey(key)) continue
            val (entry, toRect) = endPair
            if (entry.kind == "header" || entry.kind == "folder_header") {
                val fromTop = scaleAroundY(toRect.top, previewAnchorY, invRatio) - shiftY
                morphHeaders.add(
                    MorphHeader(
                        title = entry.title,
                        countText = if (entry.kind == "folder_header") "" else "${entry.count} 项",
                        left = toRect.left,
                        right = toRect.right,
                        height = toRect.height(),
                        fromTop = fromTop,
                        toTop = toRect.top,
                        fromAlpha = 0f,
                        toAlpha = 1f
                    )
                )
            } else {
                val fromRect = scaleRectAround(toRect, previewAnchorX, previewAnchorY, invRatio, -shiftX, -shiftY)
                morphTiles.add(makeMorphTile(entry, fromRect, toRect, 0f, 1f))
            }
        }

        morphOverlay.setScene(morphTiles, morphHeaders)
    }

    private fun makeMorphTile(
        entry: GridEntry,
        fromRect: RectF,
        toRect: RectF,
        fromAlpha: Float,
        toAlpha: Float
    ): MorphTile {
        val url = if (entry.kind == "folder") entry.thumbUrls.firstOrNull().orEmpty() else entry.thumbUrl
        val bmp = ThumbLoader.peekMemory(url)
        val shader = bmp?.let { BitmapShader(it, Shader.TileMode.CLAMP, Shader.TileMode.CLAMP) }
        return MorphTile(
            from = fromRect,
            to = toRect,
            fromAlpha = fromAlpha,
            toAlpha = toAlpha,
            shader = shader,
            bmpW = bmp?.width ?: 0,
            bmpH = bmp?.height ?: 0,
            badge = entry.badge,
            shareMarked = entry.shareMarked,
            selected = entry.selected
        )
    }

    private fun scaleAroundY(y: Float, pivotY: Float, scale: Float): Float =
        pivotY + (y - pivotY) * scale

    private fun scaleRectAround(
        r: RectF,
        pivotX: Float,
        pivotY: Float,
        scale: Float,
        dx: Float,
        dy: Float
    ): RectF {
        return RectF(
            pivotX + (r.left - pivotX) * scale + dx,
            pivotY + (r.top - pivotY) * scale + dy,
            pivotX + (r.right - pivotX) * scale + dx,
            pivotY + (r.bottom - pivotY) * scale + dy
        )
    }

    private fun applyTransitionFrame(progress: Float) {
        morphOverlay.setProgress(progress.coerceIn(0f, 1f))
    }

    private fun animateTransitionSettle(commit: Boolean, target: LayoutConfig) {
        cancelTransitionAnimator()
        val end = if (commit) 1f else 0f
        val start = currentProgress
        if (abs(end - start) < 0.005f) {
            finishTransitionSettle(commit, target)
            return
        }
        val duration = (210L * abs(end - start)).toLong().coerceIn(100L, 220L)
        transitionAnimator = ValueAnimator.ofFloat(start, end).apply {
            this.duration = duration
            interpolator = DecelerateInterpolator(2.0f)
            addUpdateListener { anim ->
                val p = anim.animatedValue as Float
                currentProgress = p
                applyTransitionFrame(p)
            }
            addListener(object : AnimatorListenerAdapter() {
                private var cancelled = false
                override fun onAnimationCancel(animation: Animator) {
                    cancelled = true
                }
                override fun onAnimationEnd(animation: Animator) {
                    transitionAnimator = null
                    if (!cancelled) {
                        finishTransitionSettle(commit, target)
                    }
                }
            })
            start()
        }
    }

    private fun finishTransitionSettle(commit: Boolean, target: LayoutConfig) {
        if (commit) {
            promotePreviewToPrimary(target)
            if (requestedCols != target.cols) {
                requestedCols = target.cols
                events.emit("pinchCols", mapOf("cols" to target.cols))
            }
        } else {
            cancelMorphToPrimary()
        }
        targetConfig = null
        activeDir = 0
        currentProgress = 0f
    }

    private fun promotePreviewToPrimary(target: LayoutConfig) {
        val oldPrimary = primaryLayer
        primaryLayer = previewLayer
        previewLayer = oldPrimary

        currentConfig = target
        morphOverlay.visibility = View.GONE
        morphOverlay.clearScene()

        resetLayerTransforms(primaryLayer)
        primaryLayer.rv.visibility = View.VISIBLE
        primaryLayer.rv.bringToFront()
        emptyView.bringToFront()

        resetLayerTransforms(previewLayer)
        previewLayer.rv.visibility = View.INVISIBLE
    }

    private fun cancelMorphToPrimary() {
        morphOverlay.visibility = View.GONE
        morphOverlay.clearScene()
        resetLayerTransforms(primaryLayer)
        primaryLayer.rv.visibility = View.VISIBLE
        primaryLayer.rv.bringToFront()
        emptyView.bringToFront()
        resetLayerTransforms(previewLayer)
        previewLayer.rv.visibility = View.INVISIBLE
        targetConfig = null
    }

    private fun animateBoundaryRelease() {
        cancelTransitionAnimator()
        val sx = primaryLayer.rv.scaleX
        if (abs(sx - 1f) < 0.005f) {
            resetLayerTransforms(primaryLayer)
            activeDir = 0
            return
        }
        transitionAnimator = ValueAnimator.ofFloat(sx, 1f).apply {
            duration = 160L
            interpolator = DecelerateInterpolator(1.8f)
            addUpdateListener { anim ->
                val s = anim.animatedValue as Float
                primaryLayer.rv.scaleX = s
                primaryLayer.rv.scaleY = s
            }
            addListener(object : AnimatorListenerAdapter() {
                override fun onAnimationEnd(animation: Animator) {
                    transitionAnimator = null
                    resetLayerTransforms(primaryLayer)
                    activeDir = 0
                }
            })
            start()
        }
    }

    private fun cancelTransitionAnimator() {
        transitionAnimator?.cancel()
        transitionAnimator = null
    }

    private fun cancelTransitionAndReset() {
        cancelTransitionAnimator()
        cancelMorphToPrimary()
        isPinching = false
        activeDir = 0
        currentProgress = 0f
    }

    private fun resetLayerTransforms(layer: GridLayer) {
        layer.rv.apply {
            scaleX = 1f
            scaleY = 1f
            translationX = 0f
            translationY = 0f
            alpha = 1f
        }
    }

    private fun findFocalChild(rv: RecyclerView, focusX: Float, focusY: Float): View? {
        rv.findChildViewUnder(focusX, focusY)?.let { return it }
        var best: View? = null
        var bestDist = Float.MAX_VALUE
        for (i in 0 until rv.childCount) {
            val child = rv.getChildAt(i) ?: continue
            val cx = (child.left + child.right) * 0.5f
            val cy = (child.top + child.bottom) * 0.5f
            val d = hypot(cx - focusX, cy - focusY)
            if (d < bestDist) {
                bestDist = d
                best = child
            }
        }
        return best
    }

    private fun colorOf(map: Map<*, *>, key: String, fallback: Int): Int {
        return try {
            val raw = map[key] as? String ?: return fallback
            Color.parseColor(raw)
        } catch (_: IllegalArgumentException) {
            fallback
        }
    }

    private fun applyTheme() {
        themeVersion++
        swipeRefresh.setBackgroundColor(theme.background)
        primaryLayer.rv.setBackgroundColor(theme.background)
        previewLayer.rv.setBackgroundColor(theme.background)
        morphOverlay.syncTheme()
        emptyView.setTextColor(theme.textMuted)
        swipeRefresh.setProgressBackgroundColorSchemeColor(theme.surface)
        swipeRefresh.setColorSchemeColors(theme.accent)
        primaryLayer.adapter.notifyDataSetChanged()
        if (previewLayer.rv.visibility == View.VISIBLE) {
            previewLayer.adapter.notifyDataSetChanged()
        }
    }

    private fun applyInsets() {
        val pad = dp(horizontalInsetDp)
        val top = dp(topInsetDp)
        val bottom = dp(bottomInsetDp)
        primaryLayer.rv.setPadding(pad, top, pad, bottom)
        previewLayer.rv.setPadding(pad, top, pad, bottom)
        swipeRefresh.setProgressViewOffset(false, top, top + dp(56))
    }

    private fun updateEmptyVisibility() {
        val showEmpty = items.isEmpty() && emptyMessage.isNotBlank()
        emptyView.visibility = if (showEmpty) View.VISIBLE else View.GONE
        emptyView.setTextColor(theme.textMuted)
    }

    private fun dp(v: Int): Int = (v * density).toInt()

    private fun stableIdFor(key: String): Long = stableIds.getOrPut(key) { nextStableId++ }

    private fun colWidthPx(config: LayoutConfig): Int {
        val parentW = primaryLayer.parentWidthPx(config)
        return maxOf((parentW - dp(6) * config.cols) / config.cols, dp(48))
    }

    private fun tileHeightFor(entry: GridEntry, parentWidthPx: Int, config: LayoutConfig): Int {
        if (entry.kind == "header" || entry.kind == "folder_header") return dp(38)
        val colW = maxOf((parentWidthPx - dp(6) * config.cols) / config.cols, dp(48))
        if (entry.kind == "folder") return colW + dp(28)
        if (!config.isWaterfall || entry.width <= 0 || entry.height <= 0) {
            return colW
        }
        val ratio = (entry.width.toFloat() / entry.height.toFloat()).coerceIn(0.48f, 2.1f)
        return (colW / ratio).toInt().coerceIn(dp(64), (colW * 2.1f).toInt())
    }

    private data class MorphTile(
        val from: RectF,
        val to: RectF,
        val fromAlpha: Float,
        val toAlpha: Float,
        val shader: BitmapShader?,
        val bmpW: Int,
        val bmpH: Int,
        val badge: String,
        val shareMarked: Boolean,
        val selected: Boolean
    )

    private data class MorphHeader(
        val title: String,
        val countText: String,
        val left: Float,
        val right: Float,
        val height: Float,
        val fromTop: Float,
        val toTop: Float,
        val fromAlpha: Float,
        val toAlpha: Float
    )

    private inner class MorphOverlayView(context: Context) : View(context) {
        private var tiles: List<MorphTile> = emptyList()
        private var headers: List<MorphHeader> = emptyList()
        private var progress = 0f

        private val tileRadius = dp(8).toFloat()
        private val badgeRadius = dp(4).toFloat()
        private val badgeMargin = dp(5).toFloat()
        private val badgePadX = dp(5).toFloat()
        private val badgePadY = dp(1).toFloat()
        private val headerPadX = dp(2).toFloat()

        private val tmpRect = RectF()
        private val badgeRect = RectF()
        private val tmpMatrix = Matrix()

        private val bgPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
        private val shaderPaint = Paint(Paint.ANTI_ALIAS_FLAG or Paint.FILTER_BITMAP_FLAG)
        private val borderPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = 1f
        }
        private val overlayPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
        private val badgeBgPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
        private val badgeTextPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            textSize = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, 10f, resources.displayMetrics)
        }
        private val shareTextPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            textSize = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, 9f, resources.displayMetrics)
        }
        private val headerTitlePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            textSize = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, 13f, resources.displayMetrics)
            typeface = Typeface.DEFAULT_BOLD
        }
        private val headerCountPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            textSize = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_SP, 12f, resources.displayMetrics)
            textAlign = Paint.Align.RIGHT
        }

        init {
            syncTheme()
        }

        fun syncTheme() {
            bgPaint.color = theme.surfaceAlt
            borderPaint.color = theme.border
            overlayPaint.color = theme.overlay
            badgeTextPaint.color = theme.onBadge
            shareTextPaint.color = theme.onBadge
            headerTitlePaint.color = theme.text
            headerCountPaint.color = theme.textMuted
            invalidate()
        }

        fun setScene(morphTiles: List<MorphTile>, morphHeaders: List<MorphHeader>) {
            tiles = morphTiles
            headers = morphHeaders
            invalidate()
        }

        fun clearScene() {
            tiles = emptyList()
            headers = emptyList()
            progress = 0f
        }

        fun setProgress(p: Float) {
            progress = p
            invalidate()
        }

        override fun onDraw(canvas: Canvas) {
            if (Color.alpha(theme.background) > 0) {
                canvas.drawColor(theme.background)
            }
            val p = progress

            for (i in 0 until tiles.size) {
                val t = tiles[i]
                val a = lerp(t.fromAlpha, t.toAlpha, p).coerceIn(0f, 1f)
                if (a <= 0.01f) continue

                tmpRect.set(
                    lerp(t.from.left, t.to.left, p),
                    lerp(t.from.top, t.to.top, p),
                    lerp(t.from.right, t.to.right, p),
                    lerp(t.from.bottom, t.to.bottom, p)
                )
                val dw = tmpRect.width()
                val dh = tmpRect.height()
                if (dw <= 1f || dh <= 1f) continue

                val alphaInt = (a * 255f).toInt()
                bgPaint.alpha = alphaInt
                canvas.drawRoundRect(tmpRect, tileRadius, tileRadius, bgPaint)

                val shader = t.shader
                if (shader != null && t.bmpW > 0 && t.bmpH > 0) {
                    val scale = maxOf(dw / t.bmpW, dh / t.bmpH)
                    val dx = tmpRect.left + (dw - t.bmpW * scale) * 0.5f
                    val dy = tmpRect.top + (dh - t.bmpH * scale) * 0.5f
                    tmpMatrix.setScale(scale, scale)
                    tmpMatrix.postTranslate(dx, dy)
                    shader.setLocalMatrix(tmpMatrix)
                    shaderPaint.shader = shader
                    shaderPaint.alpha = alphaInt
                    canvas.drawRoundRect(tmpRect, tileRadius, tileRadius, shaderPaint)
                }

                if (t.selected) {
                    overlayPaint.alpha = (a * Color.alpha(theme.overlay)).toInt()
                    canvas.drawRoundRect(tmpRect, tileRadius, tileRadius, overlayPaint)
                }

                borderPaint.alpha = (a * Color.alpha(theme.border)).toInt()
                canvas.drawRoundRect(tmpRect, tileRadius, tileRadius, borderPaint)

                if (t.badge.isNotEmpty() && dw > dp(44) && dh > dp(44)) {
                    drawPill(canvas, t.badge, badgeTextPaint, theme.badge, a, topRight = true)
                }
                if (t.shareMarked && dw > dp(44) && dh > dp(44)) {
                    drawPill(canvas, "分享", shareTextPaint, theme.success, a, topRight = false)
                }
            }

            for (i in 0 until headers.size) {
                val h = headers[i]
                val a = lerp(h.fromAlpha, h.toAlpha, p).coerceIn(0f, 1f)
                if (a <= 0.01f) continue
                val top = lerp(h.fromTop, h.toTop, p)
                val centerY = top + h.height * 0.5f + dp(3)
                headerTitlePaint.alpha = (a * Color.alpha(theme.text)).toInt()
                headerCountPaint.alpha = (a * Color.alpha(theme.textMuted)).toInt()
                val titleFm = headerTitlePaint.fontMetrics
                val titleBase = centerY - (titleFm.ascent + titleFm.descent) * 0.5f
                canvas.drawText(h.title, h.left + headerPadX, titleBase, headerTitlePaint)
                val countFm = headerCountPaint.fontMetrics
                val countBase = centerY - (countFm.ascent + countFm.descent) * 0.5f
                canvas.drawText(h.countText, h.right - headerPadX, countBase, headerCountPaint)
            }
        }

        private fun drawPill(
            canvas: Canvas,
            label: String,
            textPaint: Paint,
            bgColor: Int,
            alpha: Float,
            topRight: Boolean
        ) {
            val textW = textPaint.measureText(label)
            val fm = textPaint.fontMetrics
            val textH = fm.descent - fm.ascent
            val pillW = textW + badgePadX * 2f
            val pillH = textH + badgePadY * 2f
            if (topRight) {
                val right = tmpRect.right - badgeMargin
                val top = tmpRect.top + badgeMargin
                badgeRect.set(right - pillW, top, right, top + pillH)
            } else {
                val left = tmpRect.left + badgeMargin
                val bottom = tmpRect.bottom - badgeMargin
                badgeRect.set(left, bottom - pillH, left + pillW, bottom)
            }
            badgeBgPaint.color = bgColor
            badgeBgPaint.alpha = (alpha * Color.alpha(bgColor)).toInt()
            canvas.drawRoundRect(badgeRect, badgeRadius, badgeRadius, badgeBgPaint)
            textPaint.alpha = (alpha * Color.alpha(theme.onBadge)).toInt()
            val baseY = badgeRect.centerY() - (fm.ascent + fm.descent) * 0.5f
            canvas.drawText(label, badgeRect.left + badgePadX, baseY, textPaint)
        }

        private fun lerp(a: Float, b: Float, t: Float): Float = a + (b - a) * t
    }

    private inner class PinchHostFrameLayout(context: Context) : FrameLayout(context) {
        override fun dispatchTouchEvent(ev: MotionEvent): Boolean {
            if (ev.pointerCount >= 2 || isPinching || transitionAnimator?.isRunning == true) {
                parent?.requestDisallowInterceptTouchEvent(true)
            }
            scaleDetector.onTouchEvent(ev)
            val result = super.dispatchTouchEvent(ev)
            if (ev.actionMasked == MotionEvent.ACTION_UP || ev.actionMasked == MotionEvent.ACTION_CANCEL) {
                pinchBlockedUntilUp = false
            }
            return result
        }

        override fun onInterceptTouchEvent(ev: MotionEvent): Boolean {
            return isPinching || pinchBlockedUntilUp || ev.pointerCount >= 2 || (transitionAnimator?.isRunning == true)
        }

        @SuppressLint("ClickableViewAccessibility")
        override fun onTouchEvent(ev: MotionEvent): Boolean {
            return isPinching || pinchBlockedUntilUp || ev.pointerCount >= 2 || super.onTouchEvent(ev)
        }
    }

    private inner class GridLayer(initialConfig: LayoutConfig) {
        var config: LayoutConfig = initialConfig
            private set

        val adapter = GridAdapter(this)
        private var lm: RecyclerView.LayoutManager = createLayoutManager(initialConfig)

        val rv: RecyclerView = RecyclerView(context).apply {
            layoutManager = lm
            adapter = this@GridLayer.adapter
            setHasFixedSize(false)
            itemAnimator = null
            setItemViewCacheSize(24)
            clipToPadding = false
            setBackgroundColor(theme.background)
            addOnScrollListener(object : RecyclerView.OnScrollListener() {
                override fun onScrolled(recyclerView: RecyclerView, dx: Int, dy: Int) {
                    if (this@GridLayer !== primaryLayer) return
                    if (dy <= 0) return
                    val total = items.size
                    if (total == 0) return
                    val lastVisible = lastVisiblePosition()
                    if (lastVisible >= total - config.cols * 4 && loadMoreTriggeredForSize != total) {
                        loadMoreTriggeredForSize = total
                        events.emit("loadMore", null)
                    }
                }

                override fun onScrollStateChanged(recyclerView: RecyclerView, newState: Int) {
                    if (this@GridLayer !== primaryLayer) return
                    if (newState != RecyclerView.SCROLL_STATE_IDLE) return
                    val pos = firstVisiblePosition()
                    if (pos < 0) return
                    val child = lm.findViewByPosition(pos)
                    val offsetPx = if (child != null) child.top - rv.paddingTop else 0
                    events.emit("scrollState", mapOf("pos" to pos, "offset" to offsetPx))
                }
            })
        }

        private fun createLayoutManager(cfg: LayoutConfig): RecyclerView.LayoutManager {
            if (cfg.isWaterfall) {
                return WaterfallLayoutManager(cfg.cols)
            }
            return GridLayoutManager(context, cfg.cols).apply {
                spanSizeLookup = object : GridLayoutManager.SpanSizeLookup() {
                    override fun getSpanSize(position: Int): Int {
                        val item = items.getOrNull(position) ?: return 1
                        return if (item.kind == "header" || item.kind == "folder_header") cfg.cols else 1
                    }
                }
            }
        }

        fun applyConfig(next: LayoutConfig, preserveAnchor: Boolean) {
            val anchor = if (preserveAnchor) firstVisiblePosition() else RecyclerView.NO_POSITION
            configureForTransition(next)
            if (anchor >= 0 && anchor < items.size) {
                scrollToPositionWithOffset(anchor, 0)
            }
        }

        fun configureForTransition(next: LayoutConfig) {
            val prev = config
            config = next
            val typeChanged = prev.isWaterfall != next.isWaterfall
            if (typeChanged) {
                rv.recycledViewPool.clear()
                lm = createLayoutManager(next)
                rv.layoutManager = lm
                rv.adapter = adapter
            } else {
                when (val cur = lm) {
                    is GridLayoutManager -> if (cur.spanCount != next.cols) cur.spanCount = next.cols
                    is WaterfallLayoutManager -> cur.setSpanCount(next.cols)
                }
            }
            adapter.notifyDataSetChanged()
        }

        fun firstVisiblePosition(): Int {
            return when (val cur = lm) {
                is GridLayoutManager -> cur.findFirstVisibleItemPosition()
                is WaterfallLayoutManager -> cur.findFirstVisiblePosition()
                else -> RecyclerView.NO_POSITION
            }
        }

        fun lastVisiblePosition(): Int {
            return when (val cur = lm) {
                is GridLayoutManager -> cur.findLastVisibleItemPosition()
                is WaterfallLayoutManager -> cur.findLastVisiblePosition()
                else -> RecyclerView.NO_POSITION
            }
        }

        fun scrollToPositionWithOffset(position: Int, offsetPx: Int) {
            when (val cur = lm) {
                is GridLayoutManager -> cur.scrollToPositionWithOffset(position, offsetPx)
                is WaterfallLayoutManager -> cur.scrollToPositionWithOffset(position, offsetPx)
            }
        }

        fun parentWidthPx(cfg: LayoutConfig = config): Int {
            val w = rv.width - rv.paddingLeft - rv.paddingRight
            if (w > 0) return w
            val pw = primaryLayer.rv.width - primaryLayer.rv.paddingLeft - primaryLayer.rv.paddingRight
            if (pw > 0) return pw
            return context.resources.displayMetrics.widthPixels - dp(horizontalInsetDp * 2)
        }
    }

    private inner class GridAdapter(private val layer: GridLayer) : RecyclerView.Adapter<RecyclerView.ViewHolder>() {
        init {
            setHasStableIds(true)
        }

        override fun getItemCount(): Int = items.size

        override fun getItemId(position: Int): Long = stableIdFor(items[position].key)

        override fun getItemViewType(position: Int): Int = when (items[position].kind) {
            "header" -> TYPE_HEADER
            "folder_header" -> TYPE_FOLDER_HEADER
            "folder" -> TYPE_FOLDER
            else -> TYPE_TILE
        }

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
            return when (viewType) {
                TYPE_HEADER -> HeaderHolder(createHeaderView())
                TYPE_FOLDER_HEADER -> FolderHeaderHolder(createFolderHeaderView())
                TYPE_FOLDER -> FolderHolder(createFolderView(), layer)
                else -> TileHolder(createTileView(), layer)
            }
        }

        override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) {
            val entry = items[position]
            val cfg = layer.config
            when (holder) {
                is HeaderHolder -> holder.bind(entry)
                is FolderHeaderHolder -> holder.bind(entry)
                is FolderHolder -> holder.bind(entry, layer.parentWidthPx(cfg), cfg)
                is TileHolder -> holder.bind(entry, layer.parentWidthPx(cfg), cfg)
            }
        }
    }

    private data class WaterfallSlot(
        val left: Int,
        val top: Int,
        val right: Int,
        val bottom: Int
    )

    /**
     * Deterministic top-down masonry LayoutManager. Because every tile's exact
     * height is known from its aspect ratio, all positions are laid out from
     * index 0 in O(N) (<0.1ms), guaranteeing flush top alignment and zero
     * blank gaps across any sequence of pinch transitions.
     */
    private inner class WaterfallLayoutManager(initialCols: Int) : RecyclerView.LayoutManager() {
        private var cols = initialCols.coerceIn(2, 3)
        private var slots: List<WaterfallSlot> = emptyList()
        private var totalHeight = 0
        private var maxSlotHeight = 0
        private var scrollY = 0
        private var cachedWidth = -1
        private var cachedCols = -1
        private var cachedVersion = -1
        private var layoutDirty = true
        private var pendingPos = RecyclerView.NO_POSITION
        private var pendingOffset = 0
        private val attachedPos = HashSet<Int>(48)

        fun setSpanCount(nextCols: Int) {
            val clamped = nextCols.coerceIn(2, 3)
            if (clamped != cols) {
                cols = clamped
                layoutDirty = true
                requestLayout()
            }
        }

        override fun generateDefaultLayoutParams(): RecyclerView.LayoutParams =
            RecyclerView.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            )

        override fun canScrollVertically(): Boolean = true

        override fun onAdapterChanged(
            oldAdapter: RecyclerView.Adapter<*>?,
            newAdapter: RecyclerView.Adapter<*>?
        ) {
            removeAllViews()
            layoutDirty = true
        }

        override fun onItemsChanged(recyclerView: RecyclerView) {
            layoutDirty = true
        }

        override fun onItemsAdded(recyclerView: RecyclerView, positionStart: Int, itemCount: Int) {
            layoutDirty = true
        }

        override fun onItemsRemoved(recyclerView: RecyclerView, positionStart: Int, itemCount: Int) {
            layoutDirty = true
        }

        override fun onItemsUpdated(recyclerView: RecyclerView, positionStart: Int, itemCount: Int) {
            layoutDirty = true
        }

        override fun onItemsMoved(recyclerView: RecyclerView, from: Int, to: Int, itemCount: Int) {
            layoutDirty = true
        }

        override fun scrollToPosition(position: Int) {
            pendingPos = position
            pendingOffset = 0
            requestLayout()
        }

        fun scrollToPositionWithOffset(position: Int, offset: Int) {
            pendingPos = position
            pendingOffset = offset
            requestLayout()
        }

        fun findFirstVisiblePosition(): Int {
            var minPos = RecyclerView.NO_POSITION
            for (i in 0 until childCount) {
                val child = getChildAt(i) ?: continue
                val pos = getPosition(child)
                if (pos != RecyclerView.NO_POSITION && (minPos == RecyclerView.NO_POSITION || pos < minPos)) {
                    minPos = pos
                }
            }
            return minPos
        }

        fun findLastVisiblePosition(): Int {
            var maxPos = RecyclerView.NO_POSITION
            for (i in 0 until childCount) {
                val child = getChildAt(i) ?: continue
                val pos = getPosition(child)
                if (pos > maxPos) {
                    maxPos = pos
                }
            }
            return maxPos
        }

        override fun computeVerticalScrollOffset(state: RecyclerView.State): Int = scrollY

        override fun computeVerticalScrollExtent(state: RecyclerView.State): Int =
            maxOf(0, height - paddingTop - paddingBottom)

        override fun computeVerticalScrollRange(state: RecyclerView.State): Int = totalHeight

        override fun onLayoutChildren(recycler: RecyclerView.Recycler, state: RecyclerView.State) {
            if (state.isPreLayout) return
            val count = itemCount
            if (count == 0 || items.isEmpty()) {
                detachAndScrapAttachedViews(recycler)
                slots = emptyList()
                totalHeight = 0
                scrollY = 0
                return
            }
            val availW = (width - paddingLeft - paddingRight).coerceAtLeast(dp(48))
            ensureSlots(availW)

            val viewExtent = maxOf(0, height - paddingTop - paddingBottom)
            val maxScroll = maxOf(0, totalHeight - viewExtent)
            if (pendingPos != RecyclerView.NO_POSITION) {
                val p = pendingPos.coerceIn(0, slots.lastIndex)
                scrollY = (slots[p].top - pendingOffset).coerceIn(0, maxScroll)
                pendingPos = RecyclerView.NO_POSITION
                pendingOffset = 0
            } else {
                scrollY = scrollY.coerceIn(0, maxScroll)
            }

            detachAndScrapAttachedViews(recycler)
            attachMissingVisible(recycler)
        }

        override fun scrollVerticallyBy(
            dy: Int,
            recycler: RecyclerView.Recycler,
            state: RecyclerView.State
        ): Int {
            if (childCount == 0 || dy == 0 || slots.isEmpty()) return 0
            val viewExtent = maxOf(0, height - paddingTop - paddingBottom)
            val maxScroll = maxOf(0, totalHeight - viewExtent)
            val nextScroll = (scrollY + dy).coerceIn(0, maxScroll)
            val actualDy = nextScroll - scrollY
            if (actualDy == 0) return 0

            scrollY = nextScroll
            offsetChildrenVertical(-actualDy)

            for (i in childCount - 1 downTo 0) {
                val child = getChildAt(i) ?: continue
                if (getDecoratedBottom(child) < 0 || getDecoratedTop(child) > height) {
                    removeAndRecycleView(child, recycler)
                }
            }
            attachMissingVisible(recycler)
            return actualDy
        }

        private fun ensureSlots(availW: Int) {
            if (!layoutDirty && cachedWidth == availW && cachedCols == cols &&
                cachedVersion == itemsVersion && slots.size == items.size
            ) {
                return
            }
            layoutDirty = false
            cachedWidth = availW
            cachedCols = cols
            cachedVersion = itemsVersion

            val cfg = LayoutConfig(cols, isWaterfall = true)
            val marginTotal = dp(6)
            val colTops = IntArray(cols) { 0 }
            val out = ArrayList<WaterfallSlot>(items.size)
            var maxH = 0

            for (i in 0 until items.size) {
                val entry = items[i]
                if (entry.kind == "header" || entry.kind == "folder_header") {
                    var maxColY = 0
                    for (c in 0 until cols) {
                        if (colTops[c] > maxColY) maxColY = colTops[c]
                    }
                    val headerH = dp(38)
                    out.add(WaterfallSlot(paddingLeft, maxColY, paddingLeft + availW, maxColY + headerH))
                    val nextY = maxColY + headerH
                    for (c in 0 until cols) colTops[c] = nextY
                    if (headerH > maxH) maxH = headerH
                } else {
                    var bestCol = 0
                    var minY = colTops[0]
                    for (c in 1 until cols) {
                        if (colTops[c] < minY) {
                            minY = colTops[c]
                            bestCol = c
                        }
                    }
                    val slotLeft = paddingLeft + (availW * bestCol) / cols
                    val slotRight = paddingLeft + (availW * (bestCol + 1)) / cols
                    val cardH = tileHeightFor(entry, availW, cfg)
                    val slotH = cardH + marginTotal
                    val slotBottom = minY + slotH
                    out.add(WaterfallSlot(slotLeft, minY, slotRight, slotBottom))
                    colTops[bestCol] = slotBottom
                    if (slotH > maxH) maxH = slotH
                }
            }

            var total = 0
            for (c in 0 until cols) {
                if (colTops[c] > total) total = colTops[c]
            }
            slots = out
            totalHeight = total
            maxSlotHeight = maxH
        }

        private fun attachMissingVisible(recycler: RecyclerView.Recycler) {
            if (slots.isEmpty()) return
            attachedPos.clear()
            for (i in 0 until childCount) {
                val child = getChildAt(i) ?: continue
                val pos = getPosition(child)
                if (pos != RecyclerView.NO_POSITION) attachedPos.add(pos)
            }

            val contentTopOffset = paddingTop - scrollY
            val minContentTop = -contentTopOffset - maxSlotHeight
            val maxContentTop = height - contentTopOffset

            // Binary search first index whose slot.top >= minContentTop
            var lo = 0
            var hi = slots.lastIndex
            var startIdx = slots.size
            while (lo <= hi) {
                val mid = (lo + hi) ushr 1
                if (slots[mid].top < minContentTop) {
                    lo = mid + 1
                } else {
                    startIdx = mid
                    hi = mid - 1
                }
            }

            for (i in startIdx until slots.size) {
                val slot = slots[i]
                if (slot.top > maxContentTop) break
                val screenTop = slot.top + contentTopOffset
                val screenBottom = slot.bottom + contentTopOffset
                if (screenBottom < 0 || screenTop > height) continue
                if (attachedPos.contains(i)) continue

                val view = recycler.getViewForPosition(i)
                addView(view)
                val lp = view.layoutParams as RecyclerView.LayoutParams
                val childW = (slot.right - slot.left - lp.leftMargin - lp.rightMargin).coerceAtLeast(1)
                val childH = (slot.bottom - slot.top - lp.topMargin - lp.bottomMargin).coerceAtLeast(1)
                view.measure(
                    View.MeasureSpec.makeMeasureSpec(childW, View.MeasureSpec.EXACTLY),
                    View.MeasureSpec.makeMeasureSpec(childH, View.MeasureSpec.EXACTLY)
                )
                layoutDecoratedWithMargins(view, slot.left, screenTop, slot.right, screenBottom)
            }
        }
    }

    private fun createHeaderView(): View {
        val row = FrameLayout(context).apply {
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                RecyclerView.LayoutParams.WRAP_CONTENT
            )
            setPadding(dp(2), dp(12), dp(2), dp(6))
        }
        val title = TextView(context).apply {
            id = ID_HEADER_TITLE
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
            setTypeface(typeface, Typeface.BOLD)
            maxLines = 1
            ellipsize = TextUtils.TruncateAt.END
        }
        val count = TextView(context).apply {
            id = ID_HEADER_COUNT
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
        }
        row.addView(
            title,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.START or Gravity.CENTER_VERTICAL
            )
        )
        row.addView(
            count,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.END or Gravity.CENTER_VERTICAL
            )
        )
        return row
    }

    private fun createTileView(): View {
        val margin = dp(3)
        val container = FrameLayout(context).apply {
            layoutParams = RecyclerView.LayoutParams(
                RecyclerView.LayoutParams.MATCH_PARENT,
                dp(110)
            ).apply {
                setMargins(margin, margin, margin, margin)
            }
            clipToOutline = true
        }

        val image = ImageView(context).apply {
            id = ID_TILE_IMAGE
            scaleType = ImageView.ScaleType.CENTER_CROP
        }
        container.addView(
            image,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        val typeBadge = TextView(context).apply {
            id = ID_TILE_BADGE
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 10f)
            setTextColor(theme.onBadge)
            setPadding(dp(5), dp(1), dp(5), dp(1))
            background = GradientDrawable().apply {
                setColor(theme.badge)
                cornerRadius = dp(4).toFloat()
            }
            visibility = View.GONE
        }
        container.addView(
            typeBadge,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.TOP or Gravity.END
            ).apply {
                setMargins(0, dp(5), dp(5), 0)
            }
        )

        val shareBadge = TextView(context).apply {
            id = ID_TILE_SHARE
            text = "分享"
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 9f)
            setTextColor(theme.onBadge)
            setPadding(dp(5), dp(1), dp(5), dp(1))
            background = GradientDrawable().apply {
                setColor(theme.success)
                cornerRadius = dp(4).toFloat()
            }
            visibility = View.GONE
        }
        container.addView(
            shareBadge,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.WRAP_CONTENT,
                FrameLayout.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM or Gravity.START
            ).apply {
                setMargins(dp(5), 0, 0, dp(5))
            }
        )

        val selectScrim = FrameLayout(context).apply {
            id = ID_TILE_SELECT
            setBackgroundColor(theme.overlay)
            visibility = View.GONE
            val check = TextView(context).apply {
                id = ID_TILE_SELECT_CHECK
                text = "✓"
                setTextColor(theme.onAccent)
                setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
                gravity = Gravity.CENTER
                background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(theme.accent)
                }
            }
            addView(
                check,
                FrameLayout.LayoutParams(dp(22), dp(22), Gravity.TOP or Gravity.END).apply {
                    setMargins(0, dp(5), dp(5), 0)
                }
            )
        }
        container.addView(
            selectScrim,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        return container
    }

    private inner class HeaderHolder(private val view: View) : RecyclerView.ViewHolder(view) {
        private val titleView = view.findViewById<TextView>(ID_HEADER_TITLE)
        private val countView = view.findViewById<TextView>(ID_HEADER_COUNT)

        fun bind(entry: GridEntry) {
            titleView.text = entry.title
            titleView.setTextColor(theme.text)
            countView.text = "${entry.count} 项"
            countView.setTextColor(theme.textMuted)
        }
    }

    private inner class TileHolder(
        private val root: View,
        private val layer: GridLayer
    ) : RecyclerView.ViewHolder(root) {
        private val image = root.findViewById<ImageView>(ID_TILE_IMAGE)
        private val badge = root.findViewById<TextView>(ID_TILE_BADGE)
        private val share = root.findViewById<TextView>(ID_TILE_SHARE)
        private val selectOverlay = root.findViewById<View>(ID_TILE_SELECT)
        private var currentEntry: GridEntry? = null
        private var boundThemeVersion = -1
        private val bgDrawable = GradientDrawable().apply { cornerRadius = dp(8).toFloat() }

        init {
            root.setOnClickListener {
                if (isPinching || pinchBlockedUntilUp || layer !== primaryLayer) return@setOnClickListener
                currentEntry?.let { e ->
                    val loc = IntArray(2)
                    root.getLocationInWindow(loc)
                    val hostLoc = IntArray(2)
                    var p: ViewParent? = root.parent
                    while (p is View) {
                        if (p is NsgoView) {
                            p.getLocationInWindow(hostLoc)
                            break
                        }
                        p = p.parent
                    }
                    val d = root.resources.displayMetrics.density
                    events.emit("press", mapOf(
                        "index" to e.flatIndex,
                        "id" to e.id,
                        "x" to (loc[0] - hostLoc[0]) / d,
                        "y" to (loc[1] - hostLoc[1]) / d,
                        "w" to root.width / d,
                        "h" to root.height / d
                    ))
                }
            }
            root.setOnLongClickListener {
                if (isPinching || pinchBlockedUntilUp || layer !== primaryLayer) return@setOnLongClickListener true
                currentEntry?.let { e ->
                    events.emit("longPress", mapOf("index" to e.flatIndex, "id" to e.id))
                }
                true
            }
        }

        fun bind(entry: GridEntry, parentWidthPx: Int, cfg: LayoutConfig) {
            currentEntry = entry
            val colWidth = maxOf((parentWidthPx - dp(6) * cfg.cols) / cfg.cols, dp(48))
            val cellHeight = tileHeightFor(entry, parentWidthPx, cfg)
            val lp = root.layoutParams
            if (!cfg.isWaterfall && lp.height != cellHeight) {
                lp.height = cellHeight
                root.layoutParams = lp
            } else if (cfg.isWaterfall && lp.height != cellHeight) {
                lp.height = cellHeight
            }
            if (boundThemeVersion != themeVersion) {
                boundThemeVersion = themeVersion
                bgDrawable.setColor(theme.surfaceAlt)
                bgDrawable.setStroke(1, theme.border)
                root.background = bgDrawable
                badge.setTextColor(theme.onBadge)
                (badge.background as? GradientDrawable)?.setColor(theme.badge)
                share.setTextColor(theme.onBadge)
                (share.background as? GradientDrawable)?.setColor(theme.success)
                val check = selectOverlay.findViewById<TextView>(ID_TILE_SELECT_CHECK)
                check?.setTextColor(theme.onAccent)
                (check?.background as? GradientDrawable)?.setColor(theme.accent)
                (selectOverlay.background as? ColorDrawable)?.let {
                    selectOverlay.setBackgroundColor(theme.overlay)
                }
            }
            ThumbLoader.load(context, image, entry.thumbUrl, maxOf(colWidth, cellHeight))

            if (entry.badge.isNotEmpty()) {
                badge.text = entry.badge
                badge.visibility = View.VISIBLE
            } else {
                badge.visibility = View.GONE
            }
            share.visibility = if (entry.shareMarked) View.VISIBLE else View.GONE
            selectOverlay.visibility = if (entry.selected) View.VISIBLE else View.GONE
        }
    }

    private fun createFolderHeaderView(): View {
        val row = LinearLayout(context).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            layoutParams = ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(38))
            setPadding(dp(4), dp(8), dp(4), dp(4))
        }
        val chevron = TextView(context).apply {
            id = ID_FOLDER_HEADER_CHEVRON
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
            setTextColor(theme.textMuted)
            setPadding(0, 0, dp(6), 0)
        }
        val title = TextView(context).apply {
            id = ID_FOLDER_HEADER_TITLE
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
            setTypeface(typeface, Typeface.BOLD)
            setTextColor(theme.text)
            maxLines = 1
            ellipsize = TextUtils.TruncateAt.END
        }
        row.addView(chevron)
        row.addView(title)
        return row
    }

    private fun createFolderView(): View {
        val card = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = RecyclerView.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                dp(120)
            ).apply {
                val m = dp(3)
                setMargins(m, m, m, m)
            }
            setPadding(dp(5), dp(5), dp(5), dp(5))
            clipToOutline = true
        }

        val previewBox = FrameLayout(context).apply {
            id = ID_FOLDER_PREVIEW
            clipToOutline = true
            outlineProvider = object : android.view.ViewOutlineProvider() {
                override fun getOutline(view: View, outline: android.graphics.Outline) {
                    outline.setRoundRect(0, 0, view.width, view.height, dp(6).toFloat())
                }
            }
        }
        card.addView(
            previewBox,
            LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                0,
                1f
            )
        )

        val singleImage = ImageView(context).apply {
            id = ID_FOLDER_IMG_SINGLE
            scaleType = ImageView.ScaleType.CENTER_CROP
            visibility = View.GONE
        }
        previewBox.addView(
            singleImage,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        val grid2x2 = LinearLayout(context).apply {
            id = ID_FOLDER_GRID
            orientation = LinearLayout.VERTICAL
            visibility = View.GONE
        }
        val slotIds = intArrayOf(ID_FOLDER_IMG_0, ID_FOLDER_IMG_1, ID_FOLDER_IMG_2, ID_FOLDER_IMG_3)
        for (r in 0 until 2) {
            val row = LinearLayout(context).apply {
                orientation = LinearLayout.HORIZONTAL
            }
            for (c in 0 until 2) {
                val idx = r * 2 + c
                val img = ImageView(context).apply {
                    id = slotIds[idx]
                    scaleType = ImageView.ScaleType.CENTER_CROP
                }
                row.addView(
                    img,
                    LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.MATCH_PARENT, 1f).apply {
                        if (c == 0) marginEnd = dp(1)
                    }
                )
            }
            grid2x2.addView(
                row,
                LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f).apply {
                    if (r == 0) bottomMargin = dp(1)
                }
            )
        }
        previewBox.addView(
            grid2x2,
            FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
        )

        val emptyIcon = FolderGlyphView(context).apply {
            id = ID_FOLDER_EMPTY_ICON
        }
        previewBox.addView(
            emptyIcon,
            FrameLayout.LayoutParams(dp(28), dp(28), Gravity.CENTER)
        )

        val footer = LinearLayout(context).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(2), dp(4), dp(2), 0)
        }
        val smallIcon = FolderGlyphView(context).apply {
            id = ID_FOLDER_FOOTER_ICON
        }
        footer.addView(
            smallIcon,
            LinearLayout.LayoutParams(dp(13), dp(13)).apply { marginEnd = dp(4) }
        )
        val nameView = TextView(context).apply {
            id = ID_FOLDER_NAME
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
            setTypeface(typeface, Typeface.BOLD)
            maxLines = 1
            ellipsize = TextUtils.TruncateAt.END
        }
        footer.addView(
            nameView,
            LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
        )
        val countView = TextView(context).apply {
            id = ID_FOLDER_COUNT
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 10.5f)
            setPadding(dp(4), 0, 0, 0)
        }
        footer.addView(countView)
        card.addView(
            footer,
            LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                dp(22)
            )
        )

        return card
    }

    private class FolderGlyphView(context: Context) : View(context) {
        private val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeCap = Paint.Cap.ROUND
            strokeJoin = Paint.Join.ROUND
        }
        private val path = android.graphics.Path()
        var tintColor: Int = Color.GRAY
            set(value) {
                if (field != value) {
                    field = value
                    invalidate()
                }
            }

        override fun onDraw(canvas: Canvas) {
            val w = width.toFloat()
            val h = height.toFloat()
            if (w <= 0f || h <= 0f) return
            paint.color = tintColor
            paint.strokeWidth = maxOf(w * 0.085f, 1.5f)
            val l = w * 0.12f
            val r = w * 0.88f
            val t = h * 0.18f
            val b = h * 0.84f
            val tabR = w * 0.44f
            val tabB = h * 0.30f
            path.reset()
            path.moveTo(l, t + h * 0.06f)
            path.lineTo(l, b - h * 0.06f)
            path.quadTo(l, b, l + w * 0.06f, b)
            path.lineTo(r - w * 0.06f, b)
            path.quadTo(r, b, r, b - h * 0.06f)
            path.lineTo(r, tabB + h * 0.06f)
            path.quadTo(r, tabB, r - w * 0.06f, tabB)
            path.lineTo(tabR + w * 0.05f, tabB)
            path.lineTo(tabR - w * 0.03f, t)
            path.lineTo(l + w * 0.06f, t)
            path.quadTo(l, t, l, t + h * 0.06f)
            path.close()
            canvas.drawPath(path, paint)
        }
    }

    private inner class FolderHeaderHolder(private val view: View) : RecyclerView.ViewHolder(view) {
        private val chevronView = view.findViewById<TextView>(ID_FOLDER_HEADER_CHEVRON)
        private val titleView = view.findViewById<TextView>(ID_FOLDER_HEADER_TITLE)

        init {
            view.setOnClickListener {
                if (isPinching || pinchBlockedUntilUp) return@setOnClickListener
                events.emit("toggleSubfolders", emptyMap<String, Any>())
            }
        }

        fun bind(entry: GridEntry) {
            chevronView.text = if (entry.collapsed) "▸" else "▾"
            chevronView.setTextColor(theme.textMuted)
            titleView.text = entry.title
            titleView.setTextColor(theme.text)
        }
    }

    private inner class FolderHolder(
        private val root: View,
        private val layer: GridLayer
    ) : RecyclerView.ViewHolder(root) {
        private val previewBox = root.findViewById<FrameLayout>(ID_FOLDER_PREVIEW)
        private val singleImage = root.findViewById<ImageView>(ID_FOLDER_IMG_SINGLE)
        private val grid2x2 = root.findViewById<LinearLayout>(ID_FOLDER_GRID)
        private val slotImages = arrayOf(
            root.findViewById<ImageView>(ID_FOLDER_IMG_0),
            root.findViewById<ImageView>(ID_FOLDER_IMG_1),
            root.findViewById<ImageView>(ID_FOLDER_IMG_2),
            root.findViewById<ImageView>(ID_FOLDER_IMG_3)
        )
        private val emptyIcon = root.findViewById<FolderGlyphView>(ID_FOLDER_EMPTY_ICON)
        private val footerIcon = root.findViewById<FolderGlyphView>(ID_FOLDER_FOOTER_ICON)
        private val nameView = root.findViewById<TextView>(ID_FOLDER_NAME)
        private val countView = root.findViewById<TextView>(ID_FOLDER_COUNT)
        private val cardBg = GradientDrawable().apply { cornerRadius = dp(10).toFloat() }
        private val previewBg = GradientDrawable().apply { cornerRadius = dp(6).toFloat() }
        private var boundThemeVersion = -1
        private var currentEntry: GridEntry? = null

        init {
            root.setOnClickListener {
                if (isPinching || pinchBlockedUntilUp || layer !== primaryLayer) return@setOnClickListener
                currentEntry?.let { e ->
                    events.emit("pressFolder", mapOf("id" to e.id))
                }
            }
        }

        fun bind(entry: GridEntry, parentWidthPx: Int, cfg: LayoutConfig) {
            currentEntry = entry
            val colWidth = maxOf((parentWidthPx - dp(6) * cfg.cols) / cfg.cols, dp(48))
            val cellHeight = tileHeightFor(entry, parentWidthPx, cfg)
            val lp = root.layoutParams
            if (lp.height != cellHeight) {
                lp.height = cellHeight
                if (!cfg.isWaterfall) {
                    root.layoutParams = lp
                }
            }
            if (boundThemeVersion != themeVersion) {
                boundThemeVersion = themeVersion
                cardBg.setColor(theme.surface)
                cardBg.setStroke(1, theme.border)
                root.background = cardBg
                previewBg.setColor(theme.surfaceAlt)
                previewBox.background = previewBg
                emptyIcon.tintColor = theme.accent
                footerIcon.tintColor = theme.textMuted
                nameView.setTextColor(theme.text)
                countView.setTextColor(theme.faint)
                for (img in slotImages) {
                    img.setBackgroundColor(theme.surfaceAlt)
                }
            }
            nameView.text = entry.title
            countView.text = "${entry.count}"

            val urls = entry.thumbUrls
            when (urls.size) {
                0 -> {
                    singleImage.visibility = View.GONE
                    grid2x2.visibility = View.GONE
                    emptyIcon.visibility = View.VISIBLE
                }
                1 -> {
                    emptyIcon.visibility = View.GONE
                    grid2x2.visibility = View.GONE
                    singleImage.visibility = View.VISIBLE
                    ThumbLoader.load(context, singleImage, urls[0], colWidth)
                }
                else -> {
                    emptyIcon.visibility = View.GONE
                    singleImage.visibility = View.GONE
                    grid2x2.visibility = View.VISIBLE
                    val slotPx = maxOf(colWidth / 2, dp(32))
                    for (i in 0 until 4) {
                        val img = slotImages[i]
                        if (i < urls.size) {
                            ThumbLoader.load(context, img, urls[i], slotPx)
                        } else {
                            img.setImageDrawable(null)
                            img.tag = null
                        }
                    }
                }
            }
        }
    }

    companion object {
        private const val MODE_AUTO = "auto"
        private const val MODE_WATERFALL = "waterfall"
        private const val MODE_GRID = "grid"

        private const val TRIGGER_SLOP_DP = 10f
        private const val STEP_DISTANCE_DP = 76f

        private const val TYPE_HEADER = 0
        private const val TYPE_TILE = 1
        private const val TYPE_FOLDER_HEADER = 2
        private const val TYPE_FOLDER = 3
        private const val ID_HEADER_TITLE = 1001
        private const val ID_HEADER_COUNT = 1002
        private const val ID_TILE_IMAGE = 2001
        private const val ID_TILE_BADGE = 2002
        private const val ID_TILE_SHARE = 2003
        private const val ID_TILE_SELECT = 2004
        private const val ID_TILE_SELECT_CHECK = 2005
        private const val ID_FOLDER_HEADER_CHEVRON = 3001
        private const val ID_FOLDER_HEADER_TITLE = 3002
        private const val ID_FOLDER_PREVIEW = 3003
        private const val ID_FOLDER_IMG_SINGLE = 3004
        private const val ID_FOLDER_GRID = 3005
        private const val ID_FOLDER_IMG_0 = 3006
        private const val ID_FOLDER_IMG_1 = 3007
        private const val ID_FOLDER_IMG_2 = 3008
        private const val ID_FOLDER_IMG_3 = 3009
        private const val ID_FOLDER_EMPTY_ICON = 3010
        private const val ID_FOLDER_FOOTER_ICON = 3011
        private const val ID_FOLDER_NAME = 3012
        private const val ID_FOLDER_COUNT = 3013

        private fun normalizeLayoutMode(raw: String?): String = when (raw) {
            MODE_WATERFALL -> MODE_WATERFALL
            MODE_GRID -> MODE_GRID
            else -> MODE_AUTO
        }

        private fun resolveConfig(mode: String, cols: Int): LayoutConfig = when (mode) {
            MODE_WATERFALL -> LayoutConfig(cols.coerceIn(2, 3), true)
            MODE_GRID -> LayoutConfig(cols.coerceIn(2, 5), false)
            else -> {
                val c = cols.coerceIn(2, 5)
                LayoutConfig(c, c <= 3)
            }
        }

        private fun resolveStepConfig(mode: String, cur: LayoutConfig, dir: Int): LayoutConfig? {
            val nextCols = cur.cols + dir
            val maxCols = if (mode == MODE_WATERFALL) 3 else 5
            if (nextCols !in 2..maxCols) return null
            return resolveConfig(mode, nextCols)
        }
    }
}
