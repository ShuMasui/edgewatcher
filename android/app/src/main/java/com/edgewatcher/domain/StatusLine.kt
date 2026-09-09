package com.edgewatcher.domain

import com.edgewatcher.domain.model.ObservationState
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 稼働状態を1行の日本語にする。
 *
 * **画面と常駐通知が同じ文字列を使う。** 端末を持ち上げなくても状態が分かるように
 * するためであり、2箇所で文言が食い違わないよう生成をここに一本化する。
 *
 * 「オフライン」を明示するのは、画面を見ただけでは通信の成否が分からないため。
 * 屋外設置後にオーナーが端末を見に行く動機のほとんどが「本当に送れているのか」の
 * 確認であり、その答えを最初に出す。
 *
 * **端末名は出さない。** POST /device/pair の応答に含まれず、端末は自分の名前を
 * 知らないため。
 */
object StatusLine {

    private val timeFormat = DateTimeFormatter.ofPattern("HH:mm")

    fun render(state: ObservationState, zone: ZoneId = ZoneId.systemDefault()): String =
        when (state) {
            is ObservationState.Observing ->
                if (state.lastUploadAt == null) {
                    "観測中"
                } else {
                    "観測中 / 最終送信 " + timeFormat.format(state.lastUploadAt.atZone(zone))
                }

            is ObservationState.Offline -> "オフライン / " + state.pendingCount + "件待機中"

            ObservationState.Stopped -> "停止中"
        }
}
