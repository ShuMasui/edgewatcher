package com.edgewatcher.infrastructure.db

import androidx.room.InvalidationTracker
import androidx.room.RoomOpenDelegate
import androidx.room.migration.AutoMigrationSpec
import androidx.room.migration.Migration
import androidx.room.util.TableInfo
import androidx.room.util.TableInfo.Companion.read
import androidx.room.util.dropFtsSyncTriggers
import androidx.sqlite.SQLiteConnection
import androidx.sqlite.execSQL
import javax.`annotation`.processing.Generated
import kotlin.Lazy
import kotlin.String
import kotlin.Suppress
import kotlin.collections.List
import kotlin.collections.Map
import kotlin.collections.MutableList
import kotlin.collections.MutableMap
import kotlin.collections.MutableSet
import kotlin.collections.Set
import kotlin.collections.mutableListOf
import kotlin.collections.mutableMapOf
import kotlin.collections.mutableSetOf
import kotlin.reflect.KClass

@Generated(value = ["androidx.room.RoomProcessor"])
@Suppress(names = ["UNCHECKED_CAST", "DEPRECATION", "REDUNDANT_PROJECTION", "REMOVAL"])
public class ObservationDatabase_Impl : ObservationDatabase() {
  private val _observationDao: Lazy<ObservationDao> = lazy {
    ObservationDao_Impl(this)
  }

  protected override fun createOpenDelegate(): RoomOpenDelegate {
    val _openDelegate: RoomOpenDelegate = object : RoomOpenDelegate(1,
        "7131496e6e942d813d34e0f2df81bde8", "1340d00f6a758d399ff26c755411cca8") {
      public override fun createAllTables(connection: SQLiteConnection) {
        connection.execSQL("CREATE TABLE IF NOT EXISTS `observations` (`observationId` TEXT NOT NULL, `capturedAtMillis` INTEGER NOT NULL, `lat` REAL, `lng` REAL, `totalBytes` INTEGER NOT NULL, PRIMARY KEY(`observationId`))")
        connection.execSQL("CREATE TABLE IF NOT EXISTS room_master_table (id INTEGER PRIMARY KEY,identity_hash TEXT)")
        connection.execSQL("INSERT OR REPLACE INTO room_master_table (id,identity_hash) VALUES(42, '7131496e6e942d813d34e0f2df81bde8')")
      }

      public override fun dropAllTables(connection: SQLiteConnection) {
        connection.execSQL("DROP TABLE IF EXISTS `observations`")
      }

      public override fun onCreate(connection: SQLiteConnection) {
      }

      public override fun onOpen(connection: SQLiteConnection) {
        internalInitInvalidationTracker(connection)
      }

      public override fun onPreMigrate(connection: SQLiteConnection) {
        dropFtsSyncTriggers(connection)
      }

      public override fun onPostMigrate(connection: SQLiteConnection) {
      }

      public override fun onValidateSchema(connection: SQLiteConnection):
          RoomOpenDelegate.ValidationResult {
        val _columnsObservations: MutableMap<String, TableInfo.Column> = mutableMapOf()
        _columnsObservations.put("observationId", TableInfo.Column("observationId", "TEXT", true, 1,
            null, TableInfo.CREATED_FROM_ENTITY))
        _columnsObservations.put("capturedAtMillis", TableInfo.Column("capturedAtMillis", "INTEGER",
            true, 0, null, TableInfo.CREATED_FROM_ENTITY))
        _columnsObservations.put("lat", TableInfo.Column("lat", "REAL", false, 0, null,
            TableInfo.CREATED_FROM_ENTITY))
        _columnsObservations.put("lng", TableInfo.Column("lng", "REAL", false, 0, null,
            TableInfo.CREATED_FROM_ENTITY))
        _columnsObservations.put("totalBytes", TableInfo.Column("totalBytes", "INTEGER", true, 0,
            null, TableInfo.CREATED_FROM_ENTITY))
        val _foreignKeysObservations: MutableSet<TableInfo.ForeignKey> = mutableSetOf()
        val _indicesObservations: MutableSet<TableInfo.Index> = mutableSetOf()
        val _infoObservations: TableInfo = TableInfo("observations", _columnsObservations,
            _foreignKeysObservations, _indicesObservations)
        val _existingObservations: TableInfo = read(connection, "observations")
        if (!_infoObservations.equals(_existingObservations)) {
          return RoomOpenDelegate.ValidationResult(false, """
              |observations(com.edgewatcher.infrastructure.db.ObservationEntity).
              | Expected:
              |""".trimMargin() + _infoObservations + """
              |
              | Found:
              |""".trimMargin() + _existingObservations)
        }
        return RoomOpenDelegate.ValidationResult(true, null)
      }
    }
    return _openDelegate
  }

  protected override fun createInvalidationTracker(): InvalidationTracker {
    val _shadowTablesMap: MutableMap<String, String> = mutableMapOf()
    val _viewTables: MutableMap<String, Set<String>> = mutableMapOf()
    return InvalidationTracker(this, _shadowTablesMap, _viewTables, "observations")
  }

  public override fun clearAllTables() {
    super.performClear(false, "observations")
  }

  protected override fun getRequiredTypeConverterClasses(): Map<KClass<*>, List<KClass<*>>> {
    val _typeConvertersMap: MutableMap<KClass<*>, List<KClass<*>>> = mutableMapOf()
    _typeConvertersMap.put(ObservationDao::class, ObservationDao_Impl.getRequiredConverters())
    return _typeConvertersMap
  }

  public override fun getRequiredAutoMigrationSpecClasses(): Set<KClass<out AutoMigrationSpec>> {
    val _autoMigrationSpecsSet: MutableSet<KClass<out AutoMigrationSpec>> = mutableSetOf()
    return _autoMigrationSpecsSet
  }

  public override
      fun createAutoMigrations(autoMigrationSpecs: Map<KClass<out AutoMigrationSpec>, AutoMigrationSpec>):
      List<Migration> {
    val _autoMigrations: MutableList<Migration> = mutableListOf()
    return _autoMigrations
  }

  public override fun observations(): ObservationDao = _observationDao.value
}
