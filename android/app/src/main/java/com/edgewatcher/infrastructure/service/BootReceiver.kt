package com.edgewatcher.infrastructure.service

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import com.edgewatcher.domain.port.CredentialStore
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

/**
 * 再起動後の復帰。
 *
 * **Android 14 以降は自動復帰できない。** バックグラウンドから camera タイプの
 * Foreground Service を開始することが禁止されており、BOOT_COMPLETED の受信は
 * バックグラウンド起動に当たる。型を dataSync に変えても解決しない。カメラは
 * 「使用中のみ」の権限であり、camera 型の FGS がその「使用中」を成立させているため。
 *
 * 古い端末ほど完全自動で動くという皮肉な結果になるが、これは余剰端末の活用という
 * コンセプトとは相性が良い。
 *
 * **SDK 33 以下の経路は手元に実機が無く検証できない**（spec §11.2）。
 * 分岐をこの1箇所に閉じ込めてあるのはそのためで、壊れていた場合の影響は
 * 「古い端末で再起動後に自動復帰しない」に留まる。通知経由の手動再開は
 * どのバージョンでも動く。
 */
@AndroidEntryPoint
class BootReceiver : BroadcastReceiver() {

    @Inject lateinit var store: CredentialStore

    override fun onReceive(context: Context, intent: Intent) {
        // Hilt は生成された基底クラスの onReceive の中でフィールドを注入する。
        // これを呼ばないと store が未初期化のままになる。**最初に呼ぶこと。**
        super.onReceive(context, intent)

        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return

        // 未ペアリングの端末を起こしても、QR スキャナが立つだけで意味がない。
        if (store.readCredentials() == null) return

        Notifications.ensureChannels(context)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            context.getSystemService(NotificationManager::class.java)
                .notify(Notifications.RECOVERY_ID, Notifications.recovery(context))
        } else {
            ObservationService.start(context)
        }
    }
}
