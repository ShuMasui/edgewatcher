package com.edgewatcher.infrastructure.service

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import com.edgewatcher.domain.port.AlarmScheduler
import java.time.Instant

/**
 * 次回撮影時刻に必ず起こす。
 *
 * Foreground Service は CPU のサスペンドを妨げない。画面を消すと端末は
 * サスペンドに入り、プロセス内タイマーは発火しないか大幅に遅延する。
 * ウェイクロックがメーカー独自の省電力機構に剥がされた場合や、プロセスが
 * OS に落とされて START_STICKY で作り直された場合の**復帰点**にもなる。
 *
 * **繰り返しアラームは使わない。** 撮影のたびに次回分を設定することで、
 * nextConfig による間隔変更が次のサイクルから自然に反映される。
 */
class AndroidAlarmScheduler(private val context: Context) : AlarmScheduler {

    private val manager = context.getSystemService(AlarmManager::class.java)

    override fun scheduleNext(at: Instant) {
        val trigger = at.toEpochMilli()
        // Android 12 以降、setExactAndAllowWhileIdle には SCHEDULE_EXACT_ALARM が要る。
        // 案内は済んでいるが、拒否された端末でも観測は続けたいので不正確版へ落とす。
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && !manager.canScheduleExactAlarms()) {
            manager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, trigger, pendingIntent())
            return
        }
        manager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, trigger, pendingIntent())
    }

    override fun cancel() {
        manager.cancel(pendingIntent())
    }

    private fun pendingIntent(): PendingIntent = PendingIntent.getBroadcast(
        context,
        REQUEST_CODE,
        Intent(context, AlarmReceiver::class.java),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )

    private companion object {
        const val REQUEST_CODE = 1001
    }
}
