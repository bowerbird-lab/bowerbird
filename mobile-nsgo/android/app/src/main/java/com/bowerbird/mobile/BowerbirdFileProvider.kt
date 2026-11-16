package com.bowerbird.mobile

import android.content.ContentProvider
import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.MatrixCursor
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.provider.OpenableColumns
import java.io.File
import java.io.FileNotFoundException

/**
 * Serves cached clipboard/media files to external apps via content:// URIs
 * under ${applicationId}.fileprovider without requiring AndroidX FileProvider.
 */
class BowerbirdFileProvider : ContentProvider() {
    override fun onCreate(): Boolean = true

    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor {
        if (mode.contains("w") || mode.contains("t")) {
            throw FileNotFoundException("Read-only provider: $uri")
        }
        val file = resolve(uri) ?: throw FileNotFoundException("File not found: $uri")
        return ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
    }

    override fun getType(uri: Uri): String {
        return uri.getQueryParameter("type")?.takeIf { it.isNotBlank() } ?: "application/octet-stream"
    }

    override fun query(
        uri: Uri,
        projection: Array<out String>?,
        selection: String?,
        selectionArgs: Array<out String>?,
        sortOrder: String?
    ): Cursor? {
        val file = resolve(uri) ?: return null
        val cols = projection ?: arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE)
        val outCols = ArrayList<String>(cols.size)
        val row = ArrayList<Any?>(cols.size)
        for (col in cols) {
            when (col) {
                OpenableColumns.DISPLAY_NAME -> {
                    outCols.add(col)
                    row.add(uri.lastPathSegment ?: file.name)
                }
                OpenableColumns.SIZE -> {
                    outCols.add(col)
                    row.add(file.length())
                }
            }
        }
        return MatrixCursor(outCols.toTypedArray(), 1).apply { addRow(row.toTypedArray()) }
    }

    override fun insert(uri: Uri, values: ContentValues?): Uri? = null
    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?): Int = 0
    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?): Int = 0

    private fun resolve(uri: Uri): File? {
        val ctx = context ?: return null
        val segs = uri.pathSegments
        if (segs.size < 2) return null
        val rootName = segs[0]
        val baseDir = when (rootName) {
            "clipboard" -> File(ctx.cacheDir, "clipboard")
            "media" -> File(ctx.cacheDir, "media")
            else -> return null
        }.canonicalFile
        val rel = segs.subList(1, segs.size).joinToString(File.separator)
        val target = File(baseDir, rel).canonicalFile
        if (!target.path.startsWith(baseDir.path + File.separator) || !target.isFile) return null
        return target
    }

    companion object {
        fun uriForClipboardFile(context: Context, file: File, displayName: String, mimeType: String): Uri {
            val clipboardDir = File(context.cacheDir, "clipboard").canonicalFile
            val canonical = file.canonicalFile
            require(canonical.path.startsWith(clipboardDir.path + File.separator)) {
                "File must reside inside cache/clipboard"
            }
            val rel = canonical.relativeTo(clipboardDir).path.replace(File.separatorChar, '/')
            val builder = Uri.Builder()
                .scheme("content")
                .authority(context.packageName + ".fileprovider")
                .appendPath("clipboard")
            rel.split('/').filter { it.isNotEmpty() }.forEach { builder.appendPath(it) }
            if (displayName.isNotBlank() && displayName != canonical.name) {
                // Keep display name via query only when needed; path must resolve to file
            }
            return builder.appendQueryParameter("type", mimeType).build()
        }
    }
}
