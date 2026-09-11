package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

class BufferPolicyTest {

    /** capturedAt は index 分だけ後ろにずらす。index が小さいほど古い。 */
    private fun observation(index: Int, bytes: Long): PendingObservation =
        PendingObservation(
            observationId = "OBS%04d".format(index),
            capturedAt = Instant.parse("2026-09-10T00:00:00Z").plusSeconds(index * 300L),
            coordinates = null,
            totalBytes = bytes,
        )

    private fun buffer(count: Int, bytesEach: Long): List<PendingObservation> =
        (0 until count).map { observation(it, bytesEach) }

    @Test
    fun `an empty buffer evicts nothing`() {
        assertEquals(emptyList<String>(), BufferPolicy.evictions(emptyList()))
    }

    @Test
    fun `a buffer at exactly the count limit evicts nothing`() {
        val rows = buffer(count = 288, bytesEach = 300_000)

        assertEquals(emptyList<String>(), BufferPolicy.evictions(rows))
    }

    @Test
    fun `one row over the count limit evicts exactly the oldest one`() {
        val rows = buffer(count = 289, bytesEach = 300_000)

        assertEquals(listOf("OBS0000"), BufferPolicy.evictions(rows))
    }

    @Test
    fun `five rows over the count limit evict the five oldest, in order`() {
        val rows = buffer(count = 293, bytesEach = 300_000)

        assertEquals(
            listOf("OBS0000", "OBS0001", "OBS0002", "OBS0003", "OBS0004"),
            BufferPolicy.evictions(rows),
        )
    }

    @Test
    fun `the byte limit can bite well before the count limit`() {
        // 10件 x 60MiB = 600MiB > 500MiB。件数は 10 で上限のはるか下。
        val sixtyMiB = 62_914_560L
        val rows = buffer(count = 10, bytesEach = sixtyMiB)

        val evicted = BufferPolicy.evictions(rows)

        assertTrue("byte limit must evict even well under 288 rows", evicted.isNotEmpty())
        val remainingBytes = rows.filterNot { it.observationId in evicted }.sumOf { it.totalBytes }
        assertTrue(remainingBytes <= BufferPolicy.MAX_BYTES)
    }

    @Test
    fun `the byte limit evicts the fewest rows that bring it under, oldest first`() {
        val sixtyMiB = 62_914_560L
        val rows = buffer(count = 10, bytesEach = sixtyMiB)

        // 600MiB を 500MiB 以下にするには 2件（120MiB）落とせば足りる。
        assertEquals(listOf("OBS0000", "OBS0001"), BufferPolicy.evictions(rows))
    }

    @Test
    fun `a buffer at exactly the byte limit evicts nothing`() {
        val rows = listOf(observation(0, BufferPolicy.MAX_BYTES))

        assertEquals(emptyList<String>(), BufferPolicy.evictions(rows))
    }

    @Test
    fun `whichever limit is exceeded governs, and both end up satisfied`() {
        // 300件 x 2MiB = 600MiB。件数も容量も超えている。
        val twoMiB = 2_097_152L
        val rows = buffer(count = 300, bytesEach = twoMiB)

        val evicted = BufferPolicy.evictions(rows)
        val remaining = rows.filterNot { it.observationId in evicted }

        assertTrue(remaining.size <= BufferPolicy.MAX_COUNT)
        assertTrue(remaining.sumOf { it.totalBytes } <= BufferPolicy.MAX_BYTES)
        // 落とすのは常に古い側から。新しい側が残る。
        assertEquals("OBS0299", remaining.last().observationId)
    }

    @Test
    fun `the limits are the values the spec fixed`() {
        assertEquals(288, BufferPolicy.MAX_COUNT)
        assertEquals(524_288_000L, BufferPolicy.MAX_BYTES)
    }
}
