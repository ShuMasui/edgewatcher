package com.edgewatcher.domain.error

import org.junit.Assert.assertEquals
import org.junit.Test

class ApiFailuresTest {

    // ---- アップロード（POST /device/uploads） ----

    @Test
    fun `upload 400 is discarded, never retried`() {
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(400))
    }

    @Test
    fun `upload 413 is discarded, never retried`() {
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(413))
    }

    @Test
    fun `upload 401 and 403 both mean the session expired`() {
        // 端末ルートでセッションが切れたときオーソライザが返すのは 403。
        // 401 だけを見ていると、セッション失効から永久に回復できない。
        assertEquals(ApiFailure.AuthExpired, ApiFailures.forUpload(401))
        assertEquals(ApiFailure.AuthExpired, ApiFailures.forUpload(403))
    }

    @Test
    fun `upload 5xx is retryable`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(500))
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(502))
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(503))
    }

    @Test
    fun `an unmapped 4xx on upload is discarded, not retried`() {
        // 本文に起因する失敗を再送し続けると、そのフレームで永久に詰まる。
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(422))
    }

    // ---- セッション再取得（POST /device/token） ----

    @Test
    fun `token refresh 401 revokes the credentials`() {
        assertEquals(ApiFailure.CredentialsRevoked, ApiFailures.forTokenRefresh(401))
    }

    @Test
    fun `token refresh 5xx does NOT revoke the credentials`() {
        // これを取り違えると、圏外になっただけの端末が deviceSecret を捨て、
        // 復帰に Web からの QR 再発行と現地への訪問が必要になる。
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(500))
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(503))
    }

    @Test
    fun `token refresh only revokes on 401, never on another 4xx`() {
        // POST /device/token documents a 400 (VALIDATION_ERROR). 401 is the ONLY
        // response that proves the deviceSecret is invalid; widening the branch to
        // any 4xx would erase credentials on a malformed request, and recovery
        // needs a QR reissued on the web plus a trip to the mounting site.
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(400))
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(403))
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(404))
    }

    @Test
    fun `a network error never revokes the credentials`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forNetworkError())
    }

    // ---- ペアリング（POST /device/pair） ----

    @Test
    fun `pairing 404 is retryable because the lookup goes through an eventually consistent index`() {
        assertEquals(ApiFailure.PairingNotFound, ApiFailures.forPairing(404, "PAIRING_NOT_FOUND"))
    }

    @Test
    fun `pairing 409 is rejected outright and distinguishes consumed from expired`() {
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.CONSUMED),
            ApiFailures.forPairing(409, "PAIRING_CODE_CONSUMED"),
        )
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.EXPIRED),
            ApiFailures.forPairing(409, "PAIRING_CODE_EXPIRED"),
        )
    }

    @Test
    fun `pairing 400 is rejected as invalid`() {
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.INVALID),
            ApiFailures.forPairing(400, "VALIDATION_ERROR"),
        )
    }

    @Test
    fun `pairing 5xx is retryable`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forPairing(500, null))
    }

    @Test
    fun `pairing classification does not depend on the body`() {
        // 本文の形は2種類あり、パースに失敗することがある。
        // status だけで分類が決まらなければならない。
        assertEquals(ApiFailure.PairingNotFound, ApiFailures.forPairing(404, null))
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.CONSUMED),
            ApiFailures.forPairing(409, null),
        )
    }
}
