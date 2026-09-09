package com.edgewatcher.domain.error

/**
 * HTTP のステータスコードを、呼び出し側が取るべき行動へ写像する。
 *
 * **判断を本文に依存させない。** アプリケーション層のエラーは
 * `{"error":{"code","message"}}` だが、オーソライザと API Gateway が返す
 * 401 / 403 / 413 は `{"message":"..."}` である。パースに失敗しても
 * status だけで分類が決まること（spec §8.4）。
 */
object ApiFailures {

    /**
     * POST /device/uploads と POST /device/logout の分類。
     *
     * 4xx はすべてリトライ不能として捨てる。5xx を返さないのはサーバ側の
     * 約束であり、こちらが 4xx を再送すると、そのフレームで永久に詰まる。
     */
    fun forUpload(status: Int): ApiFailure = when {
        status == 401 || status == 403 -> ApiFailure.AuthExpired
        status in 400..499 -> ApiFailure.Discard
        else -> ApiFailure.Retryable
    }

    /**
     * POST /device/token の分類。
     *
     * **401 だけが資格情報を無効と断定できる唯一の応答である。**
     * サーバは失敗をすべて 401 に寄せており（404 も 403 も返らない）、
     * 5xx や通信断は「サーバ側の都合」であって切断ではない。
     */
    fun forTokenRefresh(status: Int): ApiFailure = when (status) {
        401 -> ApiFailure.CredentialsRevoked
        else -> ApiFailure.Retryable
    }

    /**
     * POST /device/pair の分類。
     *
     * 404 だけがリトライ対象。pairingCode の逆引きが結果整合な GSI2 を通るため、
     * QR 発行直後は伝播が間に合わないことがある。409 を 404 に丸めると絶対に
     * 成功しない QR を回し続け、404 を 409 に丸めると成功するはずのペアリングを諦める。
     *
     * `code` は 409 の内訳（消費済みか期限切れか）を分けるためだけに使い、
     * 欠けていても分類そのものは status で決まる。
     */
    fun forPairing(status: Int, code: String?): ApiFailure = when (status) {
        404 -> ApiFailure.PairingNotFound
        409 -> ApiFailure.PairingRejected(
            when (code) {
                "PAIRING_CODE_EXPIRED" -> ApiFailure.PairingRejected.Reason.EXPIRED
                else -> ApiFailure.PairingRejected.Reason.CONSUMED
            },
        )
        400 -> ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.INVALID)
        in 500..599 -> ApiFailure.Retryable
        else -> ApiFailure.Retryable
    }

    /** 通信できなかった。**資格情報を消す理由にはならない。** */
    fun forNetworkError(): ApiFailure = ApiFailure.Retryable
}
