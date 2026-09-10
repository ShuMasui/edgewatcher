package com.edgewatcher.domain

import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.ObservationState
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/**
 * 状態行の文言。**画面と常駐通知が同じものを使う。**
 *
 * モック（docs/mocks/native-mocks.html の running()）が題・詳細・印の3つ組で
 * 状態を出すため、生成側も同じ3つ組を返す。片方だけを組み立て直すと、
 * 画面と通知で文言が食い違う。
 */
class StatusLineTest {

    private val zone = ZoneId.of("Asia/Tokyo")

    /** 2026-09-10 14:32 JST */
    private val at1432 = Instant.parse("2026-09-10T05:32:00Z")

    /** 2026-09-10 11:48 JST */
    private val at1148 = Instant.parse("2026-09-10T02:48:00Z")

    @Test
    fun `観測中は最終送信と送信間隔を出す`() {
        val line = StatusLine.of(ObservationState.Observing(at1432), IntervalMinutes.FIVE, zone)

        assertEquals("観測中", line.title)
        assertEquals("最終送信 14:32 ・ 5分間隔", line.detail)
        assertEquals(StatusLine.Tone.OK, line.tone)
    }

    @Test
    fun `一度も送っていなければ最終送信を騙らない`() {
        val line = StatusLine.of(ObservationState.Observing(null), IntervalMinutes.FIFTEEN, zone)

        assertEquals("観測中", line.title)
        assertEquals("まだ送信していません ・ 15分間隔", line.detail)
        assertEquals(StatusLine.Tone.OK, line.tone)
    }

    @Test
    fun `オフラインは待機件数を先に出す`() {
        val line = StatusLine.of(
            ObservationState.Offline(pendingCount = 12, lastUploadAt = at1148),
            IntervalMinutes.FIVE,
            zone,
        )

        assertEquals("オフライン", line.title)
        assertEquals("12件待機中 ・ 最終送信 11:48", line.detail)
        assertEquals(StatusLine.Tone.WARN, line.tone)
    }

    @Test
    fun `オフラインで一度も送っていなければ件数だけを出す`() {
        val line = StatusLine.of(
            ObservationState.Offline(pendingCount = 3, lastUploadAt = null),
            IntervalMinutes.FIVE,
            zone,
        )

        assertEquals("オフライン", line.title)
        assertEquals("3件待機中", line.detail)
    }

    @Test
    fun `停止中は理由を出す`() {
        val line = StatusLine.of(ObservationState.Stopped, IntervalMinutes.FIVE, zone)

        assertEquals("停止中", line.title)
        assertEquals("観測は行われていません", line.detail)
        assertEquals(StatusLine.Tone.OFF, line.tone)
    }

    @Test
    fun `時刻は端末の時間帯で出す`() {
        val utc = StatusLine.of(ObservationState.Observing(at1432), IntervalMinutes.FIVE, ZoneId.of("UTC"))

        assertEquals("最終送信 05:32 ・ 5分間隔", utc.detail)
    }
}
