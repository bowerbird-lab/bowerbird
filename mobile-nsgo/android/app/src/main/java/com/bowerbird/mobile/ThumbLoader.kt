package com.bowerbird.mobile

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.os.Handler
import android.os.Looper
import android.util.LruCache
import android.widget.ImageView
import java.io.File
import java.io.FileOutputStream
import java.lang.ref.WeakReference
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.concurrent.Executors

/**
 * Multi-threaded thumbnail loader with memory LruCache and disk cache.
 * Strips ?token= from cache keys so tokens never hit disk cache filenames.
 */
object ThumbLoader {
    private val main = Handler(Looper.getMainLooper())
    private val pool = Executors.newFixedThreadPool(4) { r ->
        Thread(r, "bb-thumbs").apply { isDaemon = true }
    }

    private val memCache: LruCache<String, Bitmap> by lazy {
        val maxKb = (Runtime.getRuntime().maxMemory() / 1024L).toInt()
        val cacheKb = minOf(maxKb / 6, 64 * 1024)
        object : LruCache<String, Bitmap>(cacheKb) {
            override fun sizeOf(key: String, value: Bitmap): Int = value.byteCount / 1024
        }
    }

    fun load(context: Context, imageView: ImageView, url: String, targetPx: Int) {
        if (url.isBlank()) {
            imageView.tag = null
            imageView.setImageDrawable(null)
            return
        }
        val key = cacheKey(url)
        imageView.tag = key
        memCache.get(key)?.let { cached ->
            imageView.setImageBitmap(cached)
            return
        }
        imageView.setImageDrawable(null)
        val ref = WeakReference(imageView)
        val appCtx = context.applicationContext
        val sizePx = maxOf(targetPx, 240)
        pool.execute {
            if (ref.get()?.tag != key) return@execute
            val bmp = loadOrFetch(appCtx, url, key, sizePx) ?: return@execute
            memCache.put(key, bmp)
            main.post {
                val view = ref.get() ?: return@post
                if (view.tag == key) {
                    view.setImageBitmap(bmp)
                }
            }
        }
    }

    private fun loadOrFetch(context: Context, url: String, key: String, targetPx: Int): Bitmap? {
        val diskDir = File(context.cacheDir, "thumbs_v1").apply { mkdirs() }
        val diskFile = File(diskDir, key)
        if (diskFile.isFile && diskFile.length() > 0) {
            decodeFileSampled(diskFile, targetPx)?.let { return it }
            diskFile.delete()
        }
        var conn: HttpURLConnection? = null
        try {
            conn = (URL(url).openConnection() as HttpURLConnection).apply {
                connectTimeout = 8000
                readTimeout = 15000
                useCaches = false
            }
            if (conn.responseCode !in 200..299) return null
            val tmp = File(diskDir, ".$key.part")
            conn.inputStream.use { input ->
                FileOutputStream(tmp).use { output ->
                    input.copyTo(output, 16384)
                }
            }
            if (tmp.length() <= 0) {
                tmp.delete()
                return null
            }
            tmp.renameTo(diskFile)
            return decodeFileSampled(diskFile, targetPx)
        } catch (_: Throwable) {
            return null
        } finally {
            conn?.disconnect()
        }
    }

    private fun decodeFileSampled(file: File, targetPx: Int): Bitmap? {
        val opts = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeFile(file.absolutePath, opts)
        if (opts.outWidth <= 0 || opts.outHeight <= 0) return null
        var sample = 1
        while (opts.outWidth / (sample * 2) >= targetPx && opts.outHeight / (sample * 2) >= targetPx) {
            sample *= 2
        }
        val decOpts = BitmapFactory.Options().apply {
            inSampleSize = sample
            inPreferredConfig = Bitmap.Config.ARGB_8888
        }
        return BitmapFactory.decodeFile(file.absolutePath, decOpts)
    }

    private fun cacheKey(rawUrl: String): String {
        val clean = rawUrl.replace(Regex("([?&])token=[^&]*"), "$1")
        val digest = MessageDigest.getInstance("SHA-256").digest(clean.toByteArray())
        return digest.joinToString("") { "%02x".format(it) }
    }
}
