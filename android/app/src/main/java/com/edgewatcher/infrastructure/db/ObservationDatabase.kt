package com.edgewatcher.infrastructure.db

import androidx.room.Database
import androidx.room.RoomDatabase

/** スキーマは v1 のみ。変更時は fallbackToDestructiveMigration で作り直す。 */
@Database(entities = [ObservationEntity::class], version = 1, exportSchema = false)
abstract class ObservationDatabase : RoomDatabase() {
    abstract fun observations(): ObservationDao
}
