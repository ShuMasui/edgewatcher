package com.edgewatcher.presentation.pairing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.usecase.PairDeviceUseCase
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class PairingViewModel @Inject constructor(
    private val pairDevice: PairDeviceUseCase,
    private val deviceInfo: DeviceInfo,
) : ViewModel() {

    /** 画面に出す1行。null なら何も出さない。 */
    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    /**
     * ペアリングが成立したことを**一度だけ**知らせる。
     *
     * ここが保持される状態（StateFlow<Boolean>）だと、ログアウトして未ペアリング画面へ
     * 戻った瞬間に前回の成立が再生され、稼働画面へ跳ね返される。この ViewModel は
     * ナビゲーションを持たない構成では Activity に紐づいて生き残るため、画面を離れても
     * 値が消えないことがその不具合の前提になる。出来事は出来事として1回だけ流す。
     */
    private val _paired = Channel<Unit>(Channel.BUFFERED)
    val paired: Flow<Unit> = _paired.receiveAsFlow()

    /** 送信中の二重起動だけを防ぐ。**成立済みを恒久的に覚えてはならない** */
    private var busy = false

    /** 未ペアリング画面へ戻された理由。黙って戻すと端末の故障と区別がつかない。 */
    fun showRevokedReason() {
        _message.value = "この端末は接続を解除されました。再度ペアリングしてください。"
    }

    fun onQrScanned(payload: String) {
        if (busy) return
        busy = true
        viewModelScope.launch {
            when (val outcome = pairDevice(payload, deviceInfo)) {
                PairDeviceUseCase.Outcome.Paired -> {
                    _message.value = null
                    _paired.send(Unit)
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
}
