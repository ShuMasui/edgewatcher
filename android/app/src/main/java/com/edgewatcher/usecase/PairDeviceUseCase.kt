package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import kotlinx.coroutines.delay

/**
 * QR を deviceSecret と交換し、続けてセッションを取得する。
 *
 * 404 だけをリトライするのは、pairingCode の逆引きが結果整合な GSI2 を通るため。
 * QR 発行直後にスキャンすると伝播が間に合わず、**待てば成功する空振り**が起こりうる。
 * 409 を 404 に丸めると絶対に成功しない QR を回し続け、404 を 409 に丸めると
 * 成功するはずのペアリングを諦める。
 */
class PairDeviceUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
) {

    sealed interface Outcome {
        data object Paired : Outcome

        /** ew1: で始まらない。他のアプリの QR なので、黙って読み取りを続ける。 */
        data object NotEdgeWatcherQr : Outcome

        data class Rejected(val reason: ApiFailure.PairingRejected.Reason) : Outcome

        /** 通信できなかった / サーバ側の障害 / 伝播待ちが解消しなかった。 */
        data object Unavailable : Outcome
    }

    suspend operator fun invoke(qrPayload: String, deviceInfo: DeviceInfo): Outcome {
        if (!qrPayload.startsWith(QR_PREFIX)) return Outcome.NotEdgeWatcherQr
        val pairingCode = qrPayload.removePrefix(QR_PREFIX)
        if (pairingCode.isEmpty()) return Outcome.NotEdgeWatcherQr

        var retries = 0
        while (true) {
            when (val paired = api.pair(pairingCode, deviceInfo)) {
                is ApiResult.Success -> {
                    // deviceSecret はシステム全体でこれが平文で流れる唯一の場所。
                    // サーバは SHA-256 しか持たないため、失うと QR の再発行しかない。
                    store.writeCredentials(paired.value)
                    store.writeInterval(IntervalMinutes.DEFAULT)

                    return when (val session = api.issueSession(paired.value)) {
                        is ApiResult.Success -> {
                            store.writeSession(session.value)
                            Outcome.Paired
                        }
                        is ApiResult.Failure -> Outcome.Unavailable
                    }
                }

                is ApiResult.Failure -> when (val failure = paired.failure) {
                    ApiFailure.PairingNotFound -> {
                        if (retries >= MAX_RETRIES) return Outcome.Unavailable
                        retries += 1
                        delay(RETRY_INTERVAL_MILLIS)
                    }

                    is ApiFailure.PairingRejected -> return Outcome.Rejected(failure.reason)

                    else -> return Outcome.Unavailable
                }
            }
        }
    }

    companion object {
        /** QR のペイロードは "ew1:<pairingCode>"。deviceId は含まない。 */
        const val QR_PREFIX = "ew1:"

        /** 初回1回 + リトライ3回 = 最大4リクエスト。 */
        const val MAX_RETRIES = 3
        const val RETRY_INTERVAL_MILLIS = 1000L
    }
}
