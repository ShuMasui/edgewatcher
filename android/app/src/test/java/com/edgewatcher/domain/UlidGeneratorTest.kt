package com.edgewatcher.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.util.Random

class UlidGeneratorTest {

    private val alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    /** 先頭10文字を 48bit のミリ秒に戻す。テスト側で独立に復号する。 */
    private fun decodeTimestamp(ulid: String): Long =
        ulid.take(10).fold(0L) { acc, c -> acc * 32 + alphabet.indexOf(c) }

    @Test
    fun `timestamp part equals the instant it was minted for`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val ulid = UlidGenerator.generate(at)

        assertEquals(at.toEpochMilli(), decodeTimestamp(ulid))
    }

    @Test
    fun `timestamp part follows the argument, not the wall clock`() {
        val past = Instant.parse("2020-01-02T03:04:05.006Z")

        val ulid = UlidGenerator.generate(past)

        assertEquals(past.toEpochMilli(), decodeTimestamp(ulid))
    }

    @Test
    fun `is 26 characters of Crockford base32`() {
        val ulid = UlidGenerator.generate(Instant.parse("2026-09-10T14:32:05.123Z"))

        assertEquals(26, ulid.length)
        assertTrue(ulid.all { it in alphabet })
    }

    @Test
    fun `two ulids for the same instant differ in the random part`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val a = UlidGenerator.generate(at)
        val b = UlidGenerator.generate(at)

        assertEquals(a.take(10), b.take(10))
        assertNotEquals(a.drop(10), b.drop(10))
    }

    @Test
    fun `lexical order follows chronological order`() {
        val earlier = UlidGenerator.generate(Instant.parse("2026-09-10T00:00:00Z"))
        val later = UlidGenerator.generate(Instant.parse("2026-09-10T00:00:01Z"))

        assertTrue(earlier < later)
    }

    @Test
    fun `random part is drawn from the supplied source`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val a = UlidGenerator.generate(at, Random(42))
        val b = UlidGenerator.generate(at, Random(42))

        assertEquals(a, b)
    }
}
