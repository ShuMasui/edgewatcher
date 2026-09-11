package com.edgewatcher.infrastructure.service

import android.app.Notification
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.camera.core.Preview
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.domain.port.AlarmScheduler
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationBuffer
import com.edgewatcher.infrastructure.camera.CameraXGateway
import com.edgewatcher.usecase.CaptureObservationUseCase
import com.edgewatcher.usecase.UploadNextObservationUseCase
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import javax.inject.Inject
import kotlin.math.min
import kotlin.math.pow

/**
 * 観測の本体。
 *
 * LifecycleService なのは CameraX が LifecycleOwner を要求するため。
 * これによりカメラの寿命がサービスの寿命と一致し、画面が閉じても観測が続く。
 *
 * タイマーはウェイクロックと AlarmManager の二重化で担保する。Foreground Service は
 * 「プロセスが殺されにくい」ことを保証するだけで「動き続ける」ことは保証しない。
 */
@AndroidEntryPoint
class ObservationService : LifecycleService() {

    @Inject lateinit var capture: CaptureObservationUseCase
    @Inject lateinit var uploadNext: UploadNextObservationUseCase
    @Inject lateinit var camera: CameraXGateway
    @Inject lateinit var alarms: AlarmScheduler
    @Inject lateinit var store: CredentialStore
    @Inject lateinit var clock: Clock
    @Inject lateinit var buffer: ObservationBuffer

    private val _state = MutableStateFlow<ObservationState>(ObservationState.Observing(null))
    private val binder = LocalBinder()
    private val cycle = Mutex()

    private var wakeLock: PowerManager.WakeLock? = null
    private var consecutiveFailures = 0

    /** 直近に送信が成功した時刻。状態行の「最終送信」がこれを出す。 */
    private var lastUploadAt: java.time.Instant? = null

    inner class LocalBinder : Binder() {
        val state: StateFlow<ObservationState> get() = _state.asStateFlow()

        /** 状態行の「5分間隔」を出すために画面が読む。 */
        val interval: IntervalMinutes get() = store.readInterval()

        /** プレビュー ON。ImageCapture は bind されたままなので送信は止まらない。 */
        fun attachPreview(surfaceProvider: Preview.SurfaceProvider) =
            camera.attachPreview(surfaceProvider)

        fun detachPreview() = camera.detachPreview()
    }

    override fun onBind(intent: Intent): IBinder {
        super.onBind(intent)
        return binder
    }

    override fun onCreate() {
        super.onCreate()
        Notifications.ensureChannels(this)
        startForegroundWith(Notifications.ongoing(this, _state.value, store.readInterval()))
        acquireWakeLock()
        lifecycleScope.launch {
            camera.bindTo(this@ObservationService)
            runCycle()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        if (intent?.action == ACTION_CAPTURE) {
            lifecycleScope.launch { runCycle() }
        }
        // プロセスが OS に落とされたら作り直させる。次の撮影時刻への復帰点は
        // AlarmManager が別に持っている。
        return START_STICKY
    }

    override fun onDestroy() {
        alarms.cancel()
        camera.release()
        releaseWakeLock()
        super.onDestroy()
    }

    /** drainQueue がこの周期をどう終えたか。 */
    private enum class Drain { Done, Revoked }

    /**
     * 1周期。撮ってから、送れるだけ送り、次のアラームを置く。
     *
     * Mutex で囲うのは、アラームと手動再開が重なったときに二重に撮らないため。
     */
    private suspend fun runCycle() = cycle.withLock {
        // **資格情報が無いなら1枚も撮らない。** 撮ってから気づくと、送れない画像が
        // バッファに積まれ続け、次にペアリングした瞬間にその全部が一度に送られる。
        // ログアウト直後や失効直後にここへ来ることが実際にある。
        if (store.readCredentials() == null) {
            alarms.cancel()
            stopSelf()
            return@withLock
        }

        capture()

        if (drainQueue() == Drain.Revoked) {
            // 失効した端末に次の撮影を予約しない。予約すると、アラームがサービスを
            // 起こし直し、起きるたびに1枚撮って積むだけの往復になる。
            alarms.cancel()
            stopSelf()
            return@withLock
        }

        scheduleNext()
        publishState()
    }

    /**
     * 送れるだけ送る。
     *
     * Deferred はバックオフして同じ周期の中で再試行する。**ここで delay を使えるのは
     * PARTIAL_WAKE_LOCK を保持しているためである**（保持していなければ画面 OFF 中に
     * 発火しない）。次の撮影時刻を追い越さないよう、累計が撮影間隔に達したら諦めて
     * アラームに任せる。
     */
    private suspend fun drainQueue(): Drain {
        val intervalSeconds = store.readInterval().minutes * 60L
        var spent = 0L
        while (true) {
            when (uploadNext()) {
                is UploadNextObservationUseCase.Outcome.Sent -> {
                    consecutiveFailures = 0
                    lastUploadAt = clock.now()
                }

                UploadNextObservationUseCase.Outcome.Empty -> {
                    consecutiveFailures = 0
                    return Drain.Done
                }

                UploadNextObservationUseCase.Outcome.Deferred -> {
                    consecutiveFailures += 1
                    val wait = backoffSeconds()
                    if (spent + wait >= intervalSeconds) return Drain.Done
                    spent += wait
                    delay(wait * 1000L)
                }

                UploadNextObservationUseCase.Outcome.Revoked -> {
                    _revoked.tryEmit(Unit)
                    return Drain.Revoked
                }
            }
        }
    }

    /**
     * 次回の撮影時刻を置く。
     *
     * **撮影の間隔は送信の成否で変わらない。** リトライ中も定期撮影は止めないという
     * 要件があり、撮った画像はバッファに積まれる。送信のバックオフは撮影周期とは
     * 別に drainQueue が持つ（backoffSeconds）。
     */
    private fun scheduleNext() {
        alarms.scheduleNext(clock.now().plusSeconds(store.readInterval().minutes * 60L))
    }

    /** 連続失敗回数 n に対して min(5 * 2^(n-1), 300) 秒。 */
    private fun backoffSeconds(): Long =
        if (consecutiveFailures == 0) {
            0L
        } else {
            min(5.0 * 2.0.pow(consecutiveFailures - 1), 300.0).toLong()
        }

    /**
     * 状態の真実の源はバッファの残数。**ここで送信を試みてはならない。**
     * uploadNext() を呼ぶと drainQueue と合わせて二重に送ることになる。
     */
    private suspend fun publishState() {
        val pending = buffer.count()
        _state.value = if (pending > 0) {
            ObservationState.Offline(pending, lastUploadAt)
        } else {
            ObservationState.Observing(lastUploadAt)
        }
        startForegroundWith(Notifications.ongoing(this, _state.value, store.readInterval()))
    }

    private fun startForegroundWith(notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(
                Notifications.ONGOING_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA or
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION,
            )
        } else {
            startForeground(Notifications.ONGOING_ID, notification)
        }
    }

    /**
     * 常時給電が前提なので、稼働中は保持し続けてよい。CPU は起きているだけで
     * 実際の処理は5分に一度なので発熱も小さい。バッテリー駆動なら選べない設計。
     */
    private fun acquireWakeLock() {
        val power = getSystemService(PowerManager::class.java)
        wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, WAKE_LOCK_TAG)
            .also { it.acquire() }
    }

    private fun releaseWakeLock() {
        wakeLock?.takeIf { it.isHeld }?.release()
        wakeLock = null
    }

    companion object {
        private const val ACTION_CAPTURE = "com.edgewatcher.action.CAPTURE"
        private const val WAKE_LOCK_TAG = "edgewatcher:observation"

        private val _revoked = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

        /** deviceSecret が無効になったことを画面へ知らせる。 */
        val revoked: SharedFlow<Unit> = _revoked.asSharedFlow()

        fun start(context: Context) {
            context.startForegroundService(Intent(context, ObservationService::class.java))
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, ObservationService::class.java))
        }

        fun requestCapture(context: Context) {
            context.startForegroundService(
                Intent(context, ObservationService::class.java).setAction(ACTION_CAPTURE),
            )
        }
    }
}
