package com.edgewatcher.infrastructure.db

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * 送信待ちの観測1件。**画像本体は持たない。**
 *
 * BLOB を SQLite に入れると DB ファイルが肥大し、削除しても領域が戻りにくい。
 * バッファは常に書いては消すことを繰り返すため、この性質が効いてくる。
 */
@Entity(tableName = "observations")
data class ObservationEntity(
    @PrimaryKey val observationId: String,
    /** epoch ミリ秒。並び替えのキー。 */
    val capturedAtMillis: Long,
    val lat: Double?,
    val lng: Double?,
    /** 本画像 + サムネイルの合計。500MiB 判定に使う。 */
    val totalBytes: Long,
)
