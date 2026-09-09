package com.edgewatcher.domain.model

import java.time.Instant

/** 長期資格情報。deviceSecret はセッションの再取得にしか使わない。 */
data class DeviceCredentials(
    val deviceId: String,
    val deviceSecret: String,
)

/** 日常のアップロードに使う短期資格情報。有効期限は12時間。 */
data class Session(
    val token: String,
    val expiresAtEpochSeconds: Long,
)

/** ペアリング時に自己申告する。表示専用で、何の認可判断にも使われない。 */
data class DeviceInfo(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

data class Coordinates(
    val lat: Double,
    val lng: Double,
)

/**
 * 送信待ちの観測1件のメタデータ。
 *
 * 画像そのものはファイルに置き、ここには持たない。BLOB を SQLite に入れると
 * DB が肥大し、削除しても領域が戻りにくいため。
 */
data class PendingObservation(
    val observationId: String,
    val capturedAt: Instant,
    val coordinates: Coordinates?,
    /** 本画像 + サムネイルの合計。eviction の 500MiB 判定に使う。 */
    val totalBytes: Long,
)

/** 送信間隔。値を決めるのは Web 側で、端末は nextConfig から受け取る。 */
enum class IntervalMinutes(val minutes: Int) {
    FIVE(5),
    TEN(10),
    FIFTEEN(15),
    ;

    companion object {
        /**
         * サーバから nextConfig が届くまでの既定値。
         *
         * **backend の internal/webapi/webapi.go の
         * `defaultInterval = api.IntervalOptions[0]` と同一でなければならない。**
         * 片方だけが変更されても機械的には検出できない。
         */
        val DEFAULT = FIVE

        fun fromMinutes(value: Int): IntervalMinutes? = entries.firstOrNull { it.minutes == value }
    }
}

/** 画面と常駐通知が同じものを出す。文言の生成は StatusLine が持つ。 */
sealed interface ObservationState {
    /** 直近の送信に成功している。 */
    data class Observing(val lastUploadAt: Instant?) : ObservationState

    /** 送信に失敗し、バッファに滞留している。 */
    data class Offline(val pendingCount: Int) : ObservationState

    /** サービスが動いていない。この状態のときだけ [観測を再開] を出す。 */
    data object Stopped : ObservationState
}
