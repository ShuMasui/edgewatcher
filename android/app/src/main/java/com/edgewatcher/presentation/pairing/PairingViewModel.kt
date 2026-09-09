package com.edgewatcher.presentation.pairing

import android.os.Build
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgewatcher.BuildConfig
import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.usecase.PairDeviceUseCase
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class PairingViewModel @Inject constructor(
    private val pairDevice: PairDeviceUseCase,
) : ViewModel() {

    /** 画面に出す1行。null なら何も出さない。 */
    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    private val _paired = MutableStateFlow(false)
    val paired: StateFlow<Boolean> = _paired.asStateFlow()

    private var busy = false

    /** 未ペアリング画面へ戻された理由。黙って戻すと端末の故障と区別がつかない。 */
    fun showRevokedReason() {
        _message.value = "この端末は接続を解除されました。再度ペアリングしてください。"
    }

    fun onQrScanned(payload: String) {
        if (busy || _paired.value) return
        busy = true
        viewModelScope.launch {
            val outcome = pairDevice(payload, deviceInfo())
            when (outcome) {
                PairDeviceUseCase.Outcome.Paired -> {
                    _message.value = null
                    _paired.value = true
                }

                // 他のアプリの QR。何も出さずに読み取りを続ける。
                PairDeviceUseCase.Outcome.NotEdgeWatcherQr -> Unit

                is PairDeviceUseCase.Outcome.Rejected -> _message.value = when (outcome.reason) {
                    ApiFailure.PairingRejected.Reason.CONSUMED -> "この QR は使えません。"
                    ApiFailure.PairingRejected.Reason.EXPIRED -> "この QR の有効期限が切れています。"
                    ApiFailure.PairingRejected.Reason.INVALID -> "この QR は読み取れませんでした。"
                }

                PairDeviceUseCase.Outcome.Unavailable ->
                    _message.value = "接続できませんでした。通信状況を確認してもう一度お試しください。"
            }
            busy = false
        }
    }

    private fun deviceInfo() = DeviceInfo(
        model = Build.MODEL,
        osVersion = Build.VERSION.RELEASE,
        appVersion = BuildConfig.APP_VERSION,
    )
}
