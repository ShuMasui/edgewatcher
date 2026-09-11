package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant

class UploadOrderTest {

    private fun observation(index: Int): PendingObservation =
        PendingObservation(
            observationId = "OBS%04d".format(index),
            capturedAt = Instant.parse("2026-09-10T00:00:00Z").plusSeconds(index * 300L),
            coordinates = null,
            totalBytes = 300_000,
        )

    private fun ids(rows: List<PendingObservation>) = rows.map { it.observationId }

    @Test
    fun `an empty buffer produces an empty order`() {
        assertEquals(emptyList<PendingObservation>(), UploadOrder.order(emptyList()))
    }

    @Test
    fun `a single row is sent as-is`() {
        val rows = listOf(observation(0))

        assertEquals(listOf("OBS0000"), ids(UploadOrder.order(rows)))
    }

    @Test
    fun `the newest goes first, then the rest oldest-first`() {
        val rows = (0..4).map { observation(it) }

        assertEquals(
            listOf("OBS0004", "OBS0000", "OBS0001", "OBS0002", "OBS0003"),
            ids(UploadOrder.order(rows)),
        )
    }

    @Test
    fun `the newest is not sent twice`() {
        val rows = (0..4).map { observation(it) }

        val ordered = UploadOrder.order(rows)

        assertEquals(rows.size, ordered.size)
        assertEquals(rows.size, ordered.map { it.observationId }.toSet().size)
    }

    @Test
    fun `a long backlog still puts the newest first`() {
        val rows = (0..287).map { observation(it) }

        val ordered = UploadOrder.order(rows)

        assertEquals("OBS0287", ordered.first().observationId)
        assertEquals("OBS0000", ordered[1].observationId)
        assertEquals("OBS0286", ordered.last().observationId)
    }

    @Test
    fun `input that is not sorted is still ordered by capturedAt`() {
        val rows = listOf(observation(2), observation(0), observation(4), observation(1))

        assertEquals(
            listOf("OBS0004", "OBS0000", "OBS0001", "OBS0002"),
            ids(UploadOrder.order(rows)),
        )
    }
}
