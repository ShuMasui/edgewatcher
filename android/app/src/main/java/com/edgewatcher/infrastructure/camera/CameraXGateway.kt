package com.edgewatcher.infrastructure.camera

import android.content.Context
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import com.edgewatcher.domain.port.CameraGateway
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * カメラの所有権を持つ。**Foreground Service だけがこれを保持する。**
 *
 * Android のカメラは同時に2つのクライアントが開けない。サービスが5分ごとに
 * 撮影する一方で画面が独立してプレビューを開こうとすると競合するため、
 * 所有権をここに一本化し、画面は attachPreview で Surface を渡すだけにする。
 *
 * ImageCapture は常に bind したままにし、Preview だけを付け外しする。
 * これによりプレビューのために定期送信を止める必要がなくなる。
 */
class CameraXGateway(private val context: Context) : CameraGateway {

    private var provider: ProcessCameraProvider? = null
    private var owner: LifecycleOwner? = null
    private var preview: Preview? = null

    private val imageCapture = ImageCapture.Builder()
        .setCaptureMode(ImageCapture.CAPTURE_MODE_MINIMIZE_LATENCY)
        .build()

    /** Service の起動時に1度だけ呼ぶ。 */
    suspend fun bindTo(lifecycleOwner: LifecycleOwner) {
        val cameraProvider = suspendCancellableCoroutine { continuation ->
            val future = ProcessCameraProvider.getInstance(context)
            future.addListener(
                { runCatching { future.get() }.fold(continuation::resume, continuation::resumeWithException) },
                ContextCompat.getMainExecutor(context),
            )
        }
        provider = cameraProvider
        owner = lifecycleOwner
        rebind()
    }

    fun attachPreview(surfaceProvider: Preview.SurfaceProvider) {
        preview = Preview.Builder().build().also { it.surfaceProvider = surfaceProvider }
        rebind()
    }

    fun detachPreview() {
        preview = null
        rebind()
    }

    fun release() {
        provider?.unbindAll()
        provider = null
        owner = null
        preview = null
    }

    /**
     * Preview + ImageCapture は CameraX が LEGACY レベルの端末でも
     * サポートを保証している組み合わせ。余剰端末を前提にする以上、
     * ここに他の use case を足さない。
     */
    private fun rebind() {
        val cameraProvider = provider ?: return
        val lifecycleOwner = owner ?: return
        cameraProvider.unbindAll()
        val useCases = listOfNotNull(imageCapture, preview).toTypedArray()
        cameraProvider.bindToLifecycle(
            lifecycleOwner,
            CameraSelector.DEFAULT_BACK_CAMERA,
            *useCases,
        )
    }

    override suspend fun capture(): ByteArray = suspendCancellableCoroutine { continuation ->
        imageCapture.takePicture(
            ContextCompat.getMainExecutor(context),
            object : ImageCapture.OnImageCapturedCallback() {
                override fun onCaptureSuccess(image: ImageProxy) {
                    val buffer = image.planes[0].buffer
                    val bytes = ByteArray(buffer.remaining())
                    buffer.get(bytes)
                    image.close()
                    continuation.resume(bytes)
                }

                override fun onError(exception: ImageCaptureException) {
                    continuation.resumeWithException(exception)
                }
            },
        )
    }
}
