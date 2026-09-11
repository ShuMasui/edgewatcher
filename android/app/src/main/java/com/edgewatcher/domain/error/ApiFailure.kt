package com.edgewatcher.domain.error

/**
 * API 呼び出しの失敗を、**呼び出し側が取るべき行動**で分類したもの。
 *
 * HTTP のステータスコードを domain の外へ漏らさないための型でもある。
 * 分類の判断は本文に依存させない（本文の形が2種類あるため。spec §8.4）。
 */
sealed interface ApiFailure {
    /** 5xx・タイムアウト・通信断。バックオフして同じものを再試行してよい。 */
    data object Retryable : ApiFailure

    /** 400 / 413、および解釈できない 4xx。本文に起因するため、再送しても永久に通らない。捨てる。 */
    data object Discard : ApiFailure

    /** 401 / 403。セッションが切れた。POST /device/token で再取得する。 */
    data object AuthExpired : ApiFailure

    /**
     * POST /device/token が 401 を返した。deviceSecret が無効。
     *
     * **資格情報を消してよいのはこれを受け取ったときだけである。**
     */
    data object CredentialsRevoked : ApiFailure

    /** POST /device/pair の 404。GSI2 の伝播待ちなので、待てば成功しうる。 */
    data object PairingNotFound : ApiFailure

    /** QR が使用済み・期限切れ・本文不正。リトライしても解決しない。 */
    data class PairingRejected(val reason: Reason) : ApiFailure {
        enum class Reason { CONSUMED, EXPIRED, INVALID }
    }
}

sealed interface ApiResult<out T> {
    data class Success<T>(val value: T) : ApiResult<T>
    data class Failure(val failure: ApiFailure) : ApiResult<Nothing>
}
