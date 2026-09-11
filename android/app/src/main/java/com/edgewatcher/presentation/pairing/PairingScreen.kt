package com.edgewatcher.presentation.pairing

import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.edgewatcher.presentation.component.AppBar
import com.edgewatcher.presentation.theme.EdgeWatcherColors
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage

/**
 * 未ペアリング画面。モックの `scanner()` に対応する。設定項目も履歴も持たない。
 *
 * ここで使うカメラは ObservationService のものではない。未ペアリングの間は
 * Service が動いていないので競合しない。ペアリングが成立したらこの画面は消え、
 * カメラの所有権は Service へ移る。
 */
@Composable
fun PairingScreen(
    onPaired: () -> Unit,
    /** Web から切断されて戻された直後なら true。理由を画面に残す。 */
    showRevokedReason: Boolean = false,
    viewModel: PairingViewModel = hiltViewModel(),
) {
    val message by viewModel.message.collectAsState()
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(showRevokedReason) {
        if (showRevokedReason) viewModel.showRevokedReason()
    }

    // 一度きりの出来事として受ける。保持された値を購読すると、ログアウトで
    // この画面へ戻った瞬間に前回の成立が再生され、稼働画面へ跳ね返される。
    LaunchedEffect(Unit) {
        viewModel.paired.collect { onPaired() }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(EdgeWatcherColors.Bg),
    ) {
        AppBar()

        message?.let { Banner(it) }

        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Color(0xFF05070A)),
            contentAlignment = Alignment.Center,
        ) {
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { ctx -> scannerView(ctx, lifecycleOwner, viewModel::onQrScanned) },
            )

            FrameBox()

            Column(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .padding(start = 22.dp, end = 22.dp, bottom = 30.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    "QR コードを読み取ってください",
                    style = MaterialTheme.typography.labelLarge,
                    color = EdgeWatcherColors.Text,
                    textAlign = TextAlign.Center,
                )
                Text(
                    "EdgeWatcher の Web 画面で「端末を追加」を\n押すと QR が表示されます。",
                    style = MaterialTheme.typography.bodySmall,
                    color = EdgeWatcherColors.Muted,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 5.dp),
                )
            }
        }
    }
}

/** モックの .banner.warn。戻された理由をここに残す。 */
@Composable
private fun Banner(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = Color(0xFFF0D9A4),
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = 18.dp, end = 18.dp, top = 14.dp)
            .background(Color(0x1AD29922), RoundedCornerShape(10.dp))
            .border(1.dp, Color(0x57D29922), RoundedCornerShape(10.dp))
            .padding(horizontal = 13.dp, vertical = 11.dp),
    )
}

/**
 * モックの .frame-box。四隅だけの枠で、読み取り位置の目安を出す。
 *
 * 角ごとに横棒と縦棒を1本ずつ置く。Compose の border は4辺まとめてしか引けず、
 * 負の padding ではみ出させて隠す小細工は Modifier.padding が負値を拒むので使えない。
 */
@Composable
private fun FrameBox() {
    Box(modifier = Modifier.size(190.dp)) {
        Corner(Alignment.TopStart, top = true, start = true)
        Corner(Alignment.TopEnd, top = true, start = false)
        Corner(Alignment.BottomStart, top = false, start = true)
        Corner(Alignment.BottomEnd, top = false, start = false)
    }
}

/** 角1つ。横棒と縦棒を1本ずつ置く。 */
@Composable
private fun BoxScope.Corner(align: Alignment, top: Boolean, start: Boolean) {
    val thickness = 3.dp
    val arm = 30.dp
    val accent = EdgeWatcherColors.Accent

    Box(modifier = Modifier.size(arm).align(align)) {
        Box(
            modifier = Modifier
                .align(if (top) Alignment.TopStart else Alignment.BottomStart)
                .size(width = arm, height = thickness)
                .background(accent),
        )
        Box(
            modifier = Modifier
                .align(if (start) Alignment.TopStart else Alignment.TopEnd)
                .size(width = thickness, height = arm)
                .background(accent),
        )
    }
}

private fun scannerView(
    ctx: android.content.Context,
    lifecycleOwner: androidx.lifecycle.LifecycleOwner,
    onQr: (String) -> Unit,
): PreviewView {
    val previewView = PreviewView(ctx)
    val scanner = BarcodeScanning.getClient()
    val future = ProcessCameraProvider.getInstance(ctx)

    future.addListener({
        val provider = future.get()
        val preview = Preview.Builder().build()
            .also { it.surfaceProvider = previewView.surfaceProvider }

        val analysis = ImageAnalysis.Builder()
            .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
            .build()

        analysis.setAnalyzer(ContextCompat.getMainExecutor(ctx)) { proxy ->
            val media = proxy.image
            if (media == null) {
                proxy.close()
            } else {
                val input = InputImage.fromMediaImage(media, proxy.imageInfo.rotationDegrees)
                scanner.process(input)
                    .addOnSuccessListener { codes ->
                        codes.firstOrNull { it.format == Barcode.FORMAT_QR_CODE }
                            ?.rawValue
                            ?.let(onQr)
                    }
                    .addOnCompleteListener { proxy.close() }
            }
        }

        provider.unbindAll()
        provider.bindToLifecycle(
            lifecycleOwner,
            CameraSelector.DEFAULT_BACK_CAMERA,
            preview,
            analysis,
        )
    }, ContextCompat.getMainExecutor(ctx))

    return previewView
}
