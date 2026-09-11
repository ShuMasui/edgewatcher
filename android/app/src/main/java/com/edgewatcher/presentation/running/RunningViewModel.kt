package com.edgewatcher.presentation.running

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgewatcher.usecase.LogoutDeviceUseCase
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class RunningViewModel @Inject constructor(
    private val logoutDevice: LogoutDeviceUseCase,
) : ViewModel() {

    fun logout(onDone: () -> Unit) {
        viewModelScope.launch {
            // 戻り値は捨てる。サーバへ届かなくてもローカルは消えており、
            // ユーザーが押したログアウトを通信の都合で無かったことにはしない。
            logoutDevice()
            onDone()
        }
    }
}
