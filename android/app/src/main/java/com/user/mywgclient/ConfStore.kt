package com.user.mywgclient

import android.content.Context
import java.io.File

/** App-private copy of the last pasted config. Not backed up (allowBackup=false). */
internal object ConfStore {
    private const val NAME = "last.conf"

    fun read(context: Context): String? {
        val file = file(context)
        if (!file.isFile) return null
        return file.readText()
    }

    fun write(context: Context, text: String) {
        val dst = file(context)
        val tmp = File(context.filesDir, "$NAME.tmp")
        tmp.writeText(text)
        if (!tmp.renameTo(dst)) {
            dst.writeText(text)
            tmp.delete()
        }
    }

    private fun file(context: Context) = File(context.filesDir, NAME)
}
