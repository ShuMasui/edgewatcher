package com.edgewatcher.infrastructure.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.edgewatcher.domain.StatusLine
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.presentation.MainActivity

/**
 * 常駐通知と復帰要求通知。
 *
 * 常駐通知は画面と**同じ文字列**を出す。端末を持ち上げなくても状態が分かるように
 * するため。文言の生成は StatusLine に一本化してあり、ここでは組み立てない。
 *
 * **端末名は出さない。** 端末は自分の名前を知らない。
 */
object Notifications {

    const val CHANNEL_RUNNING = "edgewatcher_running"
    const val CHANNEL_RECOVERY = "edgewatcher_recovery"
    const val ONGOING_ID = 1
    const val RECOVERY_ID = 2

    fun ensureChannels(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_RUNNING,
                "観測の稼働状態",
                NotificationManager.IMPORTANCE_LOW,
            ).apply { setShowBadge(false) },
        )
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_RECOVERY,
                "観測の再開要求",
                NotificationManager.IMPORTANCE_HIGH,
            ),
        )
    }

    fun ongoing(context: Context, state: ObservationState): Notification =
        NotificationCompat.Builder(context, CHANNEL_RUNNING)
            .setContentTitle("EdgeWatcher")
            .setContentText(StatusLine.render(state))
            .setSmallIcon(android.R.drawable.ic_menu_camera)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(openApp(context))
            .build()

    /**
     * Android 14 以降は BOOT_COMPLETED から camera 型の FGS を開始できないため、
     * 通知を出してタップで前面に来てもらうしかない。
     */
    fun recovery(context: Context): Notification =
        NotificationCompat.Builder(context, CHANNEL_RECOVERY)
            .setContentTitle("EdgeWatcher が停止しています")
            .setContentText("タップして観測を再開してください。")
            .setSmallIcon(android.R.drawable.stat_notify_error)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setAutoCancel(true)
            .setContentIntent(openApp(context))
            .build()

    private fun openApp(context: Context): PendingIntent = PendingIntent.getActivity(
        context,
        0,
        Intent(context, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )
}
