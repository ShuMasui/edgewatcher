package com.edgewatcher.presentation

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.infrastructure.service.ObservationService
import com.edgewatcher.presentation.pairing.PairingScreen
import com.edgewatcher.presentation.permission.PermissionScreen
import com.edgewatcher.presentation.running.RunningScreen
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

/**
 * 単一 Activity。画面の選択はこれだけ。
 *
 * 資格情報を持たない → QR スキャナ / 持つ → 稼働画面。
 * 初回起動時は、未ペアリング画面に入る前に権限を通す。
 */
@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    @Inject lateinit var store: CredentialStore

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            var permitted by remember { mutableStateOf(false) }
            var paired by remember { mutableStateOf(store.readCredentials() != null) }
            var revoked by remember { mutableStateOf(false) }

            // Web から切断されると Service が知らせてくる。画面はそれを受けて
            // 未ペアリングへ戻し、**理由を残す。** 黙って QR スキャナへ戻すと、
            // オーナーには端末の故障と区別がつかない。
            LaunchedEffect(Unit) {
                ObservationService.revoked.collect {
                    paired = false
                    revoked = true
                }
            }

            MaterialTheme {
                Surface {
                    when {
                        !permitted -> PermissionScreen(onGranted = { permitted = true })

                        !paired -> PairingScreen(
                            showRevokedReason = revoked,
                            onPaired = {
                                paired = true
                                revoked = false
                                ObservationService.start(this@MainActivity)
                            },
                        )

                        else -> RunningScreen(onLoggedOut = { paired = false })
                    }
                }
            }
        }
    }
}
