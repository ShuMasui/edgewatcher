package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * 端末自身の資格情報を失効させる。
 *
 * 対象は認可された端末自身であり、本文で他の端末を指名することはできない。
 * 呼び出し側は確認ダイアログを挟むこと。復帰には Web からの QR 再発行と
 * 端末への物理的な操作が要るため、Web のログアウトとは取り返しやすさが違う。
 */
class LogoutDeviceUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
) {

    /**
     * @return サーバ側の失効に成功したか。false でもローカルは必ず消す。
     *   ユーザーが押したログアウトを、通信の都合で無かったことにはしない。
     */
    suspend operator fun invoke(): Boolean {
        val token = store.readSession()?.token
        val acknowledged = if (token == null) {
            false
        } else {
            api.logout(token) is ApiResult.Success
        }

        store.clear()
        buffer.removeAll()
        return acknowledged
    }
}
