package com.edgewatcher.domain.port

import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.Coordinates
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.model.Session
import java.time.Instant

interface Clock {
    fun now(): Instant
}

interface IdGenerator {
    /** 採番時刻を必ず受け取る。引数なしのオーバーロードを足さないこと。 */
    fun ulid(at: Instant): String
}

/** 資格情報と、サーバから受け取った設定の保管。 */
interface CredentialStore {
    fun readCredentials(): DeviceCredentials?
    fun writeCredentials(credentials: DeviceCredentials)
    fun readSession(): Session?
    fun writeSession(session: Session)
    fun readInterval(): IntervalMinutes
    fun writeInterval(interval: IntervalMinutes)

    /** 資格情報・セッション・設定をすべて消す。 */
    fun clear()
}

/**
 * 送信待ちの観測の保管。**行と画像ファイルを対で扱う。**
 * 片方だけが残る状態を外から作れないようにする責務がここにある。
 */
interface ObservationBuffer {
    suspend fun save(observation: PendingObservation, image: ByteArray, thumbnail: ByteArray)
    suspend fun readImage(observationId: String): ByteArray?
    suspend fun readThumbnail(observationId: String): ByteArray?
    suspend fun remove(observationId: String)
    suspend fun removeAll()
    suspend fun count(): Int
    suspend fun totalBytes(): Long

    /** capturedAt の昇順。BufferPolicy と UploadOrder はこの順序を前提にする。 */
    suspend fun allOldestFirst(): List<PendingObservation>
}

interface ObservationApi {
    suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): ApiResult<DeviceCredentials>
    suspend fun issueSession(credentials: DeviceCredentials): ApiResult<Session>

    /** 成功すると nextConfig の間隔が返る。**必ず適用すること。** */
    suspend fun upload(
        sessionToken: String,
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ): ApiResult<IntervalMinutes>

    suspend fun logout(sessionToken: String): ApiResult<Unit>
}

/** カメラの所有権は Foreground Service が持つ。これはその窓口。 */
interface CameraGateway {
    /** 1枚撮る。フル解像度の JPEG を返す。 */
    suspend fun capture(): ByteArray
}

/** 言われた寸法で符号化するだけ。何段階で落とすかは ImagePolicy が決める。 */
interface JpegEncoder {
    fun encode(source: ByteArray, longestSide: Int, quality: Int): ByteArray
}

interface LocationGateway {
    /** 最後の既知位置。**測位を待たない。** 取得できなければ null。 */
    suspend fun lastKnown(): Coordinates?
}

interface AlarmScheduler {
    fun scheduleNext(at: Instant)
    fun cancel()
}
