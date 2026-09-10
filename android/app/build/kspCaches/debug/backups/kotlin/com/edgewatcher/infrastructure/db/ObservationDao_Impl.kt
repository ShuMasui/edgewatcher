package com.edgewatcher.infrastructure.db

import androidx.room.EntityInsertAdapter
import androidx.room.RoomDatabase
import androidx.room.util.getColumnIndexOrThrow
import androidx.room.util.performSuspending
import androidx.sqlite.SQLiteStatement
import javax.`annotation`.processing.Generated
import kotlin.Double
import kotlin.Int
import kotlin.Long
import kotlin.String
import kotlin.Suppress
import kotlin.Unit
import kotlin.collections.List
import kotlin.collections.MutableList
import kotlin.collections.mutableListOf
import kotlin.reflect.KClass

@Generated(value = ["androidx.room.RoomProcessor"])
@Suppress(names = ["UNCHECKED_CAST", "DEPRECATION", "REDUNDANT_PROJECTION", "REMOVAL"])
public class ObservationDao_Impl(
  __db: RoomDatabase,
) : ObservationDao {
  private val __db: RoomDatabase

  private val __insertAdapterOfObservationEntity: EntityInsertAdapter<ObservationEntity>
  init {
    this.__db = __db
    this.__insertAdapterOfObservationEntity = object : EntityInsertAdapter<ObservationEntity>() {
      protected override fun createQuery(): String =
          "INSERT OR REPLACE INTO `observations` (`observationId`,`capturedAtMillis`,`lat`,`lng`,`totalBytes`) VALUES (?,?,?,?,?)"

      protected override fun bind(statement: SQLiteStatement, entity: ObservationEntity) {
        statement.bindText(1, entity.observationId)
        statement.bindLong(2, entity.capturedAtMillis)
        val _tmpLat: Double? = entity.lat
        if (_tmpLat == null) {
          statement.bindNull(3)
        } else {
          statement.bindDouble(3, _tmpLat)
        }
        val _tmpLng: Double? = entity.lng
        if (_tmpLng == null) {
          statement.bindNull(4)
        } else {
          statement.bindDouble(4, _tmpLng)
        }
        statement.bindLong(5, entity.totalBytes)
      }
    }
  }

  public override suspend fun insert(row: ObservationEntity): Unit = performSuspending(__db, false,
      true) { _connection ->
    __insertAdapterOfObservationEntity.insert(_connection, row)
  }

  public override suspend fun count(): Int {
    val _sql: String = "SELECT COUNT(*) FROM observations"
    return performSuspending(__db, true, false) { _connection ->
      val _stmt: SQLiteStatement = _connection.prepare(_sql)
      try {
        val _result: Int
        if (_stmt.step()) {
          val _tmp: Int
          _tmp = _stmt.getLong(0).toInt()
          _result = _tmp
        } else {
          _result = 0
        }
        _result
      } finally {
        _stmt.close()
      }
    }
  }

  public override suspend fun totalBytes(): Long {
    val _sql: String = "SELECT COALESCE(SUM(totalBytes), 0) FROM observations"
    return performSuspending(__db, true, false) { _connection ->
      val _stmt: SQLiteStatement = _connection.prepare(_sql)
      try {
        val _result: Long
        if (_stmt.step()) {
          val _tmp: Long
          _tmp = _stmt.getLong(0)
          _result = _tmp
        } else {
          _result = 0L
        }
        _result
      } finally {
        _stmt.close()
      }
    }
  }

  public override suspend fun allOldestFirst(): List<ObservationEntity> {
    val _sql: String = "SELECT * FROM observations ORDER BY capturedAtMillis ASC"
    return performSuspending(__db, true, false) { _connection ->
      val _stmt: SQLiteStatement = _connection.prepare(_sql)
      try {
        val _columnIndexOfObservationId: Int = getColumnIndexOrThrow(_stmt, "observationId")
        val _columnIndexOfCapturedAtMillis: Int = getColumnIndexOrThrow(_stmt, "capturedAtMillis")
        val _columnIndexOfLat: Int = getColumnIndexOrThrow(_stmt, "lat")
        val _columnIndexOfLng: Int = getColumnIndexOrThrow(_stmt, "lng")
        val _columnIndexOfTotalBytes: Int = getColumnIndexOrThrow(_stmt, "totalBytes")
        val _result: MutableList<ObservationEntity> = mutableListOf()
        while (_stmt.step()) {
          val _item: ObservationEntity
          val _tmpObservationId: String
          _tmpObservationId = _stmt.getText(_columnIndexOfObservationId)
          val _tmpCapturedAtMillis: Long
          _tmpCapturedAtMillis = _stmt.getLong(_columnIndexOfCapturedAtMillis)
          val _tmpLat: Double?
          if (_stmt.isNull(_columnIndexOfLat)) {
            _tmpLat = null
          } else {
            _tmpLat = _stmt.getDouble(_columnIndexOfLat)
          }
          val _tmpLng: Double?
          if (_stmt.isNull(_columnIndexOfLng)) {
            _tmpLng = null
          } else {
            _tmpLng = _stmt.getDouble(_columnIndexOfLng)
          }
          val _tmpTotalBytes: Long
          _tmpTotalBytes = _stmt.getLong(_columnIndexOfTotalBytes)
          _item =
              ObservationEntity(_tmpObservationId,_tmpCapturedAtMillis,_tmpLat,_tmpLng,_tmpTotalBytes)
          _result.add(_item)
        }
        _result
      } finally {
        _stmt.close()
      }
    }
  }

  public override suspend fun delete(observationId: String) {
    val _sql: String = "DELETE FROM observations WHERE observationId = ?"
    return performSuspending(__db, false, true) { _connection ->
      val _stmt: SQLiteStatement = _connection.prepare(_sql)
      try {
        var _argIndex: Int = 1
        _stmt.bindText(_argIndex, observationId)
        _stmt.step()
      } finally {
        _stmt.close()
      }
    }
  }

  public override suspend fun deleteAll() {
    val _sql: String = "DELETE FROM observations"
    return performSuspending(__db, false, true) { _connection ->
      val _stmt: SQLiteStatement = _connection.prepare(_sql)
      try {
        _stmt.step()
      } finally {
        _stmt.close()
      }
    }
  }

  public companion object {
    public fun getRequiredConverters(): List<KClass<*>> = emptyList()
  }
}
