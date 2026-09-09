package com.edgewatcher.infrastructure.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class PairRequest(
    val pairingCode: String,
    val deviceInfo: DeviceInfoDto,
)

@Serializable
data class DeviceInfoDto(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

@Serializable
data class PairResponse(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
data class TokenRequest(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
data class TokenResponse(
    val sessionToken: String,
    val expiresAt: Long,
)

/** multipart の metadata パート。**deviceId は入れない。** 入れてもサーバは読まない。 */
@Serializable
data class UploadMetadataDto(
    val observationId: String,
    val capturedAt: String,
    val lat: Double? = null,
    val lng: Double? = null,
)

@Serializable
data class UploadResponse(
    val nextConfig: NextConfigDto,
)

@Serializable
data class NextConfigDto(
    val intervalMinutes: Int,
)

/**
 * アプリケーション層のエラー本文。
 *
 * オーソライザと API Gateway が返す 401 / 403 / 413 はこの形ではなく
 * `{"message":"..."}` である。**したがって、これのパースに失敗しても
 * 分類が決まらなければならない。**
 */
@Serializable
data class ErrorEnvelope(
    @SerialName("error") val error: ErrorBody,
) {
    @Serializable
    data class ErrorBody(val code: String, val message: String)
}
