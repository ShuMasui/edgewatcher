package com.edgewatcher.infrastructure.store

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.CredentialStore

/**
 * deviceSecret と sessionToken の保管。
 *
 * deviceSecret はシステム全体でこれが平文で存在する唯一の場所であり、
 * サーバは SHA-256 しか持たない。失うと Web からの QR 再発行しか復帰手段がない。
 *
 * **ここに入れた値をログへ出さないこと。**
 */
class EncryptedCredentialStore(context: Context) : CredentialStore {

    private val prefs: SharedPreferences = EncryptedSharedPreferences.create(
        context,
        FILE_NAME,
        MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    override fun readCredentials(): DeviceCredentials? {
        val id = prefs.getString(KEY_DEVICE_ID, null) ?: return null
        val secret = prefs.getString(KEY_DEVICE_SECRET, null) ?: return null
        return DeviceCredentials(id, secret)
    }

    override fun writeCredentials(credentials: DeviceCredentials) {
        prefs.edit()
            .putString(KEY_DEVICE_ID, credentials.deviceId)
            .putString(KEY_DEVICE_SECRET, credentials.deviceSecret)
            .apply()
    }

    override fun readSession(): Session? {
        val token = prefs.getString(KEY_SESSION_TOKEN, null) ?: return null
        val expiresAt = prefs.getLong(KEY_SESSION_EXPIRES_AT, 0L)
        if (expiresAt == 0L) return null
        return Session(token, expiresAt)
    }

    override fun writeSession(session: Session) {
        prefs.edit()
            .putString(KEY_SESSION_TOKEN, session.token)
            .putLong(KEY_SESSION_EXPIRES_AT, session.expiresAtEpochSeconds)
            .apply()
    }

    override fun readInterval(): IntervalMinutes {
        val minutes = prefs.getInt(KEY_INTERVAL_MINUTES, IntervalMinutes.DEFAULT.minutes)
        return IntervalMinutes.fromMinutes(minutes) ?: IntervalMinutes.DEFAULT
    }

    override fun writeInterval(interval: IntervalMinutes) {
        prefs.edit().putInt(KEY_INTERVAL_MINUTES, interval.minutes).apply()
    }

    override fun clear() {
        prefs.edit().clear().apply()
    }

    private companion object {
        const val FILE_NAME = "edgewatcher_credentials"
        const val KEY_DEVICE_ID = "device_id"
        const val KEY_DEVICE_SECRET = "device_secret"
        const val KEY_SESSION_TOKEN = "session_token"
        const val KEY_SESSION_EXPIRES_AT = "session_expires_at"
        const val KEY_INTERVAL_MINUTES = "interval_minutes"
    }
}
