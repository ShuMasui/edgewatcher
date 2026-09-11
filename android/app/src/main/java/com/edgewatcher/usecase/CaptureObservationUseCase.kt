package com.edgewatcher.usecase

import com.edgewatcher.domain.BufferPolicy
import com.edgewatcher.domain.ImagePolicy
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.port.CameraGateway
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.IdGenerator
import com.edgewatcher.domain.port.JpegEncoder
import com.edgewatcher.domain.port.LocationGateway
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * 撮影1周期。撮って、符号化して、位置を付けて、バッファへ積む。
 *
 * **送信はしない。** 送信は UploadNextObservationUseCase の仕事であり、
 * 分けておくことで「送信に失敗しても撮影は止まらない」が構造的に保たれる。
 */
class CaptureObservationUseCase(
    private val clock: Clock,
    private val ids: IdGenerator,
    private val camera: CameraGateway,
    private val encoder: JpegEncoder,
    private val location: LocationGateway,
    private val buffer: ObservationBuffer,
) {

    /** @return 採番した observationId。撮影できなければ null。 */
    suspend operator fun invoke(): String? {
        val capturedAt = clock.now()

        // 採番時刻は capturedAt に合わせる。サーバは日別クエリの範囲を ULID の
        // タイムスタンプ部から導くため、ここがずれると観測が別の日に並ぶ。
        val observationId = ids.ulid(capturedAt)

        val raw = runCatching { camera.capture() }.getOrNull() ?: return null

        val thumbnail = encoder.encode(
            raw,
            ImagePolicy.THUMBNAIL.longestSide,
            ImagePolicy.THUMBNAIL.quality,
        )

        // 段階を順に試し、合計が上限に収まった時点で採用する。送っても必ず 400 に
        // なるフレームをバッファに積まないため。通常は第1段階で収まる。
        var image: ByteArray? = null
        for (step in ImagePolicy.MAIN_STEPS) {
            val encoded = encoder.encode(raw, step.longestSide, step.quality)
            if (ImagePolicy.fits(encoded.size, thumbnail.size)) {
                image = encoded
                break
            }
            image = encoded
        }
        val accepted = image ?: return null
        if (!ImagePolicy.fits(accepted.size, thumbnail.size)) return null

        // 測位は待たない。定点観測デバイスは動かないので、最後の既知位置で十分。
        val coordinates = runCatching { location.lastKnown() }.getOrNull()

        val observation = PendingObservation(
            observationId = observationId,
            capturedAt = capturedAt,
            coordinates = coordinates,
            totalBytes = (accepted.size + thumbnail.size).toLong(),
        )
        buffer.save(observation, accepted, thumbnail)

        evict()
        return observationId
    }

    /** 撮影のたびに上限を評価し、超えていれば古い順に落とす。 */
    private suspend fun evict() {
        BufferPolicy.evictions(buffer.allOldestFirst()).forEach { buffer.remove(it) }
    }
}
