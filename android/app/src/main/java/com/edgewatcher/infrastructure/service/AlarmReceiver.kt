package com.edgewatcher.infrastructure.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/** アラームを Service の「今すぐ撮れ」に変換するだけ。判断は持たない。 */
class AlarmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        ObservationService.requestCapture(context)
    }
}
