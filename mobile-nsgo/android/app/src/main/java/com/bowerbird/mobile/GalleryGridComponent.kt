package com.bowerbird.mobile

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.text.TextUtils
import android.util.TypedValue
import android.view.Gravity
import android.view.MotionEvent
import android.view.ScaleGestureDetector
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.TextView
import androidx.recyclerview.widget.DiffUtil
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout
import dev.oneo.nsgo.NsgoComponent

/**
 * Native 120fps gallery grid backed by Android RecyclerView + GridLayoutManager
 * + SwipeRefreshLayout. Handles month headers via SpanSizeLookup, multi-threaded
 * thumbnail decoding, pinch-to-zoom column density (2..5), and infinite scroll
 * without per-frame Go IPC.
 */
class GalleryGridComponent(
    private val context: Context,
    private val events: NsgoComponent.Events
) : NsgoComponent {

    data class GridEntry(
        val kind: String, // "header" or "item"
        val key: String,
        val title: String,
        val count: Int,
        val flatIndex: Int,
        val id: String,
        val thumbUrl: String,
        val badge: String,
        val shareMarked: Boolean,
        val selected: Boolean
    )

    private val density = context.resources.displayMetrics.density
    private var spanCount = 3
    private var isDark = false
    private var lastScrollTopSeq = 0
    private var topInsetDp = 0
    private var bottomInsetDp = 24
    private var items: List<GridEntry> = emptyList()
    private var emptyMessage = ""
    private var loadMoreTriggeredForSize = -1

    private val swipeRefresh = SwipeRefreshLayout(context)
    private val rootContainer = FrameLayout(context)
    private val recyclerView = RecyclerView(context)
    private val emptyView = TextView(context)
    private val layoutManager = GridLayoutManager(context, spanCount)
    private val adapter = GridAdapter()

    private var pinchBaseCols = 3
    private var pinchAccumScale = 1f
    private val scaleDetector = ScaleGestureDetector(
        context,
        object : ScaleGestureDetector.SimpleOnScaleGestureListener() {
            override fun onScaleBegin(detector: ScaleGestureDetector): Boolean {
                pinchBaseCols = spanCount
                pinchAccumScale = 1f
                return true
            }

            override fun onScale(detector: ScaleGestureDetector): Boolean {
                pinchAccumScale *= detector.scaleFactor
                val delta = when {
                    pinchAccumScale > 1.24f -> -1
                    pinchAccumScale < 0.78f -> 1
                    else -> 0
                }
                if (delta != 0) {
                    val next = (pinchBaseCols + delta).coerceIn(2, 5)
                    if (next != spanCount) {
                        pinchBaseCols = next
                        pinchAccumScale = 1f
                        events.emit("pinchCols", mapOf("cols" to next))
                    }
                }
                return true
            }
        }
    )

    init {
        layoutManager.spanSizeLookup = object : GridLayoutManager.SpanSizeLookup() {
            override fun getSpanSize(position: Int): Int {
                val item = items.getOrNull(position) ?: return 1
                return if (item.kind == "header") spanCount else 1
            }
        }

        recyclerView.apply {
            layoutManager = this@GalleryGridComponent.layoutManager
            adapter = this@GalleryGridComponent.adapter
            setHasFixedSize(false)
            itemAnimator = null
            setItemViewCacheSize(24)
            clipToPadding = false
            applyInsets()
            addOnScrollListener(object : RecyclerView.OnScrollListener() {
                override fun onScrolled(rv: RecyclerView, dx: Int, dy: Int) {
                    if (dy <= 0) return
                    val total = items.size
                    if (total == 0) return
                    val lastVisible = this@GalleryGridComponent.layoutManager.findLastVisibleItemPosition()
                    if (lastVisible >= total - spanCount * 4 && loadMoreTriggeredForSize != total) {
                        loadMoreTriggeredForSize = total
                        events.emit("loadMore", null)
                    }
                }
            })
            @SuppressLint("ClickableViewAccessibility")
            setOnTouchListener { _, ev ->
                scaleDetector.onTouchEvent(ev)
                ev.pointerCount > 1
            }
        }

        emptyView.apply {
            gravity = Gravity.CENTER
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 14f)
            setPadding(dp(24), dp(48), dp(24), dp(48))
            visibility = View.GONE
        }

        rootContainer.addView(
            recyclerView,
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

        swipeRefresh.apply {
            addView(
                rootContainer,
                ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT
                )
            )
            setOnRefreshListener {
                events.emit("refresh", null)
            }
        }
    }

    override fun view(): View = swipeRefresh

    override fun setProp(name: String, value: Any?) {
        when (name) {
            "cols" -> {
                val next = ((value as? Number)?.toInt() ?: 3).coerceIn(2, 5)
                if (next != spanCount) {
                    val anchorPos = layoutManager.findFirstVisibleItemPosition()
                    spanCount = next
                    layoutManager.spanCount = next
                    adapter.notifyDataSetChanged()
                    if (anchorPos >= 0 && anchorPos < items.size) {
                        layoutManager.scrollToPositionWithOffset(anchorPos, 0)
                    }
                }
            }
            "isDark" -> {
                val dark = value == true
                if (dark != isDark) {
                    isDark = dark
                    emptyView.setTextColor(if (isDark) Color.parseColor("#A1A1AA") else Color.parseColor("#71717A"))
                    adapter.notifyDataSetChanged()
                }
            }
            "refreshing" -> {
                swipeRefresh.isRefreshing = value == true
            }
            "topInsetDp" -> {
                val next = ((value as? Number)?.toInt() ?: 0).coerceAtLeast(0)
                if (next != topInsetDp) {
                    topInsetDp = next
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
                    recyclerView.scrollToPosition(0)
                }
            }
            "rows" -> {
                val rawList = value as? List<*> ?: emptyList<Any>()
                val parsed = ArrayList<GridEntry>(rawList.size)
                for (raw in rawList) {
                    val m = raw as? Map<*, *> ?: continue
                    parsed.add(
                        GridEntry(
                            kind = (m["kind"] as? String).orEmpty(),
                            key = (m["key"] as? String).orEmpty(),
                            title = (m["title"] as? String).orEmpty(),
                            count = (m["count"] as? Number)?.toInt() ?: 0,
                            flatIndex = (m["flatIndex"] as? Number)?.toInt() ?: 0,
                            id = (m["id"] as? String).orEmpty(),
                            thumbUrl = (m["thumbUrl"] as? String).orEmpty(),
                            badge = (m["badge"] as? String).orEmpty(),
                            shareMarked = m["shareMarked"] == true,
                            selected = m["selected"] == true
                        )
                    )
                }
                val old = items
                if (parsed.size != old.size) {
                    loadMoreTriggeredForSize = -1
                }
                items = parsed
                val diff = DiffUtil.calculateDiff(object : DiffUtil.Callback() {
                    override fun getOldListSize(): Int = old.size
                    override fun getNewListSize(): Int = parsed.size
                    override fun areItemsTheSame(o: Int, n: Int): Boolean = old[o].key == parsed[n].key
                    override fun areContentsTheSame(o: Int, n: Int): Boolean = old[o] == parsed[n]
                }, false)
                diff.dispatchUpdatesTo(adapter)
                updateEmptyVisibility()
            }
        }
    }

    private fun applyInsets() {
        val pad = dp(8)
        recyclerView.setPadding(pad, dp(topInsetDp), pad, dp(bottomInsetDp))
        swipeRefresh.setProgressViewOffset(false, dp(topInsetDp), dp(topInsetDp + 56))
    }

    private fun updateEmptyVisibility() {
        val showEmpty = items.isEmpty() && emptyMessage.isNotBlank()
        emptyView.visibility = if (showEmpty) View.VISIBLE else View.GONE
        emptyView.setTextColor(if (isDark) Color.parseColor("#A1A1AA") else Color.parseColor("#71717A"))
    }

    private fun dp(v: Int): Int = (v * density).toInt()

    private inner class GridAdapter : RecyclerView.Adapter<RecyclerView.ViewHolder>() {
        override fun getItemCount(): Int = items.size

        override fun getItemViewType(position: Int): Int =
            if (items[position].kind == "header") TYPE_HEADER else TYPE_TILE

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
            return if (viewType == TYPE_HEADER) HeaderHolder(createHeaderView(parent))
            else TileHolder(createTileView(parent))
        }

        override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) {
            val entry = items[position]
            if (holder is HeaderHolder) {
                holder.bind(entry)
            } else if (holder is TileHolder) {
                holder.bind(entry, parentWidthPx())
            }
        }

        private fun parentWidthPx(): Int {
            val w = recyclerView.width - recyclerView.paddingLeft - recyclerView.paddingRight
            return if (w > 0) w else context.resources.displayMetrics.widthPixels - dp(20)
        }
    }

    private fun createHeaderView(parent: ViewGroup): View {
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
            setTypeface(typeface, android.graphics.Typeface.BOLD)
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

    private fun createTileView(parent: ViewGroup): View {
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
            setTextColor(Color.WHITE)
            setPadding(dp(5), dp(1), dp(5), dp(1))
            background = GradientDrawable().apply {
                setColor(Color.argb(160, 0, 0, 0))
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
            setTextColor(Color.WHITE)
            setPadding(dp(5), dp(1), dp(5), dp(1))
            background = GradientDrawable().apply {
                setColor(Color.argb(220, 22, 163, 74))
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
            setBackgroundColor(Color.argb(96, 0, 0, 0))
            visibility = View.GONE
            val check = TextView(context).apply {
                text = "✓"
                setTextColor(Color.WHITE)
                setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
                gravity = Gravity.CENTER
                background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(Color.parseColor("#2563EB"))
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

    private inner class HeaderHolder(view: View) : RecyclerView.ViewHolder(view) {
        private val titleView = view.findViewById<TextView>(ID_HEADER_TITLE)
        private val countView = view.findViewById<TextView>(ID_HEADER_COUNT)

        fun bind(entry: GridEntry) {
            titleView.text = entry.title
            titleView.setTextColor(if (isDark) Color.parseColor("#F4F4F5") else Color.parseColor("#18181B"))
            countView.text = "${entry.count} 项"
            countView.setTextColor(if (isDark) Color.parseColor("#A1A1AA") else Color.parseColor("#71717A"))
        }
    }

    private inner class TileHolder(private val root: View) : RecyclerView.ViewHolder(root) {
        private val image = root.findViewById<ImageView>(ID_TILE_IMAGE)
        private val badge = root.findViewById<TextView>(ID_TILE_BADGE)
        private val share = root.findViewById<TextView>(ID_TILE_SHARE)
        private val selectOverlay = root.findViewById<View>(ID_TILE_SELECT)
        private var currentEntry: GridEntry? = null
        private var boundDark: Boolean? = null
        private val bgDrawable = GradientDrawable().apply { cornerRadius = dp(8).toFloat() }

        init {
            root.setOnClickListener {
                currentEntry?.let { e ->
                    events.emit("press", mapOf("index" to e.flatIndex, "id" to e.id))
                }
            }
            root.setOnLongClickListener {
                currentEntry?.let { e ->
                    events.emit("longPress", mapOf("index" to e.flatIndex, "id" to e.id))
                }
                true
            }
        }

        fun bind(entry: GridEntry, parentWidthPx: Int) {
            currentEntry = entry
            val cellSize = maxOf((parentWidthPx - dp(6) * spanCount) / spanCount, dp(48))
            val lp = root.layoutParams
            if (lp.height != cellSize) {
                lp.height = cellSize
                root.layoutParams = lp
            }
            if (boundDark != isDark) {
                boundDark = isDark
                bgDrawable.setColor(if (isDark) Color.parseColor("#27272A") else Color.parseColor("#F4F4F5"))
                bgDrawable.setStroke(1, if (isDark) Color.parseColor("#3F3F46") else Color.parseColor("#E4E4E7"))
                root.background = bgDrawable
            }
            ThumbLoader.load(context, image, entry.thumbUrl, cellSize)

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

    companion object {
        private const val TYPE_HEADER = 0
        private const val TYPE_TILE = 1
        private const val ID_HEADER_TITLE = 1001
        private const val ID_HEADER_COUNT = 1002
        private const val ID_TILE_IMAGE = 2001
        private const val ID_TILE_BADGE = 2002
        private const val ID_TILE_SHARE = 2003
        private const val ID_TILE_SELECT = 2004
    }
}
