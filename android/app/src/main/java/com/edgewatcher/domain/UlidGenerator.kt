package com.edgewatcher.domain

import java.time.Instant
import java.util.Random

/**
 * ULID を採番する。
 *
 * **採番時刻を必ず引数で受け取る。** サーバは日別クエリの範囲を ULID の
 * タイムスタンプ部から導くため、ここが `capturedAt` からずれると観測が
 * 別の日に並ぶ。端末の画面には何も現れないので、実機では気づけない。
 * 引数なしで呼べるオーバーロードを足してはならない。
 */
object UlidGenerator {

    /** Crockford Base32。I・L・O・U を除いた32文字。 */
    private const val ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    private const val TIME_CHARS = 10
    private const val RANDOM_CHARS = 16

    fun generate(at: Instant, random: Random = Random()): String {
        val builder = StringBuilder(TIME_CHARS + RANDOM_CHARS)
        encodeTime(at.toEpochMilli(), builder)
        repeat(RANDOM_CHARS) { builder.append(ALPHABET[random.nextInt(32)]) }
        return builder.toString()
    }

    /** 48bit を上位から5bitずつ、10文字に詰める。 */
    private fun encodeTime(millis: Long, out: StringBuilder) {
        for (shift in (TIME_CHARS - 1) downTo 0) {
            val index = ((millis ushr (shift * 5)) and 0x1F).toInt()
            out.append(ALPHABET[index])
        }
    }
}
