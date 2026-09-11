package com.edgewatcher.infrastructure.camera

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.edgewatcher.domain.port.JpegEncoder
import java.io.ByteArrayOutputStream
import kotlin.math.max
import kotlin.math.roundToInt

/**
 * 言われた寸法で符号化するだけ。**何段階で落とすかは ImagePolicy が決める。**
 *
 * inSampleSize で先に間引くのは、フル解像度の Bitmap を素で確保すると
 * 安価な端末で OOM になるため。2の冪でしか効かないので、そのあと
 * createScaledBitmap で正確な長辺に合わせる。
 */
class AndroidJpegEncoder : JpegEncoder {

    override fun encode(source: ByteArray, longestSide: Int, quality: Int): ByteArray {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(source, 0, source.size, bounds)

        val sourceLongest = max(bounds.outWidth, bounds.outHeight)
        val options = BitmapFactory.Options().apply {
            inSampleSize = sampleSizeFor(sourceLongest, longestSide)
        }
        val decoded = BitmapFactory.decodeByteArray(source, 0, source.size, options)
            ?: return ByteArray(0)

        val scaled = scaleToLongestSide(decoded, longestSide)
        if (scaled !== decoded) decoded.recycle()

        val out = ByteArrayOutputStream()
        scaled.compress(Bitmap.CompressFormat.JPEG, quality, out)
        scaled.recycle()
        return out.toByteArray()
    }

    private fun sampleSizeFor(sourceLongest: Int, targetLongest: Int): Int {
        var sample = 1
        while (sourceLongest / (sample * 2) >= targetLongest) sample *= 2
        return sample
    }

    private fun scaleToLongestSide(bitmap: Bitmap, longestSide: Int): Bitmap {
        val longest = max(bitmap.width, bitmap.height)
        if (longest <= longestSide) return bitmap
        val ratio = longestSide.toDouble() / longest
        return Bitmap.createScaledBitmap(
            bitmap,
            (bitmap.width * ratio).roundToInt().coerceAtLeast(1),
            (bitmap.height * ratio).roundToInt().coerceAtLeast(1),
            true,
        )
    }
}
