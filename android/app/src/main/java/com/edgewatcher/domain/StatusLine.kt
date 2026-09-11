package com.edgewatcher.domain

import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.ObservationState
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 稼働状態の文言。
 *
 * **画面と常駐通知が同じものを使う。** 端末を持ち上げなくても状態が分かるように
 * するためであり、2箇所で文言が食い違わないよう生成をここに一本化する。
 * モック（`docs/mocks/native-mocks.html` の `running()`）が題・詳細・印の3つ組で
 * 状態を出すので、生成側も同じ3つ組を返す。
 *
 * 「オフライン」を明示するのは、画面を見ただけでは通信の成否が分からないため。
 * 屋外設置後にオーナーが端末を見に行く動機のほとんどが「本当に送れているのか」の
 * 確認であり、その答えを最初に出す。
 *
 * **端末名は出さない。** POST /device/pair の応答に含まれず、端末は自分の名前を
 * 知らないため。
 */
data class StatusLine(
    val title: String,
    val detail: String,
    val tone: Tone,
) {

    /** 状態を表す印の色。モックの dot ok / warn / off に対応する。 */
    enum class Tone { OK, WARN, OFF }

    companion object {

        private val timeFormat = DateTimeFormatter.ofPattern("HH:mm")

        fun of(
            state: ObservationState,
            interval: IntervalMinutes,
            zone: ZoneId = ZoneId.systemDefault(),
        ): StatusLine = when (state) {
            is ObservationState.Observing -> StatusLine(
                title = "観測中",
                detail = lastUpload(state.lastUploadAt, zone)
                    ?.let { "$it ・ ${interval.minutes}分間隔" }
                    ?: "まだ送信していません ・ ${interval.minutes}分間隔",
                tone = Tone.OK,
            )

            is ObservationState.Offline -> StatusLine(
                title = "オフライン",
                // 待機件数を先に出す。異常の大きさが件数で伝わる。
                detail = listOfNotNull(
                    "${state.pendingCount}件待機中",
                    lastUpload(state.lastUploadAt, zone),
                ).joinToString(" ・ "),
                tone = Tone.WARN,
            )

            ObservationState.Stopped -> StatusLine(
                title = "停止中",
                detail = "観測は行われていません",
                tone = Tone.OFF,
            )
        }

        /** 一度も送っていなければ null。**送っていない時刻を騙らない。** */
        private fun lastUpload(at: java.time.Instant?, zone: ZoneId): String? =
            at?.let { "最終送信 " + timeFormat.format(it.atZone(zone)) }
    }
}
