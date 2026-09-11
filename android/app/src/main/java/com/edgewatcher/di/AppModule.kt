package com.edgewatcher.di

import android.content.Context
import android.os.Build
import androidx.room.Room
import com.edgewatcher.BuildConfig
import com.edgewatcher.domain.port.AlarmScheduler
import com.edgewatcher.domain.port.CameraGateway
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.IdGenerator
import com.edgewatcher.domain.port.JpegEncoder
import com.edgewatcher.domain.port.LocationGateway
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer
import com.edgewatcher.infrastructure.SystemClock
import com.edgewatcher.infrastructure.UlidIdGenerator
import com.edgewatcher.infrastructure.api.EdgeWatcherService
import com.edgewatcher.infrastructure.api.RetrofitObservationApi
import com.edgewatcher.infrastructure.camera.AndroidJpegEncoder
import com.edgewatcher.infrastructure.camera.CameraXGateway
import com.edgewatcher.infrastructure.db.ObservationDatabase
import com.edgewatcher.infrastructure.db.RoomObservationBuffer
import com.edgewatcher.infrastructure.location.FusedLocationGateway
import com.edgewatcher.infrastructure.service.AndroidAlarmScheduler
import com.edgewatcher.infrastructure.store.EncryptedCredentialStore
import com.edgewatcher.usecase.CaptureObservationUseCase
import com.edgewatcher.usecase.LogoutDeviceUseCase
import com.edgewatcher.usecase.PairDeviceUseCase
import com.edgewatcher.usecase.RefreshSessionUseCase
import com.edgewatcher.usecase.UploadNextObservationUseCase
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.util.concurrent.TimeUnit
import javax.inject.Singleton

/**
 * 依存の組み立ては最外装のここだけで行う。
 *
 * domain と usecase はコンストラクタ注入だけで成立しており、Hilt を知らない。
 * この向きを保っている限り、レイヤの規約は機械的に守られる。
 */
@Module
@InstallIn(SingletonComponent::class)
object AppModule {

    @Provides
    @Singleton
    fun json(): Json = Json { ignoreUnknownKeys = true; encodeDefaults = false }

    @Provides
    @Singleton
    fun okHttp(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS)
        .build()

    @Provides
    @Singleton
    fun service(client: OkHttpClient, json: Json): EdgeWatcherService = Retrofit.Builder()
        // BuildConfig.API_BASE_URL は末尾スラッシュで終わること。Retrofit の要求。
        .baseUrl(BuildConfig.API_BASE_URL.trimEnd('/') + "/")
        .client(client)
        .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
        .build()
        .create(EdgeWatcherService::class.java)

    @Provides
    @Singleton
    fun api(service: EdgeWatcherService, json: Json): ObservationApi =
        RetrofitObservationApi(service, json)

    @Provides
    @Singleton
    fun credentialStore(@ApplicationContext context: Context): CredentialStore =
        EncryptedCredentialStore(context)

    @Provides
    @Singleton
    fun database(@ApplicationContext context: Context): ObservationDatabase =
        Room.databaseBuilder(context, ObservationDatabase::class.java, "observations.db")
            // スキーマは v1 のみ。壊れたら作り直す。バッファを失っても観測が数枚欠けるだけ。
            .fallbackToDestructiveMigration(dropAllTables = true)
            .build()

    @Provides
    @Singleton
    fun buffer(@ApplicationContext context: Context, db: ObservationDatabase): ObservationBuffer =
        RoomObservationBuffer(db.observations(), context.filesDir)

    @Provides
    @Singleton
    fun cameraGateway(@ApplicationContext context: Context): CameraXGateway =
        CameraXGateway(context)

    @Provides
    @Singleton
    fun camera(gateway: CameraXGateway): CameraGateway = gateway

    @Provides
    @Singleton
    fun encoder(): JpegEncoder = AndroidJpegEncoder()

    @Provides
    @Singleton
    fun location(@ApplicationContext context: Context): LocationGateway =
        FusedLocationGateway(context)

    @Provides
    @Singleton
    fun alarms(@ApplicationContext context: Context): AlarmScheduler =
        AndroidAlarmScheduler(context)

    @Provides
    @Singleton
    fun clock(): Clock = SystemClock()

    @Provides
    @Singleton
    fun ids(): IdGenerator = UlidIdGenerator()

    /**
     * 端末が自己申告する情報。ペアリングのときだけ使う表示専用の値で、
     * 何の認可判断にも関わらない。ここで組み立てるのは、ViewModel から
     * android.os.Build への依存を外して素の Kotlin として試験できるようにするため。
     */
    @Provides
    @Singleton
    fun deviceInfo(): DeviceInfo = DeviceInfo(
        model = Build.MODEL,
        osVersion = Build.VERSION.RELEASE,
        appVersion = BuildConfig.APP_VERSION,
    )

    @Provides
    fun pairDevice(api: ObservationApi, store: CredentialStore) =
        PairDeviceUseCase(api, store)

    @Provides
    fun refreshSession(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
    ) = RefreshSessionUseCase(api, store, buffer)

    @Provides
    fun logoutDevice(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
    ) = LogoutDeviceUseCase(api, store, buffer)

    @Provides
    fun captureObservation(
        clock: Clock,
        ids: IdGenerator,
        camera: CameraGateway,
        encoder: JpegEncoder,
        location: LocationGateway,
        buffer: ObservationBuffer,
    ) = CaptureObservationUseCase(clock, ids, camera, encoder, location, buffer)

    @Provides
    fun uploadNext(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
        clock: Clock,
        refreshSession: RefreshSessionUseCase,
    ) = UploadNextObservationUseCase(api, store, buffer, clock, refreshSession)
}
