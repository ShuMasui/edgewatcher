package com.edgewatcher.infrastructure.db

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query

@Dao
interface ObservationDao {

    /** 同じ observationId の再投入は上書き。冪等性は端末側でも保つ。 */
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insert(row: ObservationEntity)

    @Query("DELETE FROM observations WHERE observationId = :observationId")
    suspend fun delete(observationId: String)

    @Query("DELETE FROM observations")
    suspend fun deleteAll()

    @Query("SELECT COUNT(*) FROM observations")
    suspend fun count(): Int

    @Query("SELECT COALESCE(SUM(totalBytes), 0) FROM observations")
    suspend fun totalBytes(): Long

    @Query("SELECT * FROM observations ORDER BY capturedAtMillis ASC")
    suspend fun allOldestFirst(): List<ObservationEntity>
}
