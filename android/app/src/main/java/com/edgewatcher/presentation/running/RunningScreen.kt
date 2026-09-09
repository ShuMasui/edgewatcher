package com.edgewatcher.presentation.running

import android.content.ComponentName
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.hilt.navigation.compose.hiltViewModel
import com.edgewatcher.domain.StatusLine
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.infrastructure.service.ObservationService

/**
 * ペアリング済み画面。
 *
 * **通常は画像を一切表示しない。** 観測画像を見る場所は Web に一本化する。
 * [画角を合わせる] を押したときだけライブプレビューに切り替わり、もう一度で戻る。
 * 直近に送信した画像のサムネイルも出さない。ライブプレビューだけが例外なのは、
 * それが閲覧ではなく設置作業のための道具だから。
 *
 * 端末名は出さない。端末は自分の名前を知らない。
 */
@Composable
fun RunningScreen(
    onLoggedOut: () -> Unit,
    viewModel: RunningViewModel = hiltViewModel(),
) {
    val context = LocalContext.current
    var binder by remember { mutableStateOf<ObservationService.LocalBinder?>(null) }
    var previewing by remember { mutableStateOf(false) }
    var confirmingLogout by remember { mutableStateOf(false) }

    // bind は画面が繋がっている間だけの付加。Service の生存は
    // startForegroundService が担うので、BIND_AUTO_CREATE は付けない。
    DisposableEffect(context) {
        val connection = object : ServiceConnection {
            override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
                binder = service as? ObservationService.LocalBinder
            }

            override fun onServiceDisconnected(name: ComponentName?) {
                binder = null
            }
        }
        // フラグ 0。BIND_AUTO_CREATE を付けないのは、bind で Service を生成させない
        // ため。生存は startForegroundService が持ち、bind は覗き窓にすぎない。
        context.bindService(Intent(context, ObservationService::class.java), connection, 0)
        onDispose {
            // unbind の前に必ず外す。付けっぱなしにすると、閉じた画面の Surface を
            // Service が掴み続け、カメラが開いたまま発熱する。
            binder?.detachPreview()
            runCatching { context.unbindService(connection) }
            binder = null
        }
    }

    // binder は接続前 null になる。collectAsState を条件分岐の中で呼ぶと
    // Composable の呼び出し規則を破るため、produceState で包んで無条件に1回だけ呼ぶ。
    val state by produceState<ObservationState>(ObservationState.Stopped, binder) {
        val connected = binder
        if (connected == null) {
            value = ObservationState.Stopped
        } else {
            connected.state.collect { value = it }
        }
    }

    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(StatusLine.render(state))

        if (previewing) {
            AndroidView(
                modifier = Modifier.fillMaxWidth().weight(1f),
                factory = { ctx ->
                    PreviewView(ctx).also { view ->
                        binder?.attachPreview(view.surfaceProvider)
                    }
                },
            )
        }

        Button(
            onClick = {
                previewing = !previewing
                if (!previewing) binder?.detachPreview()
            },
        ) { Text(if (previewing) "プレビューを閉じる" else "画角を合わせる") }

        // 停止中のときだけ出す。Android 14 以降は再起動後に自動復帰できず、
        // これがないとアプリから観測を再開する手段がなくなる。
        if (state is ObservationState.Stopped) {
            Button(onClick = { ObservationService.start(context) }) { Text("観測を再開") }
        }

        TextButton(onClick = { confirmingLogout = true }) { Text("ログアウト") }
    }

    if (confirmingLogout) {
        AlertDialog(
            onDismissRequest = { confirmingLogout = false },
            title = { Text("ログアウトしますか？") },
            // 取り返しのつきやすさが Web と違うことを、ここで正直に伝える。
            text = {
                Text(
                    "再び観測するには、Web で QR を発行し直して、" +
                        "この端末をもう一度操作する必要があります。",
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingLogout = false
                        binder?.detachPreview()
                        ObservationService.stop(context)
                        viewModel.logout(onLoggedOut)
                    },
                ) { Text("ログアウト") }
            },
            dismissButton = {
                TextButton(onClick = { confirmingLogout = false }) { Text("キャンセル") }
            },
        )
    }
}
