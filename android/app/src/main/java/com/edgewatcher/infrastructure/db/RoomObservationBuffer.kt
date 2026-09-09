package com.edgewatcher.infrastructure.db

import com.edgewatcher.domain.model.Coordinates
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.port.ObservationBuffer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.time.Instant

/**
 * バッファの実物。Room の行とアプリ専用ストレージのファイルを対で扱う。
 *
 * **書き込みは行が先、ファイルが後。削除はファイルが先、行が後。**
 *
 * どちらもプロセス死の途中状態を「行はあるがファイルが無い」側に倒すための順序である。
 * その状態は UploadNextObservationUseCase が次の送信で検出し、行ごと捨てて自己修復する。
 * 逆向きにすると「ファイルはあるが行が無い」孤児が残るが、バッファの容量計算は DB の
 * 行から導かれるためディスク上の孤児が見えず、回収する経路がどこにも無い。
 * 数か月連続稼働する端末で、これは上限のない容量の増加になる。
 */
class RoomObservationBuffer(
    private val dao: ObservationDao,
    filesDir: File,
) : ObservationBuffer {

    private val root = File(filesDir, "observations").apply { mkdirs() }

    private fun imageFile(id: String) = File(root, "$id.jpg")
    private fun thumbnailFile(id: String) = File(root, "${id}_thumb.jpg")

    override suspend fun save(
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ) = withContext(Dispatchers.IO) {
        dao.insert(
            ObservationEntity(
                observationId = observation.observationId,
                capturedAtMillis = observation.capturedAt.toEpochMilli(),
                lat = observation.coordinates?.lat,
                lng = observation.coordinates?.lng,
                totalBytes = observation.totalBytes,
            ),
        )
        imageFile(observation.observationId).writeBytes(image)
        thumbnailFile(observation.observationId).writeBytes(thumbnail)
    }

    override suspend fun readImage(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) { imageFile(observationId).takeIf { it.exists() }?.readBytes() }

    override suspend fun readThumbnail(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) {
            thumbnailFile(observationId).takeIf { it.exists() }?.readBytes()
        }

    override suspend fun remove(observationId: String) = withContext(Dispatchers.IO) {
        imageFile(observationId).delete()
        thumbnailFile(observationId).delete()
        dao.delete(observationId)
    }

    override suspend fun removeAll() = withContext(Dispatchers.IO) {
        root.listFiles()?.forEach { it.delete() }
        dao.deleteAll()
    }

    override suspend fun count(): Int = dao.count()

    override suspend fun totalBytes(): Long = dao.totalBytes()

    override suspend fun allOldestFirst(): List<PendingObservation> =
        dao.allOldestFirst().map { row ->
            PendingObservation(
                observationId = row.observationId,
                capturedAt = Instant.ofEpochMilli(row.capturedAtMillis),
                coordinates = if (row.lat != null && row.lng != null) {
                    Coordinates(row.lat, row.lng)
                } else {
                    null
                },
                totalBytes = row.totalBytes,
            )
        }
}
