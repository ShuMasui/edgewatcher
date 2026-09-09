package com.edgewatcher.infrastructure.api

import com.edgewatcher.domain.error.ApiFailures
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.ObservationApi
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.Response
import java.io.IOException
import java.time.format.DateTimeFormatter

/**
 * ObservationApi の実物。**HTTP のステータスコードを domain の外へ出さない。**
 *
 * 分類は ApiFailures が行い、ここは呼び出しと本文の組み立てだけを担う。
 */
class RetrofitObservationApi(
    private val service: EdgeWatcherService,
    private val json: Json,
) : ObservationApi {

    override suspend fun pair(
        pairingCode: String,
        deviceInfo: DeviceInfo,
    ): ApiResult<DeviceCredentials> = try {
        val response = service.pair(
            PairRequest(
                pairingCode = pairingCode,
                deviceInfo = DeviceInfoDto(
                    model = deviceInfo.model,
                    osVersion = deviceInfo.osVersion,
                    appVersion = deviceInfo.appVersion,
                ),
            ),
        )
        val body = response.body()
        if (response.isSuccessful && body != null) {
            ApiResult.Success(DeviceCredentials(body.deviceId, body.deviceSecret))
        } else {
            ApiResult.Failure(ApiFailures.forPairing(response.code(), errorCodeOf(response)))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun issueSession(
        credentials: DeviceCredentials,
    ): ApiResult<Session> = try {
        val response = service.token(
            TokenRequest(credentials.deviceId, credentials.deviceSecret),
        )
        val body = response.body()
        if (response.isSuccessful && body != null) {
            ApiResult.Success(Session(body.sessionToken, body.expiresAt))
        } else {
            ApiResult.Failure(ApiFailures.forTokenRefresh(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun upload(
        sessionToken: String,
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ): ApiResult<IntervalMinutes> = try {
        val metadata = UploadMetadataDto(
            observationId = observation.observationId,
            capturedAt = ISO_8601.format(observation.capturedAt.atZone(java.time.ZoneOffset.UTC)),
            lat = observation.coordinates?.lat,
            lng = observation.coordinates?.lng,
        )

        val response = service.upload(
            sessionToken = sessionToken,
            image = jpegPart("image", observation.observationId + ".jpg", image),
            thumbnail = jpegPart("thumbnail", observation.observationId + "_thumb.jpg", thumbnail),
            metadata = MultipartBody.Part.createFormData(
                "metadata",
                null,
                json.encodeToString(UploadMetadataDto.serializer(), metadata)
                    .toRequestBody(JSON_MEDIA_TYPE),
            ),
        )

        val body = response.body()
        if (response.isSuccessful && body != null) {
            val interval = IntervalMinutes.fromMinutes(body.nextConfig.intervalMinutes)
                ?: IntervalMinutes.DEFAULT
            ApiResult.Success(interval)
        } else {
            ApiResult.Failure(ApiFailures.forUpload(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun logout(sessionToken: String): ApiResult<Unit> = try {
        val response = service.logout(sessionToken)
        if (response.isSuccessful) {
            ApiResult.Success(Unit)
        } else {
            ApiResult.Failure(ApiFailures.forUpload(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    private fun jpegPart(name: String, filename: String, bytes: ByteArray): MultipartBody.Part =
        MultipartBody.Part.createFormData(name, filename, bytes.toRequestBody(JPEG_MEDIA_TYPE))

    /**
     * 本文から code を読む。**読めなくてもよい。**
     * 401 / 403 / 413 は本文の形が違い、パースに失敗する。分類は status で決まる。
     */
    private fun errorCodeOf(response: Response<*>): String? = runCatching {
        val raw = response.errorBody()?.string() ?: return null
        json.decodeFromString(ErrorEnvelope.serializer(), raw).error.code
    }.getOrNull()

    private companion object {
        val JPEG_MEDIA_TYPE = "image/jpeg".toMediaType()
        val JSON_MEDIA_TYPE = "application/json".toMediaType()
        val ISO_8601: DateTimeFormatter = DateTimeFormatter.ISO_OFFSET_DATE_TIME
    }
}
