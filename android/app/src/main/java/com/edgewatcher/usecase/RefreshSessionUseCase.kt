package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * セッションを取り直す。端末の自己修復の唯一の入口。
 *
 * 401 と 403 の両方がここへ来る。端末ルートでセッションが失効したとき
 * オーソライザが返すのは 403 であり、401 は Authorization ヘッダが無い場合。
 * 401 だけを見ていると、セッション失効から永久に回復できない。
 */
class RefreshSessionUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
) {

    sealed interface Outcome {
        data class Refreshed(val session: Session) : Outcome

        /** deviceSecret が無効。Web から切断されたか、端末が削除された。 */
        data object Revoked : Outcome

        /** 通信できなかった / サーバ側の障害。**何も消していない。** */
        data object Unavailable : Outcome
    }

    suspend operator fun invoke(): Outcome {
        val credentials = store.readCredentials() ?: return Outcome.Revoked

        return when (val result = api.issueSession(credentials)) {
            is ApiResult.Success -> {
                store.writeSession(result.value)
                Outcome.Refreshed(result.value)
            }

            is ApiResult.Failure -> when (result.failure) {
                // POST /device/token の 401 だけが「無効」を断定できる唯一の応答。
                ApiFailure.CredentialsRevoked -> {
                    store.clear()
                    buffer.removeAll()
                    Outcome.Revoked
                }

                // 5xx・タイムアウト・通信断では絶対に消さない。取り違えると、
                // 圏外になっただけの端末が deviceSecret を捨て、復帰に Web からの
                // QR 再発行と現地への物理的な訪問が必要になる。
                else -> Outcome.Unavailable
            }
        }
    }
}
