package com.edgewatcher.usecase

import com.edgewatcher.domain.UploadOrder
import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * バッファの先頭を1件送る。
 *
 * 呼び出し側はこれを Outcome が Empty か Deferred になるまで繰り返す。
 * 1件ずつなのは、途中で撮影が割り込んでも順序の判断が常に最新のバッファに
 * 基づくようにするため。
 */
class UploadNextObservationUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
    private val clock: Clock,
    private val refreshSession: RefreshSessionUseCase,
) {

    sealed interface Outcome {
        data class Sent(val observationId: String, val pendingCount: Int) : Outcome

        /** 送るものが無い。 */
        data object Empty : Outcome

        /** 今は送れない。バックオフして後で来ること。**何も失っていない。** */
        data object Deferred : Outcome

        /** deviceSecret が無効になった。呼び出し側は未ペアリング画面へ戻す。 */
        data object Revoked : Outcome
    }

    private sealed interface Token {
        data class Ready(val value: String) : Token
        data object Revoked : Token
        data object Unavailable : Token
    }

    suspend operator fun invoke(): Outcome {
        val next = UploadOrder.order(buffer.allOldestFirst()).firstOrNull() ?: return Outcome.Empty

        val token = when (val t = ensureFreshSession()) {
            is Token.Ready -> t.value
            Token.Revoked -> return Outcome.Revoked
            Token.Unavailable -> return Outcome.Deferred
        }

        val image = buffer.readImage(next.observationId)
        val thumbnail = buffer.readThumbnail(next.observationId)
        if (image == null || thumbnail == null) {
            // 行はあるがファイルが無い。対で扱う約束が破れているので、行ごと捨てる。
            buffer.remove(next.observationId)
            return Outcome.Deferred
        }

        return when (val result = api.upload(token, next, image, thumbnail)) {
            is ApiResult.Success -> {
                // nextConfig は必ず適用する。サーバは設定を push しないため、
                // 送信間隔の変更がオーナーから端末へ届く経路はこれ1本しかない。
                store.writeInterval(result.value)
                buffer.remove(next.observationId)
                Outcome.Sent(next.observationId, buffer.count())
            }

            is ApiResult.Failure -> when (result.failure) {
                // 400 / 413。再送しても永久に通らないので、そのフレームを捨てて次へ進む。
                ApiFailure.Discard -> {
                    buffer.remove(next.observationId)
                    Outcome.Deferred
                }

                // 401 / 403。セッションを取り直し、次の周回で同じ行を再送する。
                ApiFailure.AuthExpired -> when (refreshSession()) {
                    is RefreshSessionUseCase.Outcome.Refreshed -> Outcome.Deferred
                    RefreshSessionUseCase.Outcome.Revoked -> Outcome.Revoked
                    RefreshSessionUseCase.Outcome.Unavailable -> Outcome.Deferred
                }

                else -> Outcome.Deferred
            }
        }
    }

    /**
     * 有効なセッションを返す。期限が近ければ先に取り直す。
     * 12時間ごとに1往復が無駄になるのを避けるためで、必須ではないが安い。
     */
    private suspend fun ensureFreshSession(): Token {
        val current = store.readSession()
        val nowSeconds = clock.now().epochSecond
        if (current != null && current.expiresAtEpochSeconds - nowSeconds > REFRESH_MARGIN_SECONDS) {
            return Token.Ready(current.token)
        }
        return when (val outcome = refreshSession()) {
            is RefreshSessionUseCase.Outcome.Refreshed -> Token.Ready(outcome.session.token)
            RefreshSessionUseCase.Outcome.Revoked -> Token.Revoked
            RefreshSessionUseCase.Outcome.Unavailable -> Token.Unavailable
        }
    }

    private companion object {
        const val REFRESH_MARGIN_SECONDS = 300L
    }
}
