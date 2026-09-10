package com.edgewatcher.presentation.running

import android.content.ComponentName
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.hilt.navigation.compose.hiltViewModel
import com.edgewatcher.domain.StatusLine
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.infrastructure.service.ObservationService
import com.edgewatcher.presentation.component.AppBar
import com.edgewatcher.presentation.component.GhostDangerButton
import com.edgewatcher.presentation.component.PrimaryButton
import com.edgewatcher.presentation.component.SecondaryButton
import com.edgewatcher.presentation.component.StatusCard
import com.edgewatcher.presentation.theme.EdgeWatcherColors

/**
 * ペアリング済み画面。モックの `running()` に対応する。
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
    val interval = binder?.interval ?: IntervalMinutes.DEFAULT

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(EdgeWatcherColors.Bg),
    ) {
        AppBar()

        Column(modifier = Modifier.fillMaxSize().padding(18.dp)) {
            // ライブプレビューのときだけ画像領域が現れる。モックと同じ 3:4。
            if (previewing) {
                LivePreview(
                    onSurfaceReady = { binder?.attachPreview(it) },
                    modifier = Modifier.fillMaxWidth().aspectRatio(3f / 4f),
                )
            }

            StatusCard(
                line = StatusLine.of(state, interval),
                modifier = Modifier.padding(
                    top = if (previewing) 14.dp else 0.dp,
                    bottom = 4.dp,
                ),
            )

            Box(modifier = Modifier.padding(top = 10.dp)) {
                when {
                    // 停止中だけ [観測を再開] を出す。Android 14+ では再起動後に
                    // 自動復帰できないため、この操作がないと再開手段がなくなる。
                    state is ObservationState.Stopped ->
                        PrimaryButton("観測を再開") { ObservationService.start(context) }

                    previewing -> PrimaryButton("通常表示に戻す") {
                        previewing = false
                        binder?.detachPreview()
                    }

                    else -> SecondaryButton("画角を合わせる") { previewing = true }
                }
            }

            // 権限とは別枠の2つの設定。満たされている間は何も出ないので、
            // 通常運用で画面が増えることはない。強制もしない。
            if (!isIgnoringBatteryOptimizations(context)) {
                TextButton(
                    onClick = { requestIgnoreBatteryOptimizations(context) },
                    modifier = Modifier.fillMaxWidth(),
                ) { Text("バッテリー最適化の対象から外す", fontSize = 12.sp) }
            }

            if (!canScheduleExactAlarms(context)) {
                TextButton(
                    onClick = { requestExactAlarmPermission(context) },
                    modifier = Modifier.fillMaxWidth(),
                ) { Text("正確なアラームを許可する", fontSize = 12.sp) }
            }

            // モックの .spacer。ログアウトを最下部へ押しやる。
            Box(modifier = Modifier.weight(1f))

            GhostDangerButton("ログアウト") { confirmingLogout = true }
        }
    }

    if (confirmingLogout) {
        AlertDialog(
            onDismissRequest = { confirmingLogout = false },
            containerColor = EdgeWatcherColors.Panel,
            title = { Text("ログアウトしますか？", style = MaterialTheme.typography.titleMedium) },
            // 取り返しのつきやすさが Web と違うことを、ここで正直に伝える。
            text = {
                Text(
                    "再び観測するには、Web で QR を発行し直して、" +
                        "この端末をもう一度操作する必要があります。",
                    style = MaterialTheme.typography.bodySmall,
                    color = EdgeWatcherColors.Muted,
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingLogout = false
                        previewing = false
                        binder?.detachPreview()
                        ObservationService.stop(context)
                        viewModel.logout(onLoggedOut)
                    },
                ) { Text("ログアウト", color = EdgeWatcherColors.Danger) }
            },
            dismissButton = {
                TextButton(onClick = { confirmingLogout = false }) {
                    Text("キャンセル", color = EdgeWatcherColors.Muted)
                }
            },
        )
    }
}

/** モックの .shot + .reticle + .live。設置作業のための道具なので構図の目安を重ねる。 */
@Composable
private fun LivePreview(
    onSurfaceReady: (androidx.camera.core.Preview.SurfaceProvider) -> Unit,
    modifier: Modifier = Modifier,
) {
    Box(modifier = modifier.clip(RoundedCornerShape(12.dp)).background(Color.Black)) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx -> PreviewView(ctx).also { onSurfaceReady(it.surfaceProvider) } },
        )

        // 三分割の目安。屋外設置で水平と構図を合わせるために置く。
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(14.dp)
                .border(1.dp, Color.White.copy(alpha = 0.35f), RoundedCornerShape(6.dp)),
        )

        Text(
            text = "LIVE",
            color = Color.White,
            fontSize = 10.sp,
            fontWeight = FontWeight.Bold,
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(10.dp)
                .background(Color(0xEBF85149), RoundedCornerShape(6.dp))
                .padding(horizontal = 9.dp, vertical = 3.dp),
        )
    }
}
