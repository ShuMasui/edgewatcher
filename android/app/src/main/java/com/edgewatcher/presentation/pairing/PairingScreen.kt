package com.edgewatcher.presentation.pairing

import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage

/**
 * 未ペアリング画面。QR スキャナが全画面。設定項目も履歴も持たない。
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
    val paired by viewModel.paired.collectAsState()
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(showRevokedReason) {
        if (showRevokedReason) viewModel.showRevokedReason()
    }

    LaunchedEffect(paired) {
        if (paired) onPaired()
    }

    Box(modifier = Modifier.fillMaxSize()) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
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
                            val input = InputImage.fromMediaImage(
                                media,
                                proxy.imageInfo.rotationDegrees,
                            )
                            scanner.process(input)
                                .addOnSuccessListener { codes ->
                                    codes.firstOrNull { it.format == Barcode.FORMAT_QR_CODE }
                                        ?.rawValue
                                        ?.let(viewModel::onQrScanned)
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

                previewView
            },
        )

        message?.let {
            Text(
                text = it,
                modifier = Modifier.align(Alignment.BottomCenter).padding(24.dp),
            )
        }
    }
}
