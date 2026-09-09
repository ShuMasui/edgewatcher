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
 * **書き込みはファイルが先、行が後。** 逆にすると、行はあるがファイルが無い
 * 状態がプロセス死で生まれ、送信側がその行で詰まる。
 * **削除は行が先、ファイルが後。** 行が消えていればもう誰も参照しない。
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
        imageFile(observation.observationId).writeBytes(image)
        thumbnailFile(observation.observationId).writeBytes(thumbnail)
        dao.insert(
            ObservationEntity(
                observationId = observation.observationId,
                capturedAtMillis = observation.capturedAt.toEpochMilli(),
                lat = observation.coordinates?.lat,
                lng = observation.coordinates?.lng,
                totalBytes = observation.totalBytes,
            ),
        )
    }

    override suspend fun readImage(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) { imageFile(observationId).takeIf { it.exists() }?.readBytes() }

    override suspend fun readThumbnail(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) {
            thumbnailFile(observationId).takeIf { it.exists() }?.readBytes()
        }

    override suspend fun remove(observationId: String) = withContext(Dispatchers.IO) {
        dao.delete(observationId)
        imageFile(observationId).delete()
        thumbnailFile(observationId).delete()
        Unit
    }

    override suspend fun removeAll() = withContext(Dispatchers.IO) {
        dao.deleteAll()
        root.listFiles()?.forEach { it.delete() }
        Unit
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
