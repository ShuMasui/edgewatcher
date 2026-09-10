package com.edgewatcher.presentation.pairing

import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.usecase.PairDeviceUseCase
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withTimeoutOrNull
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/**
 * ペアリング成立は**一度きりの出来事**であり、保持される状態ではない。
 *
 * この区別が壊れると、ログアウト後に未ペアリング画面へ戻った瞬間、
 * 前回のペアリング成立が再生されて稼働画面へ跳ね返される。ViewModel は
 * ナビゲーションを持たない構成では Activity に紐づいて生き残るため、
 * 画面を離れても状態が消えないことがこの不具合の前提になる。
 */
@OptIn(ExperimentalCoroutinesApi::class)
class PairingViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val deviceInfo = DeviceInfo(model = "TEST", osVersion = "16", appVersion = "1.0.0")

    @Before
    fun setUp() = Dispatchers.setMain(dispatcher)

    @After
    fun tearDown() = Dispatchers.resetMain()

    @Test
    fun `ペアリング成立は一度だけ流れ、画面へ戻っても再生されない`() = runTest {
        val viewModel = PairingViewModel(pairDevice(), deviceInfo)

        viewModel.onQrScanned("ew1:CODE")
        advanceUntilIdle()

        assertEquals(Unit, viewModel.paired.first())

        // ログアウトして未ペアリング画面へ戻った状況。同じ ViewModel が生き残っている。
        // ここで再び流れてしまうと、稼働画面へ跳ね返されて無限に往復する。
        val replayed = withTimeoutOrNull(1_000) { viewModel.paired.first() }
        assertNull("画面へ戻ったときにペアリング成立が再生されてはならない", replayed)
    }

    @Test
    fun `ログアウト後に同じ ViewModel でもう一度ペアリングできる`() = runTest {
        val viewModel = PairingViewModel(pairDevice(), deviceInfo)

        viewModel.onQrScanned("ew1:CODE")
        advanceUntilIdle()
        assertEquals(Unit, viewModel.paired.first())

        viewModel.onQrScanned("ew1:ANOTHER")
        advanceUntilIdle()
        assertEquals(Unit, viewModel.paired.first())
    }

    private fun pairDevice() = PairDeviceUseCase(AlwaysPairsApi(), InMemoryCredentialStore())

    private class AlwaysPairsApi : ObservationApi {
        override suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo) =
            ApiResult.Success(DeviceCredentials(deviceId = "d1", deviceSecret = "s1"))

        override suspend fun issueSession(credentials: DeviceCredentials) =
            ApiResult.Success(Session(token = "t1", expiresAtEpochSeconds = Long.MAX_VALUE))

        override suspend fun upload(
            sessionToken: String,
            observation: PendingObservation,
            image: ByteArray,
            thumbnail: ByteArray,
        ) = ApiResult.Success(IntervalMinutes.FIVE)

        override suspend fun logout(sessionToken: String) = ApiResult.Success(Unit)
    }

    private class InMemoryCredentialStore : CredentialStore {
        private var credentials: DeviceCredentials? = null
        private var session: Session? = null
        private var interval = IntervalMinutes.DEFAULT

        override fun readCredentials() = credentials
        override fun writeCredentials(credentials: DeviceCredentials) {
            this.credentials = credentials
        }

        override fun readSession() = session
        override fun writeSession(session: Session) {
            this.session = session
        }

        override fun readInterval() = interval
        override fun writeInterval(interval: IntervalMinutes) {
            this.interval = interval
        }

        override fun clear() {
            credentials = null
            session = null
            interval = IntervalMinutes.DEFAULT
        }
    }
}
