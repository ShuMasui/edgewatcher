package com.edgewatcher.infrastructure.api

import okhttp3.MultipartBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.Header
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.Part

/**
 * 端末群4本。
 *
 * 応答を Response<T> で受けるのは、**status を自分で読む必要があるため**。
 * Retrofit に例外へ変換させると、401 と 403 と 413 の区別が失われる。
 */
interface EdgeWatcherService {

    @POST("device/pair")
    suspend fun pair(@Body body: PairRequest): Response<PairResponse>

    @POST("device/token")
    suspend fun token(@Body body: TokenRequest): Response<TokenResponse>

    /**
     * パート名は image / thumbnail / metadata。契約が縛っている箇所なので変えない。
     * Authorization は **Bearer を付けない素のトークン**。
     */
    @Multipart
    @POST("device/uploads")
    suspend fun upload(
        @Header("Authorization") sessionToken: String,
        @Part image: MultipartBody.Part,
        @Part thumbnail: MultipartBody.Part,
        @Part metadata: MultipartBody.Part,
    ): Response<UploadResponse>

    @POST("device/logout")
    suspend fun logout(@Header("Authorization") sessionToken: String): Response<Unit>
}
