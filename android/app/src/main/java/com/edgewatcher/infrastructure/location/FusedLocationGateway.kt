package com.edgewatcher.infrastructure.location

import android.annotation.SuppressLint
import android.content.Context
import com.edgewatcher.domain.model.Coordinates
import com.edgewatcher.domain.port.LocationGateway
import com.google.android.gms.location.LocationServices
import kotlinx.coroutines.tasks.await

/**
 * 最後の既知位置を返す。**測位を待たない。**
 *
 * getCurrentLocation は測位を待つため使わない。定点観測デバイスは動かないので、
 * 最後の既知位置で十分であり、撮影周期を測位で引き延ばす理由がない。
 * 取得できなければ null を返し、位置なしで送る。位置は必須項目ではない。
 */
class FusedLocationGateway(context: Context) : LocationGateway {

    private val client = LocationServices.getFusedLocationProviderClient(context)

    /**
     * 権限は起動前のゲートで通してある（spec §4.4）。欠けていれば
     * SecurityException になるが、その場合も null に落として撮影は続ける。
     */
    @SuppressLint("MissingPermission")
    override suspend fun lastKnown(): Coordinates? = runCatching {
        client.lastLocation.await()?.let { Coordinates(it.latitude, it.longitude) }
    }.getOrNull()
}
