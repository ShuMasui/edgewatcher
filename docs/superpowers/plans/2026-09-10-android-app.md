# EdgeWatcher Android アプリ 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 余剰 Android 端末を定点観測デバイスとして動かすアプリを完成させる。QR でペアリングし、以後カメラ画像と位置情報を定期送信し続ける。

**Architecture:** 緩い Clean Architecture。`domain` と `usecase` は `android.*` を一切 import しない純 Kotlin で、要件の判断をすべてここに置く。`infrastructure` がポートの実物を実装し、`presentation` は状態を描くだけ。カメラの所有権は Foreground Service が一本で持ち、画面は `bindService` して Surface を渡すだけ。タイマーはウェイクロックと `AlarmManager` の二重化で担保する。

**Tech Stack:** Kotlin / Gradle（Kotlin DSL・バージョンカタログ）/ Jetpack Compose / Hilt (KSP) / CameraX / ML Kit barcode-scanning (bundled) / Retrofit + OkHttp + kotlinx.serialization / Room / EncryptedSharedPreferences / Coroutines / JUnit 4

**Spec:** `docs/superpowers/specs/2026-09-10-android-app-design.md`

---

## Global Constraints

spec の全域に効く要求。**すべてのタスクの要件はこの節を暗黙に含む。**

- **作業ディレクトリは `android/`。Gradle コマンドはすべて `android/` から実行する**
- `minSdk` = **26**、`compileSdk` / `targetSdk` = **36**。プレビュー SDK は使わない
- `applicationId` = **`com.edgewatcher`**、`namespace` = **`com.edgewatcher`**
- 画面の向きは**縦固定**
- **`domain` と `usecase` は `android.*` および Android ライブラリを import しない。**
  この規約を破らないと書けないロジックが現れたら、それは `infrastructure` に属する
- **Hilt の `@Module` は `com.edgewatcher.di` にのみ置く。** 他のパッケージに Module を作らない
- 本画像は**長辺 1600px / 品質 80**、サムネイルは**長辺 480px / 品質 75**。いずれも JPEG
- 本画像 + サムネイルの合計は **4,500,000 バイト**以下。超過時は §5.3 の段階で落とす
- 送信間隔の初期値は **5分**。`nextConfig.intervalMinutes` は 5 / 10 / 15 のいずれか
- バッファ上限は **288件** または **524,288,000 バイト（500MiB）**
- ペアリングの 404 リトライは **初回1回 + リトライ3回 = 最大4リクエスト、間隔1秒**
- 送信のバックオフは連続失敗回数 n に対して **`min(5 * 2^(n-1), 300)` 秒**
- `Authorization` ヘッダは **`Bearer` を付けない素のトークン**
- **資格情報を消してよいのは `POST /device/token` が 401 を返したときだけ。**
  5xx・タイムアウト・ネットワーク断で消してはならない
- **`ACCESS_BACKGROUND_LOCATION` は要求しない**
- **端末名は画面にも通知にも出さない**（`POST /device/pair` の応答に含まれない）
- **資格情報（`deviceSecret` / `sessionToken`）をログに出力しない**
- 通信は HTTPS のみ

### テストの方針（この計画は全面 TDD ではない）

spec §10 により、**書くテストは4本だけ**である。Task 2〜5 がそれに当たり、
この4タスクのみ Red → Green のサイクルを踏む。

| # | 対象 | タスク |
| --- | --- | --- |
| 1 | ULID の採番時刻が `capturedAt` と一致する | Task 2 |
| 2 | HTTP status → `ApiFailure` の分類 | Task 3 |
| 3 | バッファの eviction（288件 / 500MiB の先着） | Task 4 |
| 4 | 送信順序（最新1枚 → 以後古い順） | Task 5 |

**それ以外のタスクにテストを書かない。** UI テスト・計装テスト・Robolectric・
Room マイグレーションテスト・MockWebServer は作らない（spec §10.1）。
これらのタスクの完了条件はコンパイルが通ることであり、動作は Task 22 の実機受け入れでまとめて確認する。

テストを増やしたくなったら、それは spec の変更であり、勝手に足さない。

---

## ファイル構成

### `android/`（ビルド定義。TDD の対象外）

| ファイル | 責務 |
| --- | --- |
| `settings.gradle.kts` | ルート。`:app` だけを含む |
| `gradle/libs.versions.toml` | バージョンカタログ。依存のバージョンはここにだけ書く |
| `gradle.properties` | `API_BASE_URL_DEV` / `API_BASE_URL_PROD` |
| `app/build.gradle.kts` | Android / Compose / Hilt / Room / `buildConfigField` |
| `app/src/main/AndroidManifest.xml` | 権限・FGS の型・Receiver・縦固定 |

### `domain/`（純 Kotlin。Android SDK を触らない）

| ファイル | 責務 |
| --- | --- |
| `domain/model/Models.kt` | `DeviceCredentials` `Session` `DeviceInfo` `Coordinates` `PendingObservation` `IntervalMinutes` `ObservationState` |
| `domain/error/ApiFailure.kt` | 失敗の封じた型と `ApiResult` |
| `domain/error/ApiFailures.kt` | **HTTP status → `ApiFailure` の分類（テスト2）** |
| `domain/port/Ports.kt` | 9つのポートのインターフェース |
| `domain/UlidGenerator.kt` | **採番時刻を引数で受ける ULID（テスト1）** |
| `domain/BufferPolicy.kt` | **eviction の判断（テスト3）** |
| `domain/UploadOrder.kt` | **送信順序（テスト4）** |
| `domain/ImagePolicy.kt` | 符号化の段階とサイズガードの判断 |
| `domain/StatusLine.kt` | 状態行の文言。画面と通知が同じ文字列を使う |

### `usecase/`（純 Kotlin。`domain/port` にだけ依存）

| ファイル | 責務 |
| --- | --- |
| `usecase/PairDeviceUseCase.kt` | QR → `deviceSecret` → `sessionToken` |
| `usecase/RefreshSessionUseCase.kt` | セッション再取得と、資格情報を消す唯一の判断 |
| `usecase/LogoutDeviceUseCase.kt` | 端末自身の失効 |
| `usecase/CaptureObservationUseCase.kt` | 撮影1周期 |
| `usecase/UploadNextObservationUseCase.kt` | 1件送る |

### `infrastructure/`

| ファイル | 責務 |
| --- | --- |
| `infrastructure/api/EdgeWatcherService.kt` | Retrofit インターフェース（端末群4本） |
| `infrastructure/api/Dtos.kt` | リクエスト / レスポンスの DTO |
| `infrastructure/api/RetrofitObservationApi.kt` | `ObservationApi` の実装。status → `ApiFailure` の変換 |
| `infrastructure/store/EncryptedCredentialStore.kt` | `CredentialStore` の実装 |
| `infrastructure/db/ObservationEntity.kt` | Room の行 |
| `infrastructure/db/ObservationDao.kt` | DAO |
| `infrastructure/db/ObservationDatabase.kt` | `RoomDatabase`。スキーマ v1 のみ |
| `infrastructure/db/RoomObservationBuffer.kt` | `ObservationBuffer` の実装。行とファイルを対で扱う |
| `infrastructure/camera/CameraXGateway.kt` | `CameraGateway` の実装。所有権と Surface の付け外し |
| `infrastructure/camera/AndroidJpegEncoder.kt` | `JpegEncoder` の実装。言われた寸法で符号化するだけ |
| `infrastructure/location/FusedLocationGateway.kt` | `LocationGateway` の実装 |
| `infrastructure/service/AndroidAlarmScheduler.kt` | `AlarmScheduler` の実装 |
| `infrastructure/service/Notifications.kt` | 常駐通知と復帰要求通知 |
| `infrastructure/service/ObservationService.kt` | `LifecycleService`。撮影周期と送信ループの本体 |
| `infrastructure/service/BootReceiver.kt` | SDK による分岐 |
| `infrastructure/SystemClock.kt` | `Clock` の実装 |

### `di/`

| ファイル | 責務 |
| --- | --- |
| `di/AppModule.kt` | すべての Hilt Module。`@Binds` でポートに実装を結ぶ |

### `presentation/`

| ファイル | 責務 |
| --- | --- |
| `presentation/MainActivity.kt` | 単一 Activity。資格情報の有無で画面を選ぶ |
| `presentation/permission/PermissionScreen.kt` | 権限ゲート |
| `presentation/pairing/PairingViewModel.kt` | ペアリングの状態 |
| `presentation/pairing/PairingScreen.kt` | QR スキャナ |
| `presentation/running/RunningViewModel.kt` | Service への bind と状態の中継 |
| `presentation/running/RunningScreen.kt` | 状態1行 + ボタン2つ + プレビュー |

---

## Task 1: Gradle の骨組みと Hilt の土台

**Files:**
- Create: `android/settings.gradle.kts`
- Create: `android/gradle.properties`
- Create: `android/gradle/libs.versions.toml`
- Create: `android/build.gradle.kts`
- Create: `android/app/build.gradle.kts`
- Create: `android/app/src/main/AndroidManifest.xml`
- Create: `android/app/src/main/java/com/edgewatcher/EdgeWatcherApp.kt`
- Create: `android/.gitignore`

**Interfaces:**
- Consumes: なし
- Produces: `com.edgewatcher.EdgeWatcherApp`（`@HiltAndroidApp`）、
  `BuildConfig.API_BASE_URL`（`String`）、`BuildConfig.APP_VERSION`（`String`）

**Note:** ビルド定義とマニフェストは Global Constraints のとおり TDD の対象外。
完了条件はビルドが通ることのみ。

- [ ] **Step 1: Gradle ラッパーを生成する**

`android/` を作り、そこで実行する。ラッパーのバージョンは 8.14.3 とする。

```bash
mkdir -p android && cd android
gradle wrapper --gradle-version 8.14.3
```

`gradle` コマンドが無い場合は、既存の Gradle プロジェクトから
`gradlew` / `gradlew.bat` / `gradle/wrapper/` をコピーしてもよい。

- [ ] **Step 2: `android/settings.gradle.kts` を書く**

```kotlin
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "edgewatcher"
include(":app")
```

- [ ] **Step 3: `android/gradle.properties` を書く**

API Gateway の URL は秘密ではないため、ここに置いてよい（spec §13）。
`<...>` の実値は `infra/envs/dev` の出力から取る。未確定なら空文字のままでよく、
Task 22 の実機受け入れの前までに埋めればよい。

```properties
org.gradle.jvmargs=-Xmx4g -Dfile.encoding=UTF-8
org.gradle.parallel=true
org.gradle.caching=true
android.useAndroidX=true
kotlin.code.style=official

API_BASE_URL_DEV=https://REPLACE_WITH_DEV_API_GATEWAY_URL
API_BASE_URL_PROD=https://REPLACE_WITH_PROD_API_GATEWAY_URL
```

- [ ] **Step 4: `android/gradle/libs.versions.toml` を書く**

```toml
[versions]
agp = "8.13.0"
kotlin = "2.2.0"
ksp = "2.2.0-2.0.2"
hilt = "2.57"
compose-bom = "2025.08.00"
lifecycle = "2.9.2"
camerax = "1.4.2"
room = "2.7.2"
retrofit = "3.0.0"
okhttp = "4.12.0"
serialization = "1.9.0"
coroutines = "1.10.2"
mlkit-barcode = "17.3.0"
security-crypto = "1.1.0-alpha07"
play-location = "21.3.0"
activity-compose = "1.10.1"
hilt-navigation-compose = "1.2.0"
junit = "4.13.2"

[libraries]
androidx-core-ktx = { module = "androidx.core:core-ktx", version = "1.16.0" }
androidx-activity-compose = { module = "androidx.activity:activity-compose", version.ref = "activity-compose" }
androidx-lifecycle-runtime-ktx = { module = "androidx.lifecycle:lifecycle-runtime-ktx", version.ref = "lifecycle" }
androidx-lifecycle-service = { module = "androidx.lifecycle:lifecycle-service", version.ref = "lifecycle" }
androidx-lifecycle-viewmodel-compose = { module = "androidx.lifecycle:lifecycle-viewmodel-compose", version.ref = "lifecycle" }
compose-bom = { module = "androidx.compose:compose-bom", version.ref = "compose-bom" }
compose-ui = { module = "androidx.compose.ui:ui" }
compose-ui-graphics = { module = "androidx.compose.ui:ui-graphics" }
compose-ui-tooling-preview = { module = "androidx.compose.ui:ui-tooling-preview" }
compose-material3 = { module = "androidx.compose.material3:material3" }
hilt-android = { module = "com.google.dagger:hilt-android", version.ref = "hilt" }
hilt-compiler = { module = "com.google.dagger:hilt-android-compiler", version.ref = "hilt" }
hilt-navigation-compose = { module = "androidx.hilt:hilt-navigation-compose", version.ref = "hilt-navigation-compose" }
camerax-core = { module = "androidx.camera:camera-core", version.ref = "camerax" }
camerax-camera2 = { module = "androidx.camera:camera-camera2", version.ref = "camerax" }
camerax-lifecycle = { module = "androidx.camera:camera-lifecycle", version.ref = "camerax" }
camerax-view = { module = "androidx.camera:camera-view", version.ref = "camerax" }
mlkit-barcode = { module = "com.google.mlkit:barcode-scanning", version.ref = "mlkit-barcode" }
room-runtime = { module = "androidx.room:room-runtime", version.ref = "room" }
room-ktx = { module = "androidx.room:room-ktx", version.ref = "room" }
room-compiler = { module = "androidx.room:room-compiler", version.ref = "room" }
retrofit = { module = "com.squareup.retrofit2:retrofit", version.ref = "retrofit" }
retrofit-serialization = { module = "com.squareup.retrofit2:converter-kotlinx-serialization", version.ref = "retrofit" }
okhttp = { module = "com.squareup.okhttp3:okhttp", version.ref = "okhttp" }
kotlinx-serialization-json = { module = "org.jetbrains.kotlinx:kotlinx-serialization-json", version.ref = "serialization" }
kotlinx-coroutines-android = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-android", version.ref = "coroutines" }
kotlinx-coroutines-play-services = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-play-services", version.ref = "coroutines" }
androidx-security-crypto = { module = "androidx.security:security-crypto", version.ref = "security-crypto" }
play-services-location = { module = "com.google.android.gms:play-services-location", version.ref = "play-location" }
junit = { module = "junit:junit", version.ref = "junit" }
kotlinx-coroutines-test = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-test", version.ref = "coroutines" }

[plugins]
android-application = { id = "com.android.application", version.ref = "agp" }
kotlin-android = { id = "org.jetbrains.kotlin.android", version.ref = "kotlin" }
kotlin-compose = { id = "org.jetbrains.kotlin.plugin.compose", version.ref = "kotlin" }
kotlin-serialization = { id = "org.jetbrains.kotlin.plugin.serialization", version.ref = "kotlin" }
ksp = { id = "com.google.devtools.ksp", version.ref = "ksp" }
hilt = { id = "com.google.dagger.hilt.android", version.ref = "hilt" }
```

- [ ] **Step 5: `android/build.gradle.kts` を書く**

```kotlin
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ksp) apply false
    alias(libs.plugins.hilt) apply false
}
```

- [ ] **Step 6: `android/app/build.gradle.kts` を書く**

```kotlin
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.hilt)
}

android {
    namespace = "com.edgewatcher"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.edgewatcher"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"
        buildConfigField("String", "APP_VERSION", "\"1.0.0\"")
    }

    buildTypes {
        debug {
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${project.findProperty("API_BASE_URL_DEV")}\"",
            )
        }
        release {
            isMinifyEnabled = false
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${project.findProperty("API_BASE_URL_PROD")}\"",
            )
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.lifecycle.viewmodel.compose)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)

    implementation(libs.hilt.android)
    implementation(libs.hilt.navigation.compose)
    ksp(libs.hilt.compiler)

    implementation(libs.camerax.core)
    implementation(libs.camerax.camera2)
    implementation(libs.camerax.lifecycle)
    implementation(libs.camerax.view)
    implementation(libs.mlkit.barcode)

    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    implementation(libs.retrofit)
    implementation(libs.retrofit.serialization)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.coroutines.play.services)

    implementation(libs.androidx.security.crypto)
    implementation(libs.play.services.location)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
```

- [ ] **Step 7: `android/app/src/main/AndroidManifest.xml` を書く**

`ACCESS_BACKGROUND_LOCATION` は書かない。FGS の型は `camera|location` の両方。

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">

    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.CAMERA" />
    <uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />
    <uses-permission android:name="android.permission.ACCESS_COARSE_LOCATION" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_CAMERA" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_LOCATION" />
    <uses-permission android:name="android.permission.WAKE_LOCK" />
    <uses-permission android:name="android.permission.SCHEDULE_EXACT_ALARM" />
    <uses-permission android:name="android.permission.RECEIVE_BOOT_COMPLETED" />
    <uses-permission android:name="android.permission.REQUEST_IGNORE_BATTERY_OPTIMIZATIONS" />

    <uses-feature android:name="android.hardware.camera.any" android:required="true" />

    <application
        android:name=".EdgeWatcherApp"
        android:allowBackup="false"
        android:label="EdgeWatcher"
        android:supportsRtl="false"
        android:usesCleartextTraffic="false"
        android:theme="@style/Theme.Material3.DayNight.NoActionBar">

        <activity
            android:name=".presentation.MainActivity"
            android:exported="true"
            android:launchMode="singleTask"
            android:screenOrientation="portrait">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>

        <service
            android:name=".infrastructure.service.ObservationService"
            android:exported="false"
            android:foregroundServiceType="camera|location" />

        <receiver
            android:name=".infrastructure.service.BootReceiver"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.BOOT_COMPLETED" />
            </intent-filter>
        </receiver>

        <receiver
            android:name=".infrastructure.service.AlarmReceiver"
            android:exported="false" />
    </application>
</manifest>
```

- [ ] **Step 8: `EdgeWatcherApp.kt` を書く**

`android/app/src/main/java/com/edgewatcher/EdgeWatcherApp.kt`:

```kotlin
package com.edgewatcher

import android.app.Application
import dagger.hilt.android.HiltAndroidApp

@HiltAndroidApp
class EdgeWatcherApp : Application()
```

- [ ] **Step 9: `android/.gitignore` を書く**

```gitignore
.gradle/
build/
local.properties
*.iml
.idea/
.kotlin/
```

- [ ] **Step 10: ビルドが通ることを確認する**

`MainActivity` と `ObservationService` と `BootReceiver` と `AlarmReceiver` はまだ無いので、
この時点ではマニフェストの参照解決でリンクが失敗する。**Step 7 のマニフェストから
`<activity>` `<service>` `<receiver>` x2 の**4ブロック**を一時的にコメントアウトし**、
アプリ本体だけが通ることを確認する。Task 20 でコメントを外す。

Run: `cd android && ./gradlew :app:assembleDebug`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 11: コミット**

```bash
git add android/
git commit -m "feat(android): Gradle の骨組みと Hilt の土台を作る"
```

---

## Task 2: ULID の採番（**テスト1**）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/domain/UlidGenerator.kt`
- Test: `android/app/src/test/java/com/edgewatcher/domain/UlidGeneratorTest.kt`

**Interfaces:**
- Consumes: なし
- Produces: `object UlidGenerator { fun generate(at: java.time.Instant, random: java.util.Random = java.util.Random()): String }`

**Why this is tested:** 採番時刻が `capturedAt` からずれると、サーバが ULID の
タイムスタンプ部から日別クエリの範囲を導くため、観測が別の日に並ぶ。
**端末の画面には何も現れない**（spec §10）。

**Background:** ULID は 26 文字の Crockford Base32。先頭 10 文字が
ミリ秒精度の Unix 時刻（48 bit）、残り 16 文字がランダム（80 bit）。
Crockford の英字は `0123456789ABCDEFGHJKMNPQRSTVWXYZ`（I・L・O・U を除く）。

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/java/com/edgewatcher/domain/UlidGeneratorTest.kt`:

```kotlin
package com.edgewatcher.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.util.Random

class UlidGeneratorTest {

    private val alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    /** 先頭10文字を 48bit のミリ秒に戻す。テスト側で独立に復号する。 */
    private fun decodeTimestamp(ulid: String): Long =
        ulid.take(10).fold(0L) { acc, c -> acc * 32 + alphabet.indexOf(c) }

    @Test
    fun `timestamp part equals the instant it was minted for`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val ulid = UlidGenerator.generate(at)

        assertEquals(at.toEpochMilli(), decodeTimestamp(ulid))
    }

    @Test
    fun `timestamp part follows the argument, not the wall clock`() {
        val past = Instant.parse("2020-01-02T03:04:05.006Z")

        val ulid = UlidGenerator.generate(past)

        assertEquals(past.toEpochMilli(), decodeTimestamp(ulid))
    }

    @Test
    fun `is 26 characters of Crockford base32`() {
        val ulid = UlidGenerator.generate(Instant.parse("2026-09-10T14:32:05.123Z"))

        assertEquals(26, ulid.length)
        assertTrue(ulid.all { it in alphabet })
    }

    @Test
    fun `two ulids for the same instant differ in the random part`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val a = UlidGenerator.generate(at)
        val b = UlidGenerator.generate(at)

        assertEquals(a.take(10), b.take(10))
        assertNotEquals(a.drop(10), b.drop(10))
    }

    @Test
    fun `lexical order follows chronological order`() {
        val earlier = UlidGenerator.generate(Instant.parse("2026-09-10T00:00:00Z"))
        val later = UlidGenerator.generate(Instant.parse("2026-09-10T00:00:01Z"))

        assertTrue(earlier < later)
    }

    @Test
    fun `random part is drawn from the supplied source`() {
        val at = Instant.parse("2026-09-10T14:32:05.123Z")

        val a = UlidGenerator.generate(at, Random(42))
        val b = UlidGenerator.generate(at, Random(42))

        assertEquals(a, b)
    }
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*UlidGeneratorTest*'`
Expected: コンパイルエラー。`Unresolved reference: UlidGenerator`

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/java/com/edgewatcher/domain/UlidGenerator.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import java.util.Random

/**
 * ULID を採番する。
 *
 * **採番時刻を必ず引数で受け取る。** サーバは日別クエリの範囲を ULID の
 * タイムスタンプ部から導くため、ここが `capturedAt` からずれると観測が
 * 別の日に並ぶ。端末の画面には何も現れないので、実機では気づけない。
 * 引数なしで呼べるオーバーロードを足してはならない。
 */
object UlidGenerator {

    /** Crockford Base32。I・L・O・U を除いた32文字。 */
    private const val ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    private const val TIME_CHARS = 10
    private const val RANDOM_CHARS = 16

    fun generate(at: Instant, random: Random = Random()): String {
        val builder = StringBuilder(TIME_CHARS + RANDOM_CHARS)
        encodeTime(at.toEpochMilli(), builder)
        repeat(RANDOM_CHARS) { builder.append(ALPHABET[random.nextInt(32)]) }
        return builder.toString()
    }

    /** 48bit を上位から5bitずつ、10文字に詰める。 */
    private fun encodeTime(millis: Long, out: StringBuilder) {
        for (shift in (TIME_CHARS - 1) downTo 0) {
            val index = ((millis ushr (shift * 5)) and 0x1F).toInt()
            out.append(ALPHABET[index])
        }
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*UlidGeneratorTest*'`
Expected: 6件すべて PASS

- [ ] **Step 5: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/domain/UlidGenerator.kt \
        android/app/src/test/java/com/edgewatcher/domain/UlidGeneratorTest.kt
git commit -m "feat(android): 採番時刻を引数で受ける ULID を実装する"
```

---

## Task 3: モデル・ポート・失敗の分類（**テスト2**）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/domain/model/Models.kt`
- Create: `android/app/src/main/java/com/edgewatcher/domain/error/ApiFailure.kt`
- Create: `android/app/src/main/java/com/edgewatcher/domain/error/ApiFailures.kt`
- Create: `android/app/src/main/java/com/edgewatcher/domain/port/Ports.kt`
- Test: `android/app/src/test/java/com/edgewatcher/domain/error/ApiFailuresTest.kt`

**Interfaces:**
- Consumes: `UlidGenerator`（Task 2）
- Produces: 以下すべて。**後続タスクはこの名前と型に従う。**
  - `DeviceCredentials(deviceId: String, deviceSecret: String)`
  - `Session(token: String, expiresAtEpochSeconds: Long)`
  - `DeviceInfo(model: String, osVersion: String, appVersion: String)`
  - `Coordinates(lat: Double, lng: Double)`
  - `PendingObservation(observationId: String, capturedAt: Instant, coordinates: Coordinates?, totalBytes: Long)`
  - `IntervalMinutes`（`FIVE` `TEN` `FIFTEEN`、`minutes: Int`、`DEFAULT = FIVE`、`fromMinutes(Int): IntervalMinutes?`）
  - `ObservationState`（`Observing(lastUploadAt: Instant?)` / `Offline(pendingCount: Int)` / `Stopped`）
  - `ApiFailure`（`Retryable` / `Discard` / `AuthExpired` / `CredentialsRevoked` / `PairingNotFound` / `PairingRejected(reason)`）
  - `ApiResult<T>`（`Success<T>(value)` / `Failure(failure)`）
  - `ApiFailures.forUpload(status: Int): ApiFailure`
  - `ApiFailures.forTokenRefresh(status: Int): ApiFailure`
  - `ApiFailures.forPairing(status: Int, code: String?): ApiFailure`
  - `ApiFailures.forNetworkError(): ApiFailure`
  - ポート9本: `Clock` `IdGenerator` `CredentialStore` `ObservationBuffer`
    `ObservationApi` `CameraGateway` `JpegEncoder` `LocationGateway` `AlarmScheduler`

**Why the classification is tested:** 1行の取り違えで「二度と繋がらない」か
「無限再送」になる。特に **5xx で資格情報を消してはならない**という規則
（spec §8.2）をここで固定する。取り違えると、圏外になっただけの端末が
資格情報を捨て、復帰に現地への物理的な訪問が必要になる。

- [ ] **Step 1: モデルを書く**

`android/app/src/main/java/com/edgewatcher/domain/model/Models.kt`:

```kotlin
package com.edgewatcher.domain.model

import java.time.Instant

/** 長期資格情報。deviceSecret はセッションの再取得にしか使わない。 */
data class DeviceCredentials(
    val deviceId: String,
    val deviceSecret: String,
)

/** 日常のアップロードに使う短期資格情報。有効期限は12時間。 */
data class Session(
    val token: String,
    val expiresAtEpochSeconds: Long,
)

/** ペアリング時に自己申告する。表示専用で、何の認可判断にも使われない。 */
data class DeviceInfo(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

data class Coordinates(
    val lat: Double,
    val lng: Double,
)

/**
 * 送信待ちの観測1件のメタデータ。
 *
 * 画像そのものはファイルに置き、ここには持たない。BLOB を SQLite に入れると
 * DB が肥大し、削除しても領域が戻りにくいため。
 */
data class PendingObservation(
    val observationId: String,
    val capturedAt: Instant,
    val coordinates: Coordinates?,
    /** 本画像 + サムネイルの合計。eviction の 500MiB 判定に使う。 */
    val totalBytes: Long,
)

/** 送信間隔。値を決めるのは Web 側で、端末は nextConfig から受け取る。 */
enum class IntervalMinutes(val minutes: Int) {
    FIVE(5),
    TEN(10),
    FIFTEEN(15),
    ;

    companion object {
        /**
         * サーバから nextConfig が届くまでの既定値。
         *
         * **backend の internal/webapi/webapi.go の
         * `defaultInterval = api.IntervalOptions[0]` と同一でなければならない。**
         * 片方だけが変更されても機械的には検出できない。
         */
        val DEFAULT = FIVE

        fun fromMinutes(value: Int): IntervalMinutes? = entries.firstOrNull { it.minutes == value }
    }
}

/** 画面と常駐通知が同じものを出す。文言の生成は StatusLine が持つ。 */
sealed interface ObservationState {
    /** 直近の送信に成功している。 */
    data class Observing(val lastUploadAt: Instant?) : ObservationState

    /** 送信に失敗し、バッファに滞留している。 */
    data class Offline(val pendingCount: Int) : ObservationState

    /** サービスが動いていない。この状態のときだけ [観測を再開] を出す。 */
    data object Stopped : ObservationState
}
```

- [ ] **Step 2: 失敗の型を書く**

`android/app/src/main/java/com/edgewatcher/domain/error/ApiFailure.kt`:

```kotlin
package com.edgewatcher.domain.error

/**
 * API 呼び出しの失敗を、**呼び出し側が取るべき行動**で分類したもの。
 *
 * HTTP のステータスコードを domain の外へ漏らさないための型でもある。
 * 分類の判断は本文に依存させない（本文の形が2種類あるため。spec §8.4）。
 */
sealed interface ApiFailure {
    /** 5xx・タイムアウト・通信断。バックオフして同じものを再試行してよい。 */
    data object Retryable : ApiFailure

    /** 400 / 413。本文に起因するため、再送しても永久に通らない。捨てる。 */
    data object Discard : ApiFailure

    /** 401 / 403。セッションが切れた。POST /device/token で再取得する。 */
    data object AuthExpired : ApiFailure

    /**
     * POST /device/token が 401 を返した。deviceSecret が無効。
     *
     * **資格情報を消してよいのはこれを受け取ったときだけである。**
     */
    data object CredentialsRevoked : ApiFailure

    /** POST /device/pair の 404。GSI2 の伝播待ちなので、待てば成功しうる。 */
    data object PairingNotFound : ApiFailure

    /** QR が使用済み・期限切れ・本文不正。リトライしても解決しない。 */
    data class PairingRejected(val reason: Reason) : ApiFailure {
        enum class Reason { CONSUMED, EXPIRED, INVALID }
    }
}

sealed interface ApiResult<out T> {
    data class Success<T>(val value: T) : ApiResult<T>
    data class Failure(val failure: ApiFailure) : ApiResult<Nothing>
}
```

- [ ] **Step 3: 失敗するテストを書く**

`android/app/src/test/java/com/edgewatcher/domain/error/ApiFailuresTest.kt`:

```kotlin
package com.edgewatcher.domain.error

import org.junit.Assert.assertEquals
import org.junit.Test

class ApiFailuresTest {

    // ---- アップロード（POST /device/uploads） ----

    @Test
    fun `upload 400 is discarded, never retried`() {
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(400))
    }

    @Test
    fun `upload 413 is discarded, never retried`() {
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(413))
    }

    @Test
    fun `upload 401 and 403 both mean the session expired`() {
        // 端末ルートでセッションが切れたときオーソライザが返すのは 403。
        // 401 だけを見ていると、セッション失効から永久に回復できない。
        assertEquals(ApiFailure.AuthExpired, ApiFailures.forUpload(401))
        assertEquals(ApiFailure.AuthExpired, ApiFailures.forUpload(403))
    }

    @Test
    fun `upload 5xx is retryable`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(500))
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(502))
        assertEquals(ApiFailure.Retryable, ApiFailures.forUpload(503))
    }

    @Test
    fun `an unmapped 4xx on upload is discarded, not retried`() {
        // 本文に起因する失敗を再送し続けると、そのフレームで永久に詰まる。
        assertEquals(ApiFailure.Discard, ApiFailures.forUpload(422))
    }

    // ---- セッション再取得（POST /device/token） ----

    @Test
    fun `token refresh 401 revokes the credentials`() {
        assertEquals(ApiFailure.CredentialsRevoked, ApiFailures.forTokenRefresh(401))
    }

    @Test
    fun `token refresh 5xx does NOT revoke the credentials`() {
        // これを取り違えると、圏外になっただけの端末が deviceSecret を捨て、
        // 復帰に Web からの QR 再発行と現地への訪問が必要になる。
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(500))
        assertEquals(ApiFailure.Retryable, ApiFailures.forTokenRefresh(503))
    }

    @Test
    fun `a network error never revokes the credentials`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forNetworkError())
    }

    // ---- ペアリング（POST /device/pair） ----

    @Test
    fun `pairing 404 is retryable because the lookup goes through an eventually consistent index`() {
        assertEquals(ApiFailure.PairingNotFound, ApiFailures.forPairing(404, "PAIRING_NOT_FOUND"))
    }

    @Test
    fun `pairing 409 is rejected outright and distinguishes consumed from expired`() {
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.CONSUMED),
            ApiFailures.forPairing(409, "PAIRING_CODE_CONSUMED"),
        )
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.EXPIRED),
            ApiFailures.forPairing(409, "PAIRING_CODE_EXPIRED"),
        )
    }

    @Test
    fun `pairing 400 is rejected as invalid`() {
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.INVALID),
            ApiFailures.forPairing(400, "VALIDATION_ERROR"),
        )
    }

    @Test
    fun `pairing 5xx is retryable`() {
        assertEquals(ApiFailure.Retryable, ApiFailures.forPairing(500, null))
    }

    @Test
    fun `pairing classification does not depend on the body`() {
        // 本文の形は2種類あり、パースに失敗することがある。
        // status だけで分類が決まらなければならない。
        assertEquals(ApiFailure.PairingNotFound, ApiFailures.forPairing(404, null))
        assertEquals(
            ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.CONSUMED),
            ApiFailures.forPairing(409, null),
        )
    }
}
```

- [ ] **Step 4: テストが失敗することを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*ApiFailuresTest*'`
Expected: コンパイルエラー。`Unresolved reference: ApiFailures`

- [ ] **Step 5: 分類を実装する**

`android/app/src/main/java/com/edgewatcher/domain/error/ApiFailures.kt`:

```kotlin
package com.edgewatcher.domain.error

/**
 * HTTP のステータスコードを、呼び出し側が取るべき行動へ写像する。
 *
 * **判断を本文に依存させない。** アプリケーション層のエラーは
 * `{"error":{"code","message"}}` だが、オーソライザと API Gateway が返す
 * 401 / 403 / 413 は `{"message":"..."}` である。パースに失敗しても
 * status だけで分類が決まること（spec §8.4）。
 */
object ApiFailures {

    /**
     * POST /device/uploads と POST /device/logout の分類。
     *
     * 4xx はすべてリトライ不能として捨てる。5xx を返さないのはサーバ側の
     * 約束であり、こちらが 4xx を再送すると、そのフレームで永久に詰まる。
     */
    fun forUpload(status: Int): ApiFailure = when {
        status == 401 || status == 403 -> ApiFailure.AuthExpired
        status in 400..499 -> ApiFailure.Discard
        else -> ApiFailure.Retryable
    }

    /**
     * POST /device/token の分類。
     *
     * **401 だけが資格情報を無効と断定できる唯一の応答である。**
     * サーバは失敗をすべて 401 に寄せており（404 も 403 も返らない）、
     * 5xx や通信断は「サーバ側の都合」であって切断ではない。
     */
    fun forTokenRefresh(status: Int): ApiFailure = when (status) {
        401 -> ApiFailure.CredentialsRevoked
        else -> ApiFailure.Retryable
    }

    /**
     * POST /device/pair の分類。
     *
     * 404 だけがリトライ対象。pairingCode の逆引きが結果整合な GSI2 を通るため、
     * QR 発行直後は伝播が間に合わないことがある。409 を 404 に丸めると絶対に
     * 成功しない QR を回し続け、404 を 409 に丸めると成功するはずのペアリングを諦める。
     *
     * `code` は 409 の内訳（消費済みか期限切れか）を分けるためだけに使い、
     * 欠けていても分類そのものは status で決まる。
     */
    fun forPairing(status: Int, code: String?): ApiFailure = when (status) {
        404 -> ApiFailure.PairingNotFound
        409 -> ApiFailure.PairingRejected(
            when (code) {
                "PAIRING_CODE_EXPIRED" -> ApiFailure.PairingRejected.Reason.EXPIRED
                else -> ApiFailure.PairingRejected.Reason.CONSUMED
            },
        )
        400 -> ApiFailure.PairingRejected(ApiFailure.PairingRejected.Reason.INVALID)
        in 500..599 -> ApiFailure.Retryable
        else -> ApiFailure.Retryable
    }

    /** 通信できなかった。**資格情報を消す理由にはならない。** */
    fun forNetworkError(): ApiFailure = ApiFailure.Retryable
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*ApiFailuresTest*'`
Expected: 13件すべて PASS

- [ ] **Step 7: ポートを書く**

`android/app/src/main/java/com/edgewatcher/domain/port/Ports.kt`:

```kotlin
package com.edgewatcher.domain.port

import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.Coordinates
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.model.Session
import java.time.Instant

interface Clock {
    fun now(): Instant
}

interface IdGenerator {
    /** 採番時刻を必ず受け取る。引数なしのオーバーロードを足さないこと。 */
    fun ulid(at: Instant): String
}

/** 資格情報と、サーバから受け取った設定の保管。 */
interface CredentialStore {
    fun readCredentials(): DeviceCredentials?
    fun writeCredentials(credentials: DeviceCredentials)
    fun readSession(): Session?
    fun writeSession(session: Session)
    fun readInterval(): IntervalMinutes
    fun writeInterval(interval: IntervalMinutes)

    /** 資格情報・セッション・設定をすべて消す。 */
    fun clear()
}

/**
 * 送信待ちの観測の保管。**行と画像ファイルを対で扱う。**
 * 片方だけが残る状態を外から作れないようにする責務がここにある。
 */
interface ObservationBuffer {
    suspend fun save(observation: PendingObservation, image: ByteArray, thumbnail: ByteArray)
    suspend fun readImage(observationId: String): ByteArray?
    suspend fun readThumbnail(observationId: String): ByteArray?
    suspend fun remove(observationId: String)
    suspend fun removeAll()
    suspend fun count(): Int
    suspend fun totalBytes(): Long

    /** capturedAt の昇順。BufferPolicy と UploadOrder はこの順序を前提にする。 */
    suspend fun allOldestFirst(): List<PendingObservation>
}

interface ObservationApi {
    suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): ApiResult<DeviceCredentials>
    suspend fun issueSession(credentials: DeviceCredentials): ApiResult<Session>

    /** 成功すると nextConfig の間隔が返る。**必ず適用すること。** */
    suspend fun upload(
        sessionToken: String,
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ): ApiResult<IntervalMinutes>

    suspend fun logout(sessionToken: String): ApiResult<Unit>
}

/** カメラの所有権は Foreground Service が持つ。これはその窓口。 */
interface CameraGateway {
    /** 1枚撮る。フル解像度の JPEG を返す。 */
    suspend fun capture(): ByteArray
}

/** 言われた寸法で符号化するだけ。何段階で落とすかは ImagePolicy が決める。 */
interface JpegEncoder {
    fun encode(source: ByteArray, longestSide: Int, quality: Int): ByteArray
}

interface LocationGateway {
    /** 最後の既知位置。**測位を待たない。** 取得できなければ null。 */
    suspend fun lastKnown(): Coordinates?
}

interface AlarmScheduler {
    fun scheduleNext(at: Instant)
    fun cancel()
}
```

- [ ] **Step 8: コンパイルとテストが通ることを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest`
Expected: `BUILD SUCCESSFUL`。Task 2 の6件と本タスクの13件が PASS

- [ ] **Step 9: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/domain/ \
        android/app/src/test/java/com/edgewatcher/domain/
git commit -m "feat(android): domain のモデル・ポートと、失敗の分類を実装する"
```

---

## Task 4: バッファの eviction（**テスト3**）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/domain/BufferPolicy.kt`
- Test: `android/app/src/test/java/com/edgewatcher/domain/BufferPolicyTest.kt`

**Interfaces:**
- Consumes: `PendingObservation`（Task 3）
- Produces: `object BufferPolicy { const val MAX_COUNT: Int; const val MAX_BYTES: Long; fun evictions(oldestFirst: List<PendingObservation>): List<String> }`
  — 返すのは**削除すべき `observationId` のリスト**（古い順）

**Why this is tested:** 実機で 500MiB まで貯めて確かめるのは非現実的（spec §10）。

**Rule:** 288件 または 524,288,000 バイトのいずれか先に達したら、
**両方の条件を満たすまで**古いものから削除する。

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/java/com/edgewatcher/domain/BufferPolicyTest.kt`:

```kotlin
package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

class BufferPolicyTest {

    /** capturedAt は index 分だけ後ろにずらす。index が小さいほど古い。 */
    private fun observation(index: Int, bytes: Long): PendingObservation =
        PendingObservation(
            observationId = "OBS%04d".format(index),
            capturedAt = Instant.parse("2026-09-10T00:00:00Z").plusSeconds(index * 300L),
            coordinates = null,
            totalBytes = bytes,
        )

    private fun buffer(count: Int, bytesEach: Long): List<PendingObservation> =
        (0 until count).map { observation(it, bytesEach) }

    @Test
    fun `an empty buffer evicts nothing`() {
        assertEquals(emptyList<String>(), BufferPolicy.evictions(emptyList()))
    }

    @Test
    fun `a buffer at exactly the count limit evicts nothing`() {
        val rows = buffer(count = 288, bytesEach = 300_000)

        assertEquals(emptyList<String>(), BufferPolicy.evictions(rows))
    }

    @Test
    fun `one row over the count limit evicts exactly the oldest one`() {
        val rows = buffer(count = 289, bytesEach = 300_000)

        assertEquals(listOf("OBS0000"), BufferPolicy.evictions(rows))
    }

    @Test
    fun `five rows over the count limit evict the five oldest, in order`() {
        val rows = buffer(count = 293, bytesEach = 300_000)

        assertEquals(
            listOf("OBS0000", "OBS0001", "OBS0002", "OBS0003", "OBS0004"),
            BufferPolicy.evictions(rows),
        )
    }

    @Test
    fun `the byte limit can bite well before the count limit`() {
        // 10件 x 60MiB = 600MiB > 500MiB。件数は 10 で上限のはるか下。
        val sixtyMiB = 62_914_560L
        val rows = buffer(count = 10, bytesEach = sixtyMiB)

        val evicted = BufferPolicy.evictions(rows)

        assertTrue("byte limit must evict even well under 288 rows", evicted.isNotEmpty())
        val remainingBytes = rows.filterNot { it.observationId in evicted }.sumOf { it.totalBytes }
        assertTrue(remainingBytes <= BufferPolicy.MAX_BYTES)
    }

    @Test
    fun `the byte limit evicts the fewest rows that bring it under, oldest first`() {
        val sixtyMiB = 62_914_560L
        val rows = buffer(count = 10, bytesEach = sixtyMiB)

        // 600MiB を 500MiB 以下にするには 2件（120MiB）落とせば足りる。
        assertEquals(listOf("OBS0000", "OBS0001"), BufferPolicy.evictions(rows))
    }

    @Test
    fun `a buffer at exactly the byte limit evicts nothing`() {
        val rows = listOf(observation(0, BufferPolicy.MAX_BYTES))

        assertEquals(emptyList<String>(), BufferPolicy.evictions(rows))
    }

    @Test
    fun `whichever limit is exceeded governs, and both end up satisfied`() {
        // 300件 x 2MiB = 600MiB。件数も容量も超えている。
        val twoMiB = 2_097_152L
        val rows = buffer(count = 300, bytesEach = twoMiB)

        val evicted = BufferPolicy.evictions(rows)
        val remaining = rows.filterNot { it.observationId in evicted }

        assertTrue(remaining.size <= BufferPolicy.MAX_COUNT)
        assertTrue(remaining.sumOf { it.totalBytes } <= BufferPolicy.MAX_BYTES)
        // 落とすのは常に古い側から。新しい側が残る。
        assertEquals("OBS0299", remaining.last().observationId)
    }

    @Test
    fun `the limits are the values the spec fixed`() {
        assertEquals(288, BufferPolicy.MAX_COUNT)
        assertEquals(524_288_000L, BufferPolicy.MAX_BYTES)
    }
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*BufferPolicyTest*'`
Expected: コンパイルエラー。`Unresolved reference: BufferPolicy`

- [ ] **Step 3: 実装を書く**

`android/app/src/main/java/com/edgewatcher/domain/BufferPolicy.kt`:

```kotlin
package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation

/**
 * オフラインバッファの上限を決める。
 *
 * 件数だけで制限すると高解像度設定でストレージを圧迫し、容量だけで制限すると
 * 低解像度設定で保持期間を超えた画像を大量に抱える。両方を置くことで、
 * どちらの設定でも破綻しない。
 *
 * バッファ上限を保持期間より長くしても意味がない。観測レコードの TTL は
 * capturedAt を基準に決まるため、長時間オフラインだった端末が古い画像を
 * 送っても、サーバ到着時点で期限切れになりうる。
 */
object BufferPolicy {

    /** 5分間隔で24時間相当。 */
    const val MAX_COUNT = 288

    /** 500MiB。 */
    const val MAX_BYTES = 524_288_000L

    /**
     * 削除すべき observationId を、古い順に返す。
     *
     * @param oldestFirst capturedAt の昇順であること。
     * @return 削除対象。何も落とす必要がなければ空。
     */
    fun evictions(oldestFirst: List<PendingObservation>): List<String> {
        var count = oldestFirst.size
        var bytes = oldestFirst.sumOf { it.totalBytes }

        val doomed = mutableListOf<String>()
        for (row in oldestFirst) {
            if (count <= MAX_COUNT && bytes <= MAX_BYTES) break
            doomed += row.observationId
            count -= 1
            bytes -= row.totalBytes
        }
        return doomed
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*BufferPolicyTest*'`
Expected: 9件すべて PASS

- [ ] **Step 5: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/domain/BufferPolicy.kt \
        android/app/src/test/java/com/edgewatcher/domain/BufferPolicyTest.kt
git commit -m "feat(android): バッファの eviction を実装する"
```

---

## Task 5: 送信順序（**テスト4**）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/domain/UploadOrder.kt`
- Test: `android/app/src/test/java/com/edgewatcher/domain/UploadOrderTest.kt`

**Interfaces:**
- Consumes: `PendingObservation`（Task 3）
- Produces: `object UploadOrder { fun order(oldestFirst: List<PendingObservation>): List<PendingObservation> }`

**Why this is tested:** 復帰時にしか現れず、実機での再現に長時間のオフラインを要する（spec §10）。

**Rule:** **最新の1枚を先に送り、そのあと残りを古い順に送る。** 単純な FIFO では
Web のダッシュボードに何時間も前の画像が出続ける。オーナーが最初に知りたいのは
「今どうなっているか」である。

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/java/com/edgewatcher/domain/UploadOrderTest.kt`:

```kotlin
package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant

class UploadOrderTest {

    private fun observation(index: Int): PendingObservation =
        PendingObservation(
            observationId = "OBS%04d".format(index),
            capturedAt = Instant.parse("2026-09-10T00:00:00Z").plusSeconds(index * 300L),
            coordinates = null,
            totalBytes = 300_000,
        )

    private fun ids(rows: List<PendingObservation>) = rows.map { it.observationId }

    @Test
    fun `an empty buffer produces an empty order`() {
        assertEquals(emptyList<PendingObservation>(), UploadOrder.order(emptyList()))
    }

    @Test
    fun `a single row is sent as-is`() {
        val rows = listOf(observation(0))

        assertEquals(listOf("OBS0000"), ids(UploadOrder.order(rows)))
    }

    @Test
    fun `the newest goes first, then the rest oldest-first`() {
        val rows = (0..4).map { observation(it) }

        assertEquals(
            listOf("OBS0004", "OBS0000", "OBS0001", "OBS0002", "OBS0003"),
            ids(UploadOrder.order(rows)),
        )
    }

    @Test
    fun `the newest is not sent twice`() {
        val rows = (0..4).map { observation(it) }

        val ordered = UploadOrder.order(rows)

        assertEquals(rows.size, ordered.size)
        assertEquals(rows.size, ordered.map { it.observationId }.toSet().size)
    }

    @Test
    fun `a long backlog still puts the newest first`() {
        val rows = (0..287).map { observation(it) }

        val ordered = UploadOrder.order(rows)

        assertEquals("OBS0287", ordered.first().observationId)
        assertEquals("OBS0000", ordered[1].observationId)
        assertEquals("OBS0286", ordered.last().observationId)
    }

    @Test
    fun `input that is not sorted is still ordered by capturedAt`() {
        val rows = listOf(observation(2), observation(0), observation(4), observation(1))

        assertEquals(
            listOf("OBS0004", "OBS0000", "OBS0001", "OBS0002"),
            ids(UploadOrder.order(rows)),
        )
    }
}
```

- [ ] **Step 2: テストが失敗することを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*UploadOrderTest*'`
Expected: コンパイルエラー。`Unresolved reference: UploadOrder`

- [ ] **Step 3: 実装を書く**

`android/app/src/main/java/com/edgewatcher/domain/UploadOrder.kt`:

```kotlin
package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation

/**
 * 送信の順序を決める。
 *
 * **最新の1枚を先に送り、そのあと残りを古い順に送る。**
 *
 * 長時間オフラインから復帰したとき、単純な FIFO で古い順に送ると、Web の
 * ダッシュボードには何時間も前の画像が出続ける。オーナーが最初に知りたいのは
 * 「今どうなっているか」であり、それが即座に反映されるようにする。
 *
 * 遅れて届いた古い画像が Device.latestThumbnailKey を巻き戻さないよう、
 * サーバ側は条件付き更新を行っている（engineering/dynamodb.md §5.1）。
 * したがって端末はこの順序を素直に守ってよい。
 */
object UploadOrder {

    /**
     * @param oldestFirst 通常は capturedAt の昇順。順不同で渡されても正しく並べる。
     */
    fun order(oldestFirst: List<PendingObservation>): List<PendingObservation> {
        if (oldestFirst.size <= 1) return oldestFirst

        val sorted = oldestFirst.sortedBy { it.capturedAt }
        val newest = sorted.last()
        return listOf(newest) + sorted.dropLast(1)
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest --tests '*UploadOrderTest*'`
Expected: 6件すべて PASS

- [ ] **Step 5: 4本のテストがすべて揃ったことを確認する**

Run: `cd android && ./gradlew :app:testDebugUnitTest`
Expected: `BUILD SUCCESSFUL`。合計34件が PASS
（Ulid 6 + ApiFailures 13 + BufferPolicy 9 + UploadOrder 6）

**これで spec §10 が要求するテストはすべて揃った。以降のタスクでテストを足さない。**

- [ ] **Step 6: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/domain/UploadOrder.kt \
        android/app/src/test/java/com/edgewatcher/domain/UploadOrderTest.kt
git commit -m "feat(android): 最新1枚を先に送る送信順序を実装する"
```

---

## Task 6: 画像方針と状態行の文言

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/domain/ImagePolicy.kt`
- Create: `android/app/src/main/java/com/edgewatcher/domain/StatusLine.kt`

**Interfaces:**
- Consumes: `ObservationState`（Task 3）
- Produces:
  - `ImagePolicy.EncodeStep(longestSide: Int, quality: Int)`
  - `ImagePolicy.THUMBNAIL: EncodeStep`（480 / 75）
  - `ImagePolicy.MAIN_STEPS: List<EncodeStep>`
  - `ImagePolicy.MAX_TOTAL_BYTES: Long`
  - `ImagePolicy.fits(imageBytes: Int, thumbnailBytes: Int): Boolean`
  - `StatusLine.render(state: ObservationState, zone: ZoneId = ZoneId.systemDefault()): String`

**Note:** テストは書かない（Global Constraints）。完了条件はコンパイルが通ること。

- [ ] **Step 1: `ImagePolicy.kt` を書く**

```kotlin
package com.edgewatcher.domain

/**
 * 「どう符号化するか」の判断を持つ。画素の操作は JpegEncoder が行う。
 *
 * 長辺・品質・落とす段階は方針であって画像処理ではないので domain に置く。
 */
object ImagePolicy {

    data class EncodeStep(val longestSide: Int, val quality: Int)

    /** 本画像 + サムネイルの合計の上限。超過はサーバが 400 を返し、リトライ不能。 */
    const val MAX_TOTAL_BYTES = 4_500_000L

    /**
     * サムネイル。長辺480px。
     *
     * Web のダッシュボードは端末カードの画像に latestThumbnailUrl をそのまま
     * 使っており、カード幅は 280〜360px ある。160px では明確にぼやける。
     * 480px なら履歴のコマ列（表示は 80〜120px）にも耐える。
     */
    val THUMBNAIL = EncodeStep(longestSide = 480, quality = 75)

    /**
     * 本画像を符号化する段階。**先頭から順に試し、合計が収まった時点で止める。**
     *
     * 送っても必ず 400 になるフレームをバッファに積まないためのガードであり、
     * 通常は第1段階で収まる（実効 250〜450KB）。上限が routine な制約ではなく
     * 安全網として機能する状態を保つ。
     */
    val MAIN_STEPS = listOf(
        EncodeStep(longestSide = 1600, quality = 80),
        EncodeStep(longestSide = 1600, quality = 65),
        EncodeStep(longestSide = 1600, quality = 50),
        EncodeStep(longestSide = 1280, quality = 50),
    )

    fun fits(imageBytes: Int, thumbnailBytes: Int): Boolean =
        imageBytes.toLong() + thumbnailBytes.toLong() <= MAX_TOTAL_BYTES
}
```

- [ ] **Step 2: `StatusLine.kt` を書く**

```kotlin
package com.edgewatcher.domain

import com.edgewatcher.domain.model.ObservationState
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 稼働状態を1行の日本語にする。
 *
 * **画面と常駐通知が同じ文字列を使う。** 端末を持ち上げなくても状態が分かるように
 * するためであり、2箇所で文言が食い違わないよう生成をここに一本化する。
 *
 * 「オフライン」を明示するのは、画面を見ただけでは通信の成否が分からないため。
 * 屋外設置後にオーナーが端末を見に行く動機のほとんどが「本当に送れているのか」の
 * 確認であり、その答えを最初に出す。
 *
 * **端末名は出さない。** POST /device/pair の応答に含まれず、端末は自分の名前を
 * 知らないため。
 */
object StatusLine {

    private val timeFormat = DateTimeFormatter.ofPattern("HH:mm")

    fun render(state: ObservationState, zone: ZoneId = ZoneId.systemDefault()): String =
        when (state) {
            is ObservationState.Observing ->
                if (state.lastUploadAt == null) {
                    "観測中"
                } else {
                    "観測中 / 最終送信 " + timeFormat.format(state.lastUploadAt.atZone(zone))
                }

            is ObservationState.Offline -> "オフライン / " + state.pendingCount + "件待機中"

            ObservationState.Stopped -> "停止中"
        }
}
```

- [ ] **Step 3: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/domain/ImagePolicy.kt \
        android/app/src/main/java/com/edgewatcher/domain/StatusLine.kt
git commit -m "feat(android): 画像方針と状態行の文言を domain に置く"
```

---

## Task 7: 認証系のユースケース

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/usecase/PairDeviceUseCase.kt`
- Create: `android/app/src/main/java/com/edgewatcher/usecase/RefreshSessionUseCase.kt`
- Create: `android/app/src/main/java/com/edgewatcher/usecase/LogoutDeviceUseCase.kt`

**Interfaces:**
- Consumes: `ObservationApi` `CredentialStore` `ObservationBuffer`（Task 3 のポート）、
  `ApiFailure` `ApiResult`（Task 3）、`IntervalMinutes`（Task 3）
- Produces:
  - `class PairDeviceUseCase(api: ObservationApi, store: CredentialStore)`
    - `suspend operator fun invoke(qrPayload: String, deviceInfo: DeviceInfo): PairDeviceUseCase.Outcome`
    - `Outcome`: `Paired` / `NotEdgeWatcherQr` / `Rejected(reason: ApiFailure.PairingRejected.Reason)` / `Unavailable`
    - `companion object { const val QR_PREFIX = "ew1:"; const val MAX_RETRIES = 3; const val RETRY_INTERVAL_MILLIS = 1000L }`
  - `class RefreshSessionUseCase(api: ObservationApi, store: CredentialStore, buffer: ObservationBuffer)`
    - `suspend operator fun invoke(): RefreshSessionUseCase.Outcome`
    - `Outcome`: `Refreshed(session: Session)` / `Revoked` / `Unavailable`
  - `class LogoutDeviceUseCase(api: ObservationApi, store: CredentialStore, buffer: ObservationBuffer)`
    - `suspend operator fun invoke(): Boolean`

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `PairDeviceUseCase.kt` を書く**

404 のリトライは **初回1回 + リトライ3回 = 最大4リクエスト、間隔1秒**。

```kotlin
package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import kotlinx.coroutines.delay

/**
 * QR を deviceSecret と交換し、続けてセッションを取得する。
 *
 * 404 だけをリトライするのは、pairingCode の逆引きが結果整合な GSI2 を通るため。
 * QR 発行直後にスキャンすると伝播が間に合わず、**待てば成功する空振り**が起こりうる。
 * 409 を 404 に丸めると絶対に成功しない QR を回し続け、404 を 409 に丸めると
 * 成功するはずのペアリングを諦める。
 */
class PairDeviceUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
) {

    sealed interface Outcome {
        data object Paired : Outcome

        /** ew1: で始まらない。他のアプリの QR なので、黙って読み取りを続ける。 */
        data object NotEdgeWatcherQr : Outcome

        data class Rejected(val reason: ApiFailure.PairingRejected.Reason) : Outcome

        /** 通信できなかった / サーバ側の障害 / 伝播待ちが解消しなかった。 */
        data object Unavailable : Outcome
    }

    suspend operator fun invoke(qrPayload: String, deviceInfo: DeviceInfo): Outcome {
        if (!qrPayload.startsWith(QR_PREFIX)) return Outcome.NotEdgeWatcherQr
        val pairingCode = qrPayload.removePrefix(QR_PREFIX)
        if (pairingCode.isEmpty()) return Outcome.NotEdgeWatcherQr

        var retries = 0
        while (true) {
            when (val paired = api.pair(pairingCode, deviceInfo)) {
                is ApiResult.Success -> {
                    // deviceSecret はシステム全体でこれが平文で流れる唯一の場所。
                    // サーバは SHA-256 しか持たないため、失うと QR の再発行しかない。
                    store.writeCredentials(paired.value)
                    store.writeInterval(IntervalMinutes.DEFAULT)

                    return when (val session = api.issueSession(paired.value)) {
                        is ApiResult.Success -> {
                            store.writeSession(session.value)
                            Outcome.Paired
                        }
                        is ApiResult.Failure -> Outcome.Unavailable
                    }
                }

                is ApiResult.Failure -> when (val failure = paired.failure) {
                    ApiFailure.PairingNotFound -> {
                        if (retries >= MAX_RETRIES) return Outcome.Unavailable
                        retries += 1
                        delay(RETRY_INTERVAL_MILLIS)
                    }

                    is ApiFailure.PairingRejected -> return Outcome.Rejected(failure.reason)

                    else -> return Outcome.Unavailable
                }
            }
        }
    }

    companion object {
        /** QR のペイロードは "ew1:<pairingCode>"。deviceId は含まない。 */
        const val QR_PREFIX = "ew1:"

        /** 初回1回 + リトライ3回 = 最大4リクエスト。 */
        const val MAX_RETRIES = 3
        const val RETRY_INTERVAL_MILLIS = 1000L
    }
}
```

`store.writeSession` が失敗した場合でも `deviceSecret` は保存済みである。
セッションは次の周期で `RefreshSessionUseCase` が取り直せるため、
ペアリングそのものをやり直す必要はない。ただし画面には `Unavailable` を出し、
「繋がっている」と誤解させない。

- [ ] **Step 2: `RefreshSessionUseCase.kt` を書く**

**資格情報を消してよいのは `CredentialsRevoked` を受け取ったときだけ。**

```kotlin
package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * セッションを取り直す。端末の自己修復の唯一の入口。
 *
 * 401 と 403 の両方がここへ来る。端末ルートでセッションが失効したとき
 * オーソライザが返すのは 403 であり、401 は Authorization ヘッダが無い場合。
 * 401 だけを見ていると、セッション失効から永久に回復できない。
 */
class RefreshSessionUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
) {

    sealed interface Outcome {
        data class Refreshed(val session: Session) : Outcome

        /** deviceSecret が無効。Web から切断されたか、端末が削除された。 */
        data object Revoked : Outcome

        /** 通信できなかった / サーバ側の障害。**何も消していない。** */
        data object Unavailable : Outcome
    }

    suspend operator fun invoke(): Outcome {
        val credentials = store.readCredentials() ?: return Outcome.Revoked

        return when (val result = api.issueSession(credentials)) {
            is ApiResult.Success -> {
                store.writeSession(result.value)
                Outcome.Refreshed(result.value)
            }

            is ApiResult.Failure -> when (result.failure) {
                // POST /device/token の 401 だけが「無効」を断定できる唯一の応答。
                ApiFailure.CredentialsRevoked -> {
                    store.clear()
                    buffer.removeAll()
                    Outcome.Revoked
                }

                // 5xx・タイムアウト・通信断では絶対に消さない。取り違えると、
                // 圏外になっただけの端末が deviceSecret を捨て、復帰に Web からの
                // QR 再発行と現地への物理的な訪問が必要になる。
                else -> Outcome.Unavailable
            }
        }
    }
}
```

- [ ] **Step 3: `LogoutDeviceUseCase.kt` を書く**

```kotlin
package com.edgewatcher.usecase

import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * 端末自身の資格情報を失効させる。
 *
 * 対象は認可された端末自身であり、本文で他の端末を指名することはできない。
 * 呼び出し側は確認ダイアログを挟むこと。復帰には Web からの QR 再発行と
 * 端末への物理的な操作が要るため、Web のログアウトとは取り返しやすさが違う。
 */
class LogoutDeviceUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
) {

    /**
     * @return サーバ側の失効に成功したか。false でもローカルは必ず消す。
     *   ユーザーが押したログアウトを、通信の都合で無かったことにはしない。
     */
    suspend operator fun invoke(): Boolean {
        val token = store.readSession()?.token
        val acknowledged = if (token == null) {
            false
        } else {
            api.logout(token) is ApiResult.Success
        }

        store.clear()
        buffer.removeAll()
        return acknowledged
    }
}
```

- [ ] **Step 4: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 5: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/usecase/
git commit -m "feat(android): ペアリング・セッション再取得・ログアウトを実装する"
```

---

## Task 8: 観測パイプラインのユースケース

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/usecase/CaptureObservationUseCase.kt`
- Create: `android/app/src/main/java/com/edgewatcher/usecase/UploadNextObservationUseCase.kt`

**Interfaces:**
- Consumes: `Clock` `IdGenerator` `CameraGateway` `JpegEncoder` `LocationGateway`
  `ObservationBuffer` `ObservationApi` `CredentialStore`（Task 3）、
  `ImagePolicy` `BufferPolicy` `UploadOrder`（Task 4〜6）、
  `RefreshSessionUseCase`（Task 7）
- Produces:
  - `class CaptureObservationUseCase(clock, ids, camera, encoder, location, buffer)`
    - `suspend operator fun invoke(): String?` — 採番した observationId。撮影に失敗したら null
  - `class UploadNextObservationUseCase(api, store, buffer, clock, refreshSession)`
    - `suspend operator fun invoke(): UploadNextObservationUseCase.Outcome`
    - `Outcome`: `Sent(observationId: String, pendingCount: Int)` / `Empty` / `Deferred` / `Revoked`

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `CaptureObservationUseCase.kt` を書く**

```kotlin
package com.edgewatcher.usecase

import com.edgewatcher.domain.BufferPolicy
import com.edgewatcher.domain.ImagePolicy
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.port.CameraGateway
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.IdGenerator
import com.edgewatcher.domain.port.JpegEncoder
import com.edgewatcher.domain.port.LocationGateway
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * 撮影1周期。撮って、符号化して、位置を付けて、バッファへ積む。
 *
 * **送信はしない。** 送信は UploadNextObservationUseCase の仕事であり、
 * 分けておくことで「送信に失敗しても撮影は止まらない」が構造的に保たれる。
 */
class CaptureObservationUseCase(
    private val clock: Clock,
    private val ids: IdGenerator,
    private val camera: CameraGateway,
    private val encoder: JpegEncoder,
    private val location: LocationGateway,
    private val buffer: ObservationBuffer,
) {

    /** @return 採番した observationId。撮影できなければ null。 */
    suspend operator fun invoke(): String? {
        val capturedAt = clock.now()

        // 採番時刻は capturedAt に合わせる。サーバは日別クエリの範囲を ULID の
        // タイムスタンプ部から導くため、ここがずれると観測が別の日に並ぶ。
        val observationId = ids.ulid(capturedAt)

        val raw = runCatching { camera.capture() }.getOrNull() ?: return null

        val thumbnail = encoder.encode(
            raw,
            ImagePolicy.THUMBNAIL.longestSide,
            ImagePolicy.THUMBNAIL.quality,
        )

        // 段階を順に試し、合計が上限に収まった時点で採用する。送っても必ず 400 に
        // なるフレームをバッファに積まないため。通常は第1段階で収まる。
        var image: ByteArray? = null
        for (step in ImagePolicy.MAIN_STEPS) {
            val encoded = encoder.encode(raw, step.longestSide, step.quality)
            if (ImagePolicy.fits(encoded.size, thumbnail.size)) {
                image = encoded
                break
            }
            image = encoded
        }
        val accepted = image ?: return null
        if (!ImagePolicy.fits(accepted.size, thumbnail.size)) return null

        // 測位は待たない。定点観測デバイスは動かないので、最後の既知位置で十分。
        val coordinates = runCatching { location.lastKnown() }.getOrNull()

        val observation = PendingObservation(
            observationId = observationId,
            capturedAt = capturedAt,
            coordinates = coordinates,
            totalBytes = (accepted.size + thumbnail.size).toLong(),
        )
        buffer.save(observation, accepted, thumbnail)

        evict()
        return observationId
    }

    /** 撮影のたびに上限を評価し、超えていれば古い順に落とす。 */
    private suspend fun evict() {
        BufferPolicy.evictions(buffer.allOldestFirst()).forEach { buffer.remove(it) }
    }
}
```

- [ ] **Step 2: `UploadNextObservationUseCase.kt` を書く**

```kotlin
package com.edgewatcher.usecase

import com.edgewatcher.domain.UploadOrder
import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer

/**
 * バッファの先頭を1件送る。
 *
 * 呼び出し側はこれを Outcome が Empty か Deferred になるまで繰り返す。
 * 1件ずつなのは、途中で撮影が割り込んでも順序の判断が常に最新のバッファに
 * 基づくようにするため。
 */
class UploadNextObservationUseCase(
    private val api: ObservationApi,
    private val store: CredentialStore,
    private val buffer: ObservationBuffer,
    private val clock: Clock,
    private val refreshSession: RefreshSessionUseCase,
) {

    sealed interface Outcome {
        data class Sent(val observationId: String, val pendingCount: Int) : Outcome

        /** 送るものが無い。 */
        data object Empty : Outcome

        /** 今は送れない。バックオフして後で来ること。**何も失っていない。** */
        data object Deferred : Outcome

        /** deviceSecret が無効になった。呼び出し側は未ペアリング画面へ戻す。 */
        data object Revoked : Outcome
    }

    private sealed interface Token {
        data class Ready(val value: String) : Token
        data object Revoked : Token
        data object Unavailable : Token
    }

    suspend operator fun invoke(): Outcome {
        val next = UploadOrder.order(buffer.allOldestFirst()).firstOrNull() ?: return Outcome.Empty

        val token = when (val t = ensureFreshSession()) {
            is Token.Ready -> t.value
            Token.Revoked -> return Outcome.Revoked
            Token.Unavailable -> return Outcome.Deferred
        }

        val image = buffer.readImage(next.observationId)
        val thumbnail = buffer.readThumbnail(next.observationId)
        if (image == null || thumbnail == null) {
            // 行はあるがファイルが無い。対で扱う約束が破れているので、行ごと捨てる。
            buffer.remove(next.observationId)
            return Outcome.Deferred
        }

        return when (val result = api.upload(token, next, image, thumbnail)) {
            is ApiResult.Success -> {
                // nextConfig は必ず適用する。サーバは設定を push しないため、
                // 送信間隔の変更がオーナーから端末へ届く経路はこれ1本しかない。
                store.writeInterval(result.value)
                buffer.remove(next.observationId)
                Outcome.Sent(next.observationId, buffer.count())
            }

            is ApiResult.Failure -> when (result.failure) {
                // 400 / 413。再送しても永久に通らないので、そのフレームを捨てて次へ進む。
                ApiFailure.Discard -> {
                    buffer.remove(next.observationId)
                    Outcome.Deferred
                }

                // 401 / 403。セッションを取り直し、次の周回で同じ行を再送する。
                ApiFailure.AuthExpired -> when (refreshSession()) {
                    is RefreshSessionUseCase.Outcome.Refreshed -> Outcome.Deferred
                    RefreshSessionUseCase.Outcome.Revoked -> Outcome.Revoked
                    RefreshSessionUseCase.Outcome.Unavailable -> Outcome.Deferred
                }

                else -> Outcome.Deferred
            }
        }
    }

    /**
     * 有効なセッションを返す。期限が近ければ先に取り直す。
     * 12時間ごとに1往復が無駄になるのを避けるためで、必須ではないが安い。
     */
    private suspend fun ensureFreshSession(): Token {
        val current = store.readSession()
        val nowSeconds = clock.now().epochSecond
        if (current != null && current.expiresAtEpochSeconds - nowSeconds > REFRESH_MARGIN_SECONDS) {
            return Token.Ready(current.token)
        }
        return when (val outcome = refreshSession()) {
            is RefreshSessionUseCase.Outcome.Refreshed -> Token.Ready(outcome.session.token)
            RefreshSessionUseCase.Outcome.Revoked -> Token.Revoked
            RefreshSessionUseCase.Outcome.Unavailable -> Token.Unavailable
        }
    }

    private companion object {
        const val REFRESH_MARGIN_SECONDS = 300L
    }
}
```

- [ ] **Step 3: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/usecase/
git commit -m "feat(android): 撮影1周期と送信1件のユースケースを実装する"
```

---

## Task 9: API クライアント

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/api/EdgeWatcherService.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/api/Dtos.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/api/RetrofitObservationApi.kt`

**Interfaces:**
- Consumes: `ObservationApi` `ApiResult` `ApiFailures` `DeviceCredentials` `Session`
  `DeviceInfo` `IntervalMinutes` `PendingObservation`（Task 3）
- Produces:
  - `interface EdgeWatcherService`（Retrofit）
  - `class RetrofitObservationApi(service: EdgeWatcherService) : ObservationApi`

**契約（`docs/engineering/api.yml` 端末群4本）:**

| メソッド | パス | 認可 |
| --- | --- | --- |
| POST | `device/pair` | なし |
| POST | `device/token` | なし |
| POST | `device/uploads` | `Authorization: <素のトークン>` |
| POST | `device/logout` | `Authorization: <素のトークン>` |

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `Dtos.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class PairRequest(
    val pairingCode: String,
    val deviceInfo: DeviceInfoDto,
)

@Serializable
data class DeviceInfoDto(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

@Serializable
data class PairResponse(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
data class TokenRequest(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
data class TokenResponse(
    val sessionToken: String,
    val expiresAt: Long,
)

/** multipart の metadata パート。**deviceId は入れない。** 入れてもサーバは読まない。 */
@Serializable
data class UploadMetadataDto(
    val observationId: String,
    val capturedAt: String,
    val lat: Double? = null,
    val lng: Double? = null,
)

@Serializable
data class UploadResponse(
    val nextConfig: NextConfigDto,
)

@Serializable
data class NextConfigDto(
    val intervalMinutes: Int,
)

/**
 * アプリケーション層のエラー本文。
 *
 * オーソライザと API Gateway が返す 401 / 403 / 413 はこの形ではなく
 * `{"message":"..."}` である。**したがって、これのパースに失敗しても
 * 分類が決まらなければならない。**
 */
@Serializable
data class ErrorEnvelope(
    @SerialName("error") val error: ErrorBody,
) {
    @Serializable
    data class ErrorBody(val code: String, val message: String)
}
```

- [ ] **Step 2: `EdgeWatcherService.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.api

import okhttp3.MultipartBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.Header
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.Part

/**
 * 端末群4本。
 *
 * 応答を Response<T> で受けるのは、**status を自分で読む必要があるため**。
 * Retrofit に例外へ変換させると、401 と 403 と 413 の区別が失われる。
 */
interface EdgeWatcherService {

    @POST("device/pair")
    suspend fun pair(@Body body: PairRequest): Response<PairResponse>

    @POST("device/token")
    suspend fun token(@Body body: TokenRequest): Response<TokenResponse>

    /**
     * パート名は image / thumbnail / metadata。契約が縛っている箇所なので変えない。
     * Authorization は **Bearer を付けない素のトークン**。
     */
    @Multipart
    @POST("device/uploads")
    suspend fun upload(
        @Header("Authorization") sessionToken: String,
        @Part image: MultipartBody.Part,
        @Part thumbnail: MultipartBody.Part,
        @Part metadata: MultipartBody.Part,
    ): Response<UploadResponse>

    @POST("device/logout")
    suspend fun logout(@Header("Authorization") sessionToken: String): Response<Unit>
}
```

- [ ] **Step 3: `RetrofitObservationApi.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.api

import com.edgewatcher.domain.error.ApiFailures
import com.edgewatcher.domain.error.ApiResult
import com.edgewatcher.domain.model.DeviceCredentials
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.domain.model.IntervalMinutes
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.model.Session
import com.edgewatcher.domain.port.ObservationApi
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.Response
import java.io.IOException
import java.time.format.DateTimeFormatter

/**
 * ObservationApi の実物。**HTTP のステータスコードを domain の外へ出さない。**
 *
 * 分類は ApiFailures が行い、ここは呼び出しと本文の組み立てだけを担う。
 */
class RetrofitObservationApi(
    private val service: EdgeWatcherService,
    private val json: Json,
) : ObservationApi {

    override suspend fun pair(
        pairingCode: String,
        deviceInfo: DeviceInfo,
    ): ApiResult<DeviceCredentials> = try {
        val response = service.pair(
            PairRequest(
                pairingCode = pairingCode,
                deviceInfo = DeviceInfoDto(
                    model = deviceInfo.model,
                    osVersion = deviceInfo.osVersion,
                    appVersion = deviceInfo.appVersion,
                ),
            ),
        )
        val body = response.body()
        if (response.isSuccessful && body != null) {
            ApiResult.Success(DeviceCredentials(body.deviceId, body.deviceSecret))
        } else {
            ApiResult.Failure(ApiFailures.forPairing(response.code(), errorCodeOf(response)))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun issueSession(
        credentials: DeviceCredentials,
    ): ApiResult<Session> = try {
        val response = service.token(
            TokenRequest(credentials.deviceId, credentials.deviceSecret),
        )
        val body = response.body()
        if (response.isSuccessful && body != null) {
            ApiResult.Success(Session(body.sessionToken, body.expiresAt))
        } else {
            ApiResult.Failure(ApiFailures.forTokenRefresh(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun upload(
        sessionToken: String,
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ): ApiResult<IntervalMinutes> = try {
        val metadata = UploadMetadataDto(
            observationId = observation.observationId,
            capturedAt = ISO_8601.format(observation.capturedAt.atZone(java.time.ZoneOffset.UTC)),
            lat = observation.coordinates?.lat,
            lng = observation.coordinates?.lng,
        )

        val response = service.upload(
            sessionToken = sessionToken,
            image = jpegPart("image", observation.observationId + ".jpg", image),
            thumbnail = jpegPart("thumbnail", observation.observationId + "_thumb.jpg", thumbnail),
            metadata = MultipartBody.Part.createFormData(
                "metadata",
                null,
                json.encodeToString(UploadMetadataDto.serializer(), metadata)
                    .toRequestBody(JSON_MEDIA_TYPE),
            ),
        )

        val body = response.body()
        if (response.isSuccessful && body != null) {
            val interval = IntervalMinutes.fromMinutes(body.nextConfig.intervalMinutes)
                ?: IntervalMinutes.DEFAULT
            ApiResult.Success(interval)
        } else {
            ApiResult.Failure(ApiFailures.forUpload(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    override suspend fun logout(sessionToken: String): ApiResult<Unit> = try {
        val response = service.logout(sessionToken)
        if (response.isSuccessful) {
            ApiResult.Success(Unit)
        } else {
            ApiResult.Failure(ApiFailures.forUpload(response.code()))
        }
    } catch (e: IOException) {
        ApiResult.Failure(ApiFailures.forNetworkError())
    }

    private fun jpegPart(name: String, filename: String, bytes: ByteArray): MultipartBody.Part =
        MultipartBody.Part.createFormData(name, filename, bytes.toRequestBody(JPEG_MEDIA_TYPE))

    /**
     * 本文から code を読む。**読めなくてもよい。**
     * 401 / 403 / 413 は本文の形が違い、パースに失敗する。分類は status で決まる。
     */
    private fun errorCodeOf(response: Response<*>): String? = runCatching {
        val raw = response.errorBody()?.string() ?: return null
        json.decodeFromString(ErrorEnvelope.serializer(), raw).error.code
    }.getOrNull()

    private companion object {
        val JPEG_MEDIA_TYPE = "image/jpeg".toMediaType()
        val JSON_MEDIA_TYPE = "application/json".toMediaType()
        val ISO_8601: DateTimeFormatter = DateTimeFormatter.ISO_OFFSET_DATE_TIME
    }
}
```

- [ ] **Step 4: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 5: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/api/
git commit -m "feat(android): 端末群4本の API クライアントを実装する"
```

---

## Task 10: 資格情報の保管と時刻・採番のアダプタ

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/store/EncryptedCredentialStore.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/SystemClock.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/UlidIdGenerator.kt`

**Interfaces:**
- Consumes: `CredentialStore` `Clock` `IdGenerator`（Task 3）、`UlidGenerator`（Task 2）
- Produces:
  - `class EncryptedCredentialStore(context: Context) : CredentialStore`
  - `class SystemClock : Clock`
  - `class UlidIdGenerator : IdGenerator`

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `EncryptedCredentialStore.kt` を書く**

```kotlin
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
```

- [ ] **Step 2: `SystemClock.kt` と `UlidIdGenerator.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure

import com.edgewatcher.domain.port.Clock
import java.time.Instant

class SystemClock : Clock {
    override fun now(): Instant = Instant.now()
}
```

```kotlin
package com.edgewatcher.infrastructure

import com.edgewatcher.domain.UlidGenerator
import com.edgewatcher.domain.port.IdGenerator
import java.time.Instant

class UlidIdGenerator : IdGenerator {
    override fun ulid(at: Instant): String = UlidGenerator.generate(at)
}
```

- [ ] **Step 3: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/
git commit -m "feat(android): 資格情報の保管と時刻・採番のアダプタを実装する"
```

---

## Task 11: バッファの永続化（Room + ファイル）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/db/ObservationEntity.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/db/ObservationDao.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/db/ObservationDatabase.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/db/RoomObservationBuffer.kt`

**Interfaces:**
- Consumes: `ObservationBuffer` `PendingObservation` `Coordinates`（Task 3）
- Produces: `class RoomObservationBuffer(dao: ObservationDao, filesDir: File) : ObservationBuffer`

**Note:** テストは書かない。**スキーマは v1 のみ**とし、変更時は
`fallbackToDestructiveMigration` を使う。バッファを失っても観測が数枚欠けるだけであり、
マイグレーションを書いて維持する費用に見合わない。

- [ ] **Step 1: `ObservationEntity.kt` と `ObservationDao.kt` を書く**

```kotlin
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
```

```kotlin
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
```

- [ ] **Step 2: `ObservationDatabase.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.db

import androidx.room.Database
import androidx.room.RoomDatabase

/** スキーマは v1 のみ。変更時は fallbackToDestructiveMigration で作り直す。 */
@Database(entities = [ObservationEntity::class], version = 1, exportSchema = false)
abstract class ObservationDatabase : RoomDatabase() {
    abstract fun observations(): ObservationDao
}
```

- [ ] **Step 3: `RoomObservationBuffer.kt` を書く**

**行と画像ファイルを対で扱う。** 片方だけが残る状態を外から作れないようにする。

```kotlin
package com.edgewatcher.infrastructure.db

import com.edgewatcher.domain.model.Coordinates
import com.edgewatcher.domain.model.PendingObservation
import com.edgewatcher.domain.port.ObservationBuffer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.time.Instant

/**
 * バッファの実物。Room の行とアプリ専用ストレージのファイルを対で扱う。
 *
 * **書き込みはファイルが先、行が後。** 逆にすると、行はあるがファイルが無い
 * 状態がプロセス死で生まれ、送信側がその行で詰まる。
 * **削除は行が先、ファイルが後。** 行が消えていればもう誰も参照しない。
 */
class RoomObservationBuffer(
    private val dao: ObservationDao,
    filesDir: File,
) : ObservationBuffer {

    private val root = File(filesDir, "observations").apply { mkdirs() }

    private fun imageFile(id: String) = File(root, "$id.jpg")
    private fun thumbnailFile(id: String) = File(root, "${id}_thumb.jpg")

    override suspend fun save(
        observation: PendingObservation,
        image: ByteArray,
        thumbnail: ByteArray,
    ) = withContext(Dispatchers.IO) {
        imageFile(observation.observationId).writeBytes(image)
        thumbnailFile(observation.observationId).writeBytes(thumbnail)
        dao.insert(
            ObservationEntity(
                observationId = observation.observationId,
                capturedAtMillis = observation.capturedAt.toEpochMilli(),
                lat = observation.coordinates?.lat,
                lng = observation.coordinates?.lng,
                totalBytes = observation.totalBytes,
            ),
        )
    }

    override suspend fun readImage(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) { imageFile(observationId).takeIf { it.exists() }?.readBytes() }

    override suspend fun readThumbnail(observationId: String): ByteArray? =
        withContext(Dispatchers.IO) {
            thumbnailFile(observationId).takeIf { it.exists() }?.readBytes()
        }

    override suspend fun remove(observationId: String) = withContext(Dispatchers.IO) {
        dao.delete(observationId)
        imageFile(observationId).delete()
        thumbnailFile(observationId).delete()
        Unit
    }

    override suspend fun removeAll() = withContext(Dispatchers.IO) {
        dao.deleteAll()
        root.listFiles()?.forEach { it.delete() }
        Unit
    }

    override suspend fun count(): Int = dao.count()

    override suspend fun totalBytes(): Long = dao.totalBytes()

    override suspend fun allOldestFirst(): List<PendingObservation> =
        dao.allOldestFirst().map { row ->
            PendingObservation(
                observationId = row.observationId,
                capturedAt = Instant.ofEpochMilli(row.capturedAtMillis),
                coordinates = if (row.lat != null && row.lng != null) {
                    Coordinates(row.lat, row.lng)
                } else {
                    null
                },
                totalBytes = row.totalBytes,
            )
        }
}
```

- [ ] **Step 4: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 5: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/db/
git commit -m "feat(android): バッファの永続化を Room とファイルで実装する"
```

---

## Task 12: カメラと符号化

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/camera/CameraXGateway.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/camera/AndroidJpegEncoder.kt`

**Interfaces:**
- Consumes: `CameraGateway` `JpegEncoder`（Task 3）
- Produces:
  - `class CameraXGateway(context: Context) : CameraGateway`
    - `suspend fun bindTo(owner: LifecycleOwner)` — Service が起動時に1度呼ぶ
    - `fun attachPreview(surfaceProvider: Preview.SurfaceProvider)`
    - `fun detachPreview()`
    - `fun release()`
  - `class AndroidJpegEncoder : JpegEncoder`

**カメラの所有権はここに一本化される。** 画面は `attachPreview` で Surface を渡すだけで、
`ImageCapture` は常に bind されたままなので、プレビューの ON / OFF で定期撮影が止まらない。

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `AndroidJpegEncoder.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.camera

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.edgewatcher.domain.port.JpegEncoder
import java.io.ByteArrayOutputStream
import kotlin.math.max
import kotlin.math.roundToInt

/**
 * 言われた寸法で符号化するだけ。**何段階で落とすかは ImagePolicy が決める。**
 *
 * inSampleSize で先に間引くのは、フル解像度の Bitmap を素で確保すると
 * 安価な端末で OOM になるため。2の冪でしか効かないので、そのあと
 * createScaledBitmap で正確な長辺に合わせる。
 */
class AndroidJpegEncoder : JpegEncoder {

    override fun encode(source: ByteArray, longestSide: Int, quality: Int): ByteArray {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(source, 0, source.size, bounds)

        val sourceLongest = max(bounds.outWidth, bounds.outHeight)
        val options = BitmapFactory.Options().apply {
            inSampleSize = sampleSizeFor(sourceLongest, longestSide)
        }
        val decoded = BitmapFactory.decodeByteArray(source, 0, source.size, options)
            ?: return ByteArray(0)

        val scaled = scaleToLongestSide(decoded, longestSide)
        if (scaled !== decoded) decoded.recycle()

        val out = ByteArrayOutputStream()
        scaled.compress(Bitmap.CompressFormat.JPEG, quality, out)
        scaled.recycle()
        return out.toByteArray()
    }

    private fun sampleSizeFor(sourceLongest: Int, targetLongest: Int): Int {
        var sample = 1
        while (sourceLongest / (sample * 2) >= targetLongest) sample *= 2
        return sample
    }

    private fun scaleToLongestSide(bitmap: Bitmap, longestSide: Int): Bitmap {
        val longest = max(bitmap.width, bitmap.height)
        if (longest <= longestSide) return bitmap
        val ratio = longestSide.toDouble() / longest
        return Bitmap.createScaledBitmap(
            bitmap,
            (bitmap.width * ratio).roundToInt().coerceAtLeast(1),
            (bitmap.height * ratio).roundToInt().coerceAtLeast(1),
            true,
        )
    }
}
```

- [ ] **Step 2: `CameraXGateway.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.camera

import android.content.Context
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import com.edgewatcher.domain.port.CameraGateway
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * カメラの所有権を持つ。**Foreground Service だけがこれを保持する。**
 *
 * Android のカメラは同時に2つのクライアントが開けない。サービスが5分ごとに
 * 撮影する一方で画面が独立してプレビューを開こうとすると競合するため、
 * 所有権をここに一本化し、画面は attachPreview で Surface を渡すだけにする。
 *
 * ImageCapture は常に bind したままにし、Preview だけを付け外しする。
 * これによりプレビューのために定期送信を止める必要がなくなる。
 */
class CameraXGateway(private val context: Context) : CameraGateway {

    private var provider: ProcessCameraProvider? = null
    private var owner: LifecycleOwner? = null
    private var preview: Preview? = null

    private val imageCapture = ImageCapture.Builder()
        .setCaptureMode(ImageCapture.CAPTURE_MODE_MINIMIZE_LATENCY)
        .build()

    /** Service の起動時に1度だけ呼ぶ。 */
    suspend fun bindTo(lifecycleOwner: LifecycleOwner) {
        val cameraProvider = suspendCancellableCoroutine { continuation ->
            val future = ProcessCameraProvider.getInstance(context)
            future.addListener(
                { runCatching { future.get() }.fold(continuation::resume, continuation::resumeWithException) },
                ContextCompat.getMainExecutor(context),
            )
        }
        provider = cameraProvider
        owner = lifecycleOwner
        rebind()
    }

    fun attachPreview(surfaceProvider: Preview.SurfaceProvider) {
        preview = Preview.Builder().build().also { it.surfaceProvider = surfaceProvider }
        rebind()
    }

    fun detachPreview() {
        preview = null
        rebind()
    }

    fun release() {
        provider?.unbindAll()
        provider = null
        owner = null
        preview = null
    }

    /**
     * Preview + ImageCapture は CameraX が LEGACY レベルの端末でも
     * サポートを保証している組み合わせ。余剰端末を前提にする以上、
     * ここに他の use case を足さない。
     */
    private fun rebind() {
        val cameraProvider = provider ?: return
        val lifecycleOwner = owner ?: return
        cameraProvider.unbindAll()
        val useCases = listOfNotNull(imageCapture, preview).toTypedArray()
        cameraProvider.bindToLifecycle(
            lifecycleOwner,
            CameraSelector.DEFAULT_BACK_CAMERA,
            *useCases,
        )
    }

    override suspend fun capture(): ByteArray = suspendCancellableCoroutine { continuation ->
        imageCapture.takePicture(
            ContextCompat.getMainExecutor(context),
            object : ImageCapture.OnImageCapturedCallback() {
                override fun onCaptureSuccess(image: ImageProxy) {
                    val buffer = image.planes[0].buffer
                    val bytes = ByteArray(buffer.remaining())
                    buffer.get(bytes)
                    image.close()
                    continuation.resume(bytes)
                }

                override fun onError(exception: ImageCaptureException) {
                    continuation.resumeWithException(exception)
                }
            },
        )
    }
}
```

- [ ] **Step 3: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/camera/
git commit -m "feat(android): カメラの所有権と JPEG の符号化を実装する"
```

---

## Task 13: 位置情報

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/location/FusedLocationGateway.kt`

**Interfaces:**
- Consumes: `LocationGateway` `Coordinates`（Task 3）
- Produces: `class FusedLocationGateway(context: Context) : LocationGateway`

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `FusedLocationGateway.kt` を書く**

```kotlin
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
```

- [ ] **Step 2: コンパイルが通ることを確認する**

Run: `cd android && ./gradlew :app:compileDebugKotlin`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 3: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/location/
git commit -m "feat(android): 最後の既知位置を測位を待たずに取る"
```

---

## Task 14: アラームと通知

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/service/AndroidAlarmScheduler.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/service/AlarmReceiver.kt`
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/service/Notifications.kt`

**Interfaces:**
- Consumes: `AlarmScheduler`（Task 3）、`StatusLine` `ObservationState`（Task 3・6）
- Produces:
  - `class AndroidAlarmScheduler(context: Context) : AlarmScheduler`
  - `class AlarmReceiver : BroadcastReceiver`
  - `object Notifications`
    - `const val CHANNEL_RUNNING: String` / `const val CHANNEL_RECOVERY: String`
    - `const val ONGOING_ID: Int` / `const val RECOVERY_ID: Int`
    - `fun ensureChannels(context: Context)`
    - `fun ongoing(context: Context, state: ObservationState): Notification`
    - `fun recovery(context: Context): Notification`

**Note:** テストは書かない。完了条件はコンパイルが通ること。

- [ ] **Step 1: `AndroidAlarmScheduler.kt` と `AlarmReceiver.kt` を書く**

```kotlin
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
```

```kotlin
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
```

- [ ] **Step 2: `Notifications.kt` を書く**

```kotlin
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
```

- [ ] **Step 3: コンパイルは Task 15・18 の後に通る**

`ObservationService` と `MainActivity` がまだ無いため、この時点では
`Unresolved reference` が出る。**このタスクではビルドを通さない。**
Task 18 の Step 6 でまとめて確認する。

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/service/
git commit -m "feat(android): アラームと通知を実装する"
```

---

## Task 15: Foreground Service

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/service/ObservationService.kt`

**Interfaces:**
- Consumes: `CaptureObservationUseCase` `UploadNextObservationUseCase`（Task 8）、
  `CameraXGateway`（Task 12）、`AlarmScheduler`（Task 3）、`CredentialStore`（Task 3）、
  `Notifications`（Task 14）、`ObservationState`（Task 3）
- Produces:
  - `class ObservationService : LifecycleService()`
  - `inner class LocalBinder : Binder { val state: StateFlow<ObservationState>; fun attachPreview(...); fun detachPreview() }`
  - `companion object { fun start(context: Context); fun stop(context: Context); fun requestCapture(context: Context); val revoked: SharedFlow<Unit> }`

**Note:** テストは書かない。完了条件は Task 18 でのビルド成功。

- [ ] **Step 1: `ObservationService.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.service

import android.app.Notification
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.camera.core.Preview
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.domain.port.AlarmScheduler
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.ObservationBuffer
import com.edgewatcher.infrastructure.camera.CameraXGateway
import com.edgewatcher.usecase.CaptureObservationUseCase
import com.edgewatcher.usecase.UploadNextObservationUseCase
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import javax.inject.Inject
import kotlin.math.min
import kotlin.math.pow

/**
 * 観測の本体。
 *
 * LifecycleService なのは CameraX が LifecycleOwner を要求するため。
 * これによりカメラの寿命がサービスの寿命と一致し、画面が閉じても観測が続く。
 *
 * タイマーはウェイクロックと AlarmManager の二重化で担保する。Foreground Service は
 * 「プロセスが殺されにくい」ことを保証するだけで「動き続ける」ことは保証しない。
 */
@AndroidEntryPoint
class ObservationService : LifecycleService() {

    @Inject lateinit var capture: CaptureObservationUseCase
    @Inject lateinit var uploadNext: UploadNextObservationUseCase
    @Inject lateinit var camera: CameraXGateway
    @Inject lateinit var alarms: AlarmScheduler
    @Inject lateinit var store: CredentialStore
    @Inject lateinit var clock: Clock
    @Inject lateinit var buffer: ObservationBuffer

    private val _state = MutableStateFlow<ObservationState>(ObservationState.Observing(null))
    private val binder = LocalBinder()
    private val cycle = Mutex()

    private var wakeLock: PowerManager.WakeLock? = null
    private var consecutiveFailures = 0

    /** 直近に送信が成功した時刻。状態行の「最終送信」がこれを出す。 */
    private var lastUploadAt: java.time.Instant? = null

    inner class LocalBinder : Binder() {
        val state: StateFlow<ObservationState> get() = _state.asStateFlow()

        /** プレビュー ON。ImageCapture は bind されたままなので送信は止まらない。 */
        fun attachPreview(surfaceProvider: Preview.SurfaceProvider) =
            camera.attachPreview(surfaceProvider)

        fun detachPreview() = camera.detachPreview()
    }

    override fun onBind(intent: Intent): IBinder {
        super.onBind(intent)
        return binder
    }

    override fun onCreate() {
        super.onCreate()
        Notifications.ensureChannels(this)
        startForegroundWith(Notifications.ongoing(this, _state.value))
        acquireWakeLock()
        lifecycleScope.launch {
            camera.bindTo(this@ObservationService)
            runCycle()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        if (intent?.action == ACTION_CAPTURE) {
            lifecycleScope.launch { runCycle() }
        }
        // プロセスが OS に落とされたら作り直させる。次の撮影時刻への復帰点は
        // AlarmManager が別に持っている。
        return START_STICKY
    }

    override fun onDestroy() {
        alarms.cancel()
        camera.release()
        releaseWakeLock()
        super.onDestroy()
    }

    /**
     * 1周期。撮ってから、送れるだけ送り、次のアラームを置く。
     *
     * Mutex で囲うのは、アラームと手動再開が重なったときに二重に撮らないため。
     */
    private suspend fun runCycle() = cycle.withLock {
        capture()
        drainQueue()
        scheduleNext()
        publishState()
    }

    /**
     * 送れるだけ送る。
     *
     * Deferred はバックオフして同じ周期の中で再試行する。**ここで delay を使えるのは
     * PARTIAL_WAKE_LOCK を保持しているためである**（保持していなければ画面 OFF 中に
     * 発火しない）。次の撮影時刻を追い越さないよう、累計が撮影間隔に達したら諦めて
     * アラームに任せる。
     */
    private suspend fun drainQueue() {
        val intervalSeconds = store.readInterval().minutes * 60L
        var spent = 0L
        while (true) {
            when (uploadNext()) {
                is UploadNextObservationUseCase.Outcome.Sent -> {
                    consecutiveFailures = 0
                    lastUploadAt = clock.now()
                }

                UploadNextObservationUseCase.Outcome.Empty -> {
                    consecutiveFailures = 0
                    return
                }

                UploadNextObservationUseCase.Outcome.Deferred -> {
                    consecutiveFailures += 1
                    val wait = backoffSeconds()
                    if (spent + wait >= intervalSeconds) return
                    spent += wait
                    delay(wait * 1000L)
                }

                UploadNextObservationUseCase.Outcome.Revoked -> {
                    _revoked.tryEmit(Unit)
                    stopSelf()
                    return
                }
            }
        }
    }

    /**
     * 次回の撮影時刻を置く。
     *
     * **撮影の間隔は送信の成否で変わらない。** リトライ中も定期撮影は止めないという
     * 要件があり、撮った画像はバッファに積まれる。送信のバックオフは撮影周期とは
     * 別に drainQueue が持つ（backoffSeconds）。
     */
    private fun scheduleNext() {
        alarms.scheduleNext(clock.now().plusSeconds(store.readInterval().minutes * 60L))
    }

    /** 連続失敗回数 n に対して min(5 * 2^(n-1), 300) 秒。 */
    private fun backoffSeconds(): Long =
        if (consecutiveFailures == 0) {
            0L
        } else {
            min(5.0 * 2.0.pow(consecutiveFailures - 1), 300.0).toLong()
        }

    /**
     * 状態の真実の源はバッファの残数。**ここで送信を試みてはならない。**
     * uploadNext() を呼ぶと drainQueue と合わせて二重に送ることになる。
     */
    private suspend fun publishState() {
        val pending = buffer.count()
        _state.value = if (pending > 0) {
            ObservationState.Offline(pending)
        } else {
            ObservationState.Observing(lastUploadAt)
        }
        startForegroundWith(Notifications.ongoing(this, _state.value))
    }

    private fun startForegroundWith(notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(
                Notifications.ONGOING_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_CAMERA or
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION,
            )
        } else {
            startForeground(Notifications.ONGOING_ID, notification)
        }
    }

    /**
     * 常時給電が前提なので、稼働中は保持し続けてよい。CPU は起きているだけで
     * 実際の処理は5分に一度なので発熱も小さい。バッテリー駆動なら選べない設計。
     */
    private fun acquireWakeLock() {
        val power = getSystemService(PowerManager::class.java)
        wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, WAKE_LOCK_TAG)
            .also { it.acquire() }
    }

    private fun releaseWakeLock() {
        wakeLock?.takeIf { it.isHeld }?.release()
        wakeLock = null
    }

    companion object {
        private const val ACTION_CAPTURE = "com.edgewatcher.action.CAPTURE"
        private const val WAKE_LOCK_TAG = "edgewatcher:observation"

        private val _revoked = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

        /** deviceSecret が無効になったことを画面へ知らせる。 */
        val revoked: SharedFlow<Unit> = _revoked.asSharedFlow()

        fun start(context: Context) {
            context.startForegroundService(Intent(context, ObservationService::class.java))
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, ObservationService::class.java))
        }

        fun requestCapture(context: Context) {
            context.startForegroundService(
                Intent(context, ObservationService::class.java).setAction(ACTION_CAPTURE),
            )
        }
    }
}
```

- [ ] **Step 2: コミット**

ビルドは Task 18 で通す。

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/service/ObservationService.kt
git commit -m "feat(android): 観測の Foreground Service を実装する"
```

---

## Task 16: 再起動後の復帰

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/infrastructure/service/BootReceiver.kt`

**Interfaces:**
- Consumes: `ObservationService`（Task 15）、`Notifications`（Task 14）、`CredentialStore`（Task 3）
- Produces: `class BootReceiver : BroadcastReceiver`

**この分岐は SDK ≤33 側を検証できない**（spec §11.2）。したがって
**分岐は1箇所に閉じ込め、両側の処理を最小に保つ。**

- [ ] **Step 1: `BootReceiver.kt` を書く**

```kotlin
package com.edgewatcher.infrastructure.service

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import com.edgewatcher.domain.port.CredentialStore
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

/**
 * 再起動後の復帰。
 *
 * **Android 14 以降は自動復帰できない。** バックグラウンドから camera タイプの
 * Foreground Service を開始することが禁止されており、BOOT_COMPLETED の受信は
 * バックグラウンド起動に当たる。型を dataSync に変えても解決しない。カメラは
 * 「使用中のみ」の権限であり、camera 型の FGS がその「使用中」を成立させているため。
 *
 * 古い端末ほど完全自動で動くという皮肉な結果になるが、これは余剰端末の活用という
 * コンセプトとは相性が良い。
 *
 * **SDK 33 以下の経路は手元に実機が無く検証できない**（spec §11.2）。
 * 分岐をこの1箇所に閉じ込めてあるのはそのためで、壊れていた場合の影響は
 * 「古い端末で再起動後に自動復帰しない」に留まる。通知経由の手動再開は
 * どのバージョンでも動く。
 */
@AndroidEntryPoint
class BootReceiver : BroadcastReceiver() {

    @Inject lateinit var store: CredentialStore

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return

        // 未ペアリングの端末を起こしても、QR スキャナが立つだけで意味がない。
        if (store.readCredentials() == null) return

        Notifications.ensureChannels(context)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            context.getSystemService(NotificationManager::class.java)
                .notify(Notifications.RECOVERY_ID, Notifications.recovery(context))
        } else {
            ObservationService.start(context)
        }
    }
}
```

- [ ] **Step 2: コミット**

ビルドは Task 18 で通す。

```bash
git add android/app/src/main/java/com/edgewatcher/infrastructure/service/BootReceiver.kt
git commit -m "feat(android): 再起動後の復帰を OS バージョンで分岐させる"
```

---

## Task 17: DI（最外装）

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/di/AppModule.kt`

**Interfaces:**
- Consumes: Task 9〜13 のすべての実装、Task 7〜8 のすべてのユースケース
- Produces: Hilt のグラフ。**`@Module` はこのファイルにしか無い。**

**Note:** テストは書かない。完了条件は Task 18 でのビルド成功。

- [ ] **Step 1: `AppModule.kt` を書く**

```kotlin
package com.edgewatcher.di

import android.content.Context
import androidx.room.Room
import com.edgewatcher.BuildConfig
import com.edgewatcher.domain.port.AlarmScheduler
import com.edgewatcher.domain.port.CameraGateway
import com.edgewatcher.domain.port.Clock
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.domain.port.IdGenerator
import com.edgewatcher.domain.port.JpegEncoder
import com.edgewatcher.domain.port.LocationGateway
import com.edgewatcher.domain.port.ObservationApi
import com.edgewatcher.domain.port.ObservationBuffer
import com.edgewatcher.infrastructure.SystemClock
import com.edgewatcher.infrastructure.UlidIdGenerator
import com.edgewatcher.infrastructure.api.EdgeWatcherService
import com.edgewatcher.infrastructure.api.RetrofitObservationApi
import com.edgewatcher.infrastructure.camera.AndroidJpegEncoder
import com.edgewatcher.infrastructure.camera.CameraXGateway
import com.edgewatcher.infrastructure.db.ObservationDatabase
import com.edgewatcher.infrastructure.db.RoomObservationBuffer
import com.edgewatcher.infrastructure.location.FusedLocationGateway
import com.edgewatcher.infrastructure.service.AndroidAlarmScheduler
import com.edgewatcher.infrastructure.store.EncryptedCredentialStore
import com.edgewatcher.usecase.CaptureObservationUseCase
import com.edgewatcher.usecase.LogoutDeviceUseCase
import com.edgewatcher.usecase.PairDeviceUseCase
import com.edgewatcher.usecase.RefreshSessionUseCase
import com.edgewatcher.usecase.UploadNextObservationUseCase
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.util.concurrent.TimeUnit
import javax.inject.Singleton

/**
 * 依存の組み立ては最外装のここだけで行う。
 *
 * domain と usecase はコンストラクタ注入だけで成立しており、Hilt を知らない。
 * この向きを保っている限り、レイヤの規約は機械的に守られる。
 */
@Module
@InstallIn(SingletonComponent::class)
object AppModule {

    @Provides
    @Singleton
    fun json(): Json = Json { ignoreUnknownKeys = true; encodeDefaults = false }

    @Provides
    @Singleton
    fun okHttp(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS)
        .build()

    @Provides
    @Singleton
    fun service(client: OkHttpClient, json: Json): EdgeWatcherService = Retrofit.Builder()
        // BuildConfig.API_BASE_URL は末尾スラッシュで終わること。Retrofit の要求。
        .baseUrl(BuildConfig.API_BASE_URL.trimEnd('/') + "/")
        .client(client)
        .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
        .build()
        .create(EdgeWatcherService::class.java)

    @Provides
    @Singleton
    fun api(service: EdgeWatcherService, json: Json): ObservationApi =
        RetrofitObservationApi(service, json)

    @Provides
    @Singleton
    fun credentialStore(@ApplicationContext context: Context): CredentialStore =
        EncryptedCredentialStore(context)

    @Provides
    @Singleton
    fun database(@ApplicationContext context: Context): ObservationDatabase =
        Room.databaseBuilder(context, ObservationDatabase::class.java, "observations.db")
            // スキーマは v1 のみ。壊れたら作り直す。バッファを失っても観測が数枚欠けるだけ。
            .fallbackToDestructiveMigration(dropAllTables = true)
            .build()

    @Provides
    @Singleton
    fun buffer(@ApplicationContext context: Context, db: ObservationDatabase): ObservationBuffer =
        RoomObservationBuffer(db.observations(), context.filesDir)

    @Provides
    @Singleton
    fun cameraGateway(@ApplicationContext context: Context): CameraXGateway =
        CameraXGateway(context)

    @Provides
    @Singleton
    fun camera(gateway: CameraXGateway): CameraGateway = gateway

    @Provides
    @Singleton
    fun encoder(): JpegEncoder = AndroidJpegEncoder()

    @Provides
    @Singleton
    fun location(@ApplicationContext context: Context): LocationGateway =
        FusedLocationGateway(context)

    @Provides
    @Singleton
    fun alarms(@ApplicationContext context: Context): AlarmScheduler =
        AndroidAlarmScheduler(context)

    @Provides
    @Singleton
    fun clock(): Clock = SystemClock()

    @Provides
    @Singleton
    fun ids(): IdGenerator = UlidIdGenerator()

    @Provides
    fun pairDevice(api: ObservationApi, store: CredentialStore) =
        PairDeviceUseCase(api, store)

    @Provides
    fun refreshSession(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
    ) = RefreshSessionUseCase(api, store, buffer)

    @Provides
    fun logoutDevice(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
    ) = LogoutDeviceUseCase(api, store, buffer)

    @Provides
    fun captureObservation(
        clock: Clock,
        ids: IdGenerator,
        camera: CameraGateway,
        encoder: JpegEncoder,
        location: LocationGateway,
        buffer: ObservationBuffer,
    ) = CaptureObservationUseCase(clock, ids, camera, encoder, location, buffer)

    @Provides
    fun uploadNext(
        api: ObservationApi,
        store: CredentialStore,
        buffer: ObservationBuffer,
        clock: Clock,
        refreshSession: RefreshSessionUseCase,
    ) = UploadNextObservationUseCase(api, store, buffer, clock, refreshSession)
}
```

- [ ] **Step 2: コミット**

ビルドは Task 18 で通す。

```bash
git add android/app/src/main/java/com/edgewatcher/di/
git commit -m "feat(android): 依存の組み立てを最外装に置く"
```

---

## Task 18: 画面一式とビルドの復帰

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/presentation/MainActivity.kt`
- Create: `android/app/src/main/java/com/edgewatcher/presentation/permission/PermissionScreen.kt`
- Create: `android/app/src/main/java/com/edgewatcher/presentation/pairing/PairingViewModel.kt`
- Create: `android/app/src/main/java/com/edgewatcher/presentation/pairing/PairingScreen.kt`
- Create: `android/app/src/main/java/com/edgewatcher/presentation/running/RunningViewModel.kt`
- Create: `android/app/src/main/java/com/edgewatcher/presentation/running/RunningScreen.kt`
- Modify: `android/app/src/main/AndroidManifest.xml`（Task 1 Step 10 でコメントアウトした4ブロックを戻す）

**Interfaces:**
- Consumes: `PairDeviceUseCase` `LogoutDeviceUseCase`（Task 7）、`ObservationService`（Task 15）、
  `CredentialStore`（Task 3）、`StatusLine` `ObservationState`（Task 3・6）
- Produces: `class MainActivity : ComponentActivity`

**画面は実質2つ。** タブもナビゲーションドロワーも持たない。
未ペアリングなら QR スキャナ、ペアリング済みなら状態1行と `[画角を合わせる]`・`[ログアウト]` のみ。

**Note:** テストは書かない。完了条件は `assembleDebug` が通ること。

- [ ] **Step 1: `PermissionScreen.kt` を書く**

```kotlin
package com.edgewatcher.presentation.permission

import android.Manifest
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

/**
 * QR スキャナに入る前に権限を通す。
 *
 * **縮退モードは持たない。** カメラか位置情報のどちらかが欠けた時点で
 * このアプリの目的が成立しないため、恒久的に拒否された場合は設定への導線だけを出す。
 *
 * ACCESS_BACKGROUND_LOCATION は要求しない。Foreground Service の location タイプが
 * 「使用中」を成立させるため不要であり、要求すると Play の審査で背景位置情報の
 * 正当化が必要になる。
 */
@Composable
fun PermissionScreen(onGranted: () -> Unit) {
    val context = LocalContext.current
    var denied by remember { mutableStateOf(false) }

    val required = remember {
        buildList {
            add(Manifest.permission.CAMERA)
            add(Manifest.permission.ACCESS_FINE_LOCATION)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                add(Manifest.permission.POST_NOTIFICATIONS)
            }
        }.toTypedArray()
    }

    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { result ->
        val essential = result[Manifest.permission.CAMERA] == true &&
            result[Manifest.permission.ACCESS_FINE_LOCATION] == true
        if (essential) onGranted() else denied = true
    }

    LaunchedEffect(Unit) { launcher.launch(required) }

    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (denied) {
            Text("カメラと位置情報の権限がないと観測できません。")
            Button(
                onClick = {
                    context.startActivity(
                        Intent(
                            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                            Uri.fromParts("package", context.packageName, null),
                        ),
                    )
                },
                modifier = Modifier.padding(top = 16.dp),
            ) { Text("設定を開く") }
        } else {
            Text("権限を確認しています...")
        }
    }
}
```

- [ ] **Step 2: `PairingViewModel.kt` と `PairingScreen.kt` を書く**

```kotlin
package com.edgewatcher.presentation.pairing

import android.os.Build
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgewatcher.BuildConfig
import com.edgewatcher.domain.error.ApiFailure
import com.edgewatcher.domain.model.DeviceInfo
import com.edgewatcher.usecase.PairDeviceUseCase
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class PairingViewModel @Inject constructor(
    private val pairDevice: PairDeviceUseCase,
) : ViewModel() {

    /** 画面に出す1行。null なら何も出さない。 */
    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    private val _paired = MutableStateFlow(false)
    val paired: StateFlow<Boolean> = _paired.asStateFlow()

    private var busy = false

    /** 未ペアリング画面へ戻された理由。黙って戻すと端末の故障と区別がつかない。 */
    fun showRevokedReason() {
        _message.value = "この端末は接続を解除されました。再度ペアリングしてください。"
    }

    fun onQrScanned(payload: String) {
        if (busy || _paired.value) return
        busy = true
        viewModelScope.launch {
            val outcome = pairDevice(payload, deviceInfo())
            when (outcome) {
                PairDeviceUseCase.Outcome.Paired -> {
                    _message.value = null
                    _paired.value = true
                }

                // 他のアプリの QR。何も出さずに読み取りを続ける。
                PairDeviceUseCase.Outcome.NotEdgeWatcherQr -> Unit

                is PairDeviceUseCase.Outcome.Rejected -> _message.value = when (outcome.reason) {
                    ApiFailure.PairingRejected.Reason.CONSUMED -> "この QR は使えません。"
                    ApiFailure.PairingRejected.Reason.EXPIRED -> "この QR の有効期限が切れています。"
                    ApiFailure.PairingRejected.Reason.INVALID -> "この QR は読み取れませんでした。"
                }

                PairDeviceUseCase.Outcome.Unavailable ->
                    _message.value = "接続できませんでした。通信状況を確認してもう一度お試しください。"
            }
            busy = false
        }
    }

    private fun deviceInfo() = DeviceInfo(
        model = Build.MODEL,
        osVersion = Build.VERSION.RELEASE,
        appVersion = BuildConfig.APP_VERSION,
    )
}
```

```kotlin
package com.edgewatcher.presentation.pairing

import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage

/**
 * 未ペアリング画面。QR スキャナが全画面。設定項目も履歴も持たない。
 *
 * ここで使うカメラは ObservationService のものではない。未ペアリングの間は
 * Service が動いていないので競合しない。ペアリングが成立したらこの画面は消え、
 * カメラの所有権は Service へ移る。
 */
@Composable
fun PairingScreen(
    onPaired: () -> Unit,
    /** Web から切断されて戻された直後なら true。理由を画面に残す。 */
    showRevokedReason: Boolean = false,
    viewModel: PairingViewModel = hiltViewModel(),
) {
    val message by viewModel.message.collectAsState()
    val paired by viewModel.paired.collectAsState()
    val lifecycleOwner = LocalLifecycleOwner.current

    LaunchedEffect(showRevokedReason) {
        if (showRevokedReason) viewModel.showRevokedReason()
    }

    LaunchedEffect(paired) {
        if (paired) onPaired()
    }

    Box(modifier = Modifier.fillMaxSize()) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
                val previewView = PreviewView(ctx)
                val scanner = BarcodeScanning.getClient()
                val future = ProcessCameraProvider.getInstance(ctx)

                future.addListener({
                    val provider = future.get()
                    val preview = Preview.Builder().build()
                        .also { it.surfaceProvider = previewView.surfaceProvider }

                    val analysis = ImageAnalysis.Builder()
                        .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                        .build()

                    analysis.setAnalyzer(ContextCompat.getMainExecutor(ctx)) { proxy ->
                        val media = proxy.image
                        if (media == null) {
                            proxy.close()
                        } else {
                            val input = InputImage.fromMediaImage(
                                media,
                                proxy.imageInfo.rotationDegrees,
                            )
                            scanner.process(input)
                                .addOnSuccessListener { codes ->
                                    codes.firstOrNull { it.format == Barcode.FORMAT_QR_CODE }
                                        ?.rawValue
                                        ?.let(viewModel::onQrScanned)
                                }
                                .addOnCompleteListener { proxy.close() }
                        }
                    }

                    provider.unbindAll()
                    provider.bindToLifecycle(
                        lifecycleOwner,
                        CameraSelector.DEFAULT_BACK_CAMERA,
                        preview,
                        analysis,
                    )
                }, ContextCompat.getMainExecutor(ctx))

                previewView
            },
        )

        message?.let {
            Text(
                text = it,
                modifier = Modifier.align(Alignment.BottomCenter).padding(24.dp),
            )
        }
    }
}
```

- [ ] **Step 3: `RunningViewModel.kt` と `RunningScreen.kt` を書く**

```kotlin
package com.edgewatcher.presentation.running

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.edgewatcher.usecase.LogoutDeviceUseCase
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class RunningViewModel @Inject constructor(
    private val logoutDevice: LogoutDeviceUseCase,
) : ViewModel() {

    fun logout(onDone: () -> Unit) {
        viewModelScope.launch {
            // 戻り値は捨てる。サーバへ届かなくてもローカルは消えており、
            // ユーザーが押したログアウトを通信の都合で無かったことにはしない。
            logoutDevice()
            onDone()
        }
    }
}
```

```kotlin
package com.edgewatcher.presentation.running

import android.content.ComponentName
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.weight
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.hilt.navigation.compose.hiltViewModel
import com.edgewatcher.domain.StatusLine
import com.edgewatcher.domain.model.ObservationState
import com.edgewatcher.infrastructure.service.ObservationService

/**
 * ペアリング済み画面。
 *
 * **通常は画像を一切表示しない。** 観測画像を見る場所は Web に一本化する。
 * [画角を合わせる] を押したときだけライブプレビューに切り替わり、もう一度で戻る。
 * 直近に送信した画像のサムネイルも出さない。ライブプレビューだけが例外なのは、
 * それが閲覧ではなく設置作業のための道具だから。
 *
 * 端末名は出さない。端末は自分の名前を知らない。
 */
@Composable
fun RunningScreen(
    onLoggedOut: () -> Unit,
    viewModel: RunningViewModel = hiltViewModel(),
) {
    val context = LocalContext.current
    var binder by remember { mutableStateOf<ObservationService.LocalBinder?>(null) }
    var previewing by remember { mutableStateOf(false) }
    var confirmingLogout by remember { mutableStateOf(false) }

    // bind は画面が繋がっている間だけの付加。Service の生存は
    // startForegroundService が担うので、BIND_AUTO_CREATE は付けない。
    DisposableEffect(context) {
        val connection = object : ServiceConnection {
            override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
                binder = service as? ObservationService.LocalBinder
            }

            override fun onServiceDisconnected(name: ComponentName?) {
                binder = null
            }
        }
        // フラグ 0。BIND_AUTO_CREATE を付けないのは、bind で Service を生成させない
        // ため。生存は startForegroundService が持ち、bind は覗き窓にすぎない。
        context.bindService(Intent(context, ObservationService::class.java), connection, 0)
        onDispose {
            // unbind の前に必ず外す。付けっぱなしにすると、閉じた画面の Surface を
            // Service が掴み続け、カメラが開いたまま発熱する。
            binder?.detachPreview()
            runCatching { context.unbindService(connection) }
            binder = null
        }
    }

    // binder は接続前 null になる。collectAsState を条件分岐の中で呼ぶと
    // Composable の呼び出し規則を破るため、produceState で包んで無条件に1回だけ呼ぶ。
    val state by produceState<ObservationState>(ObservationState.Stopped, binder) {
        val connected = binder
        if (connected == null) value = ObservationState.Stopped else connected.state.collect { value = it }
    }

    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(StatusLine.render(state))

        if (previewing) {
            AndroidView(
                modifier = Modifier.fillMaxWidth().weight(1f),
                factory = { ctx ->
                    PreviewView(ctx).also { view ->
                        binder?.attachPreview(view.surfaceProvider)
                    }
                },
            )
        }

        Button(
            onClick = {
                previewing = !previewing
                if (!previewing) binder?.detachPreview()
            },
        ) { Text(if (previewing) "プレビューを閉じる" else "画角を合わせる") }

        // 停止中のときだけ出す。Android 14 以降は再起動後に自動復帰できず、
        // これがないとアプリから観測を再開する手段がなくなる。
        if (state is ObservationState.Stopped) {
            Button(onClick = { ObservationService.start(context) }) { Text("観測を再開") }
        }

        TextButton(onClick = { confirmingLogout = true }) { Text("ログアウト") }
    }

    if (confirmingLogout) {
        AlertDialog(
            onDismissRequest = { confirmingLogout = false },
            title = { Text("ログアウトしますか？") },
            // 取り返しのつきやすさが Web と違うことを、ここで正直に伝える。
            text = {
                Text(
                    "再び観測するには、Web で QR を発行し直して、" +
                        "この端末をもう一度操作する必要があります。",
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmingLogout = false
                        binder?.detachPreview()
                        ObservationService.stop(context)
                        viewModel.logout(onLoggedOut)
                    },
                ) { Text("ログアウト") }
            },
            dismissButton = {
                TextButton(onClick = { confirmingLogout = false }) { Text("キャンセル") }
            },
        )
    }
}
```

- [ ] **Step 4: `MainActivity.kt` を書く**

```kotlin
package com.edgewatcher.presentation

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.lifecycle.lifecycleScope
import com.edgewatcher.domain.port.CredentialStore
import com.edgewatcher.infrastructure.service.ObservationService
import com.edgewatcher.presentation.pairing.PairingScreen
import com.edgewatcher.presentation.permission.PermissionScreen
import com.edgewatcher.presentation.running.RunningScreen
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * 単一 Activity。画面の選択はこれだけ。
 *
 * 資格情報を持たない → QR スキャナ / 持つ → 稼働画面。
 * 初回起動時は、未ペアリング画面に入る前に権限を通す。
 */
@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    @Inject lateinit var store: CredentialStore

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            var permitted by remember { mutableStateOf(false) }
            var paired by remember { mutableStateOf(store.readCredentials() != null) }
            var revoked by remember { mutableStateOf(false) }

            // Web から切断されると Service が知らせてくる。画面はそれを受けて
            // 未ペアリングへ戻し、**理由を残す。** 黙って QR スキャナへ戻すと、
            // オーナーには端末の故障と区別がつかない。
            androidx.compose.runtime.LaunchedEffect(Unit) {
                lifecycleScope.launch {
                    ObservationService.revoked.collect {
                        paired = false
                        revoked = true
                    }
                }
            }

            MaterialTheme {
                Surface {
                    when {
                        !permitted -> PermissionScreen(onGranted = { permitted = true })

                        !paired -> PairingScreen(
                            showRevokedReason = revoked,
                            onPaired = {
                                paired = true
                                revoked = false
                                ObservationService.start(this@MainActivity)
                            },
                        )

                        else -> RunningScreen(onLoggedOut = { paired = false })
                    }
                }
            }
        }
    }
}
```

- [ ] **Step 5: マニフェストのコメントを外す**

Task 1 Step 10 でコメントアウトした `<activity>` `<service>` `<receiver>` の
4ブロック（activity 1・service 1・receiver 2）を元に戻す。

- [ ] **Step 6: ビルドとテストが通ることを確認する**

Run: `cd android && ./gradlew :app:assembleDebug :app:testDebugUnitTest`
Expected: `BUILD SUCCESSFUL`。ユニットテスト34件が PASS

**これが Task 14〜17 の完了条件でもある。** ここが緑になるまで先へ進まない。

- [ ] **Step 7: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/presentation/ \
        android/app/src/main/AndroidManifest.xml
git commit -m "feat(android): 権限・ペアリング・稼働の3画面を実装する"
```

---

## Task 19: バッテリー最適化と正確なアラームの案内

**Files:**
- Create: `android/app/src/main/java/com/edgewatcher/presentation/running/SystemSettingsPrompts.kt`
- Modify: `android/app/src/main/java/com/edgewatcher/presentation/running/RunningScreen.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `fun isIgnoringBatteryOptimizations(context: Context): Boolean`
  - `fun requestIgnoreBatteryOptimizations(context: Context)`
  - `fun canScheduleExactAlarms(context: Context): Boolean`
  - `fun requestExactAlarmPermission(context: Context)`

権限とは別枠。**メーカー独自の省電力機構が Foreground Service を停止させることが
実際に多く、これを案内しないと「なぜか止まる」の主要因になる。**

導線は稼働画面に置き、**条件が満たされていない間だけ出す。** ペアリング直後は
稼働画面が即座に開くので「直後に案内する」を満たし、後から設定を変えられる
という要件も同じ仕組みで満たせる。満たされた時点で導線が消えるため、
通常運用では画面が増えない。強制はしない。

- [ ] **Step 1: `SystemSettingsPrompts.kt` を書く**

```kotlin
package com.edgewatcher.presentation.running

import android.app.AlarmManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings

/**
 * 権限ではないが、これが無いと観測が静かに止まる2つの設定。
 *
 * どちらも強制しない。案内するだけで、拒否されても観測自体は続ける
 * （アラームは不正確版へ落ちる）。
 */

fun isIgnoringBatteryOptimizations(context: Context): Boolean =
    context.getSystemService(PowerManager::class.java)
        .isIgnoringBatteryOptimizations(context.packageName)

@Suppress("BatteryLife")
fun requestIgnoreBatteryOptimizations(context: Context) {
    context.startActivity(
        Intent(
            Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
            Uri.fromParts("package", context.packageName, null),
        ).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
    )
}

fun canScheduleExactAlarms(context: Context): Boolean =
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        context.getSystemService(AlarmManager::class.java).canScheduleExactAlarms()
    } else {
        true
    }

fun requestExactAlarmPermission(context: Context) {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return
    context.startActivity(
        Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM)
            .setData(Uri.fromParts("package", context.packageName, null))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
    )
}
```

- [ ] **Step 2: `RunningScreen.kt` に導線を足す**

`TextButton(onClick = { confirmingLogout = true }) { Text("ログアウト") }` の**直前**に
次を挿入する。条件が満たされていれば何も出ないので、通常運用では画面が増えない。

```kotlin
        if (!isIgnoringBatteryOptimizations(context)) {
            TextButton(onClick = { requestIgnoreBatteryOptimizations(context) }) {
                Text("バッテリー最適化の対象から外す")
            }
        }

        if (!canScheduleExactAlarms(context)) {
            TextButton(onClick = { requestExactAlarmPermission(context) }) {
                Text("正確なアラームを許可する")
            }
        }
```

- [ ] **Step 3: ビルドが通ることを確認する**

Run: `cd android && ./gradlew :app:assembleDebug`
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 4: コミット**

```bash
git add android/app/src/main/java/com/edgewatcher/presentation/running/
git commit -m "feat(android): バッテリー最適化と正確なアラームの案内を出す"
```

---

## Task 20: CI（検証のみ）

**Files:**
- Create: `.github/workflows/android-verify.yml`

**Interfaces:**
- Consumes: なし
- Produces: なし

**配布は実機への直接インストール。** 署名鍵の管理、Play への公開、難読化は範囲外
（spec §13）。したがってこのワークフローは**成果物を作らない**。

- [ ] **Step 1: ワークフローを書く**

```yaml
name: android-verify

on:
  pull_request:
    paths:
      - "android/**"
      - ".github/workflows/android-verify.yml"
  push:
    branches: [develop]
    paths:
      - "android/**"
      - ".github/workflows/android-verify.yml"

permissions:
  contents: read

concurrency:
  group: android-verify-${{ github.ref }}
  cancel-in-progress: true

jobs:
  verify:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: android

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "17"

      - uses: gradle/actions/setup-gradle@v4

      # gradle.properties の API_BASE_URL は本物である必要がない。
      # ビルドが通ることだけを確かめており、通信はしない。
      - name: Unit tests
        run: ./gradlew --no-daemon testDebugUnitTest

      - name: Assemble debug
        run: ./gradlew --no-daemon assembleDebug
```

- [ ] **Step 2: コミット**

```bash
git add .github/workflows/android-verify.yml
git commit -m "ci(android): PR と develop で検証を回す"
```

---

## Task 21: 既存ドキュメントの修正

**Files:**
- Modify: `docs/04-native.md`（§1.4、§1.5、§1.9、§5）
- Modify: `docs/engineering/api.yml`（`/device/uploads` の `thumbnail` の説明）
- Modify: `docs/01-openquestion.md`（APP-01、APP-03、APP-04）
- Modify: `docs/mocks/native-mocks.html`（端末名の3箇所）

**Interfaces:**
- Consumes: なし
- Produces: なし

放置すると、次に読む人が誤った実装を書く（spec §12）。

- [ ] **Step 1: `04-native.md` を直す**

- §1.9 の見出し「401 を起点とした自己修復」を「401 と 403 を起点とした自己修復」にし、
  本文の図の「アップロードが 401」を「アップロードが 401 または 403」に変える。
  理由として次を追記する:

  > 端末ルートでは、セッションが失効するとオーソライザが拒否し、API Gateway が
  > **403**(`{"message":"Forbidden"}`)を返す。401 は Authorization ヘッダそのものが
  > 無い場合であり、日常運用で起きるのは 403 の方である。401 だけを見ていると
  > セッション失効から永久に回復できない。なお `POST /device/token` 自体の失敗は
  > すべて 401 であり、この一点では従来の記述と一致する。

- §1.4 の「4. `POST /device/token` でセッショントークンを取得する」の後に
  「以後、アップロードが 401 または 403 を返したらここへ戻る」を足す
- §1.5 の「サムネイル(長辺160px)」を「サムネイル(長辺480px)」に変える
- §5 の未確定事項の表から、画像仕様・カメラ API・位置情報の取得方式・テスト方針の
  4行を削り、決定値を本文に反映する。表に残るのは無し（空になったら §5 ごと削る）

- [ ] **Step 2: `api.yml` を直す**

`/device/uploads` の `requestBody` の `thumbnail` の description:

```yaml
                thumbnail:
                  type: string
                  format: binary
                  description: サムネイル(JPEG、長辺480px)。空パートは 400。
```

変更後、契約が壊れていないことを確認する。

Run: `python3 -c "import openapi_spec_validator,yaml,sys; openapi_spec_validator.validate(yaml.safe_load(open('docs/engineering/api.yml'))); print('ok')"`
Expected: `ok`

- [ ] **Step 3: `01-openquestion.md` を直す**

一覧表の3行のステータスを変え、各節の本文に決定を書き足す。

| ID | 変更 |
| --- | --- |
| APP-01 | `open` → `decided`。**5分。** backend の `internal/webapi/webapi.go` の `defaultInterval = api.IntervalOptions[0]` と一致させる必要がある旨を明記する |
| APP-03 | `pending` → `decided`。本画像 長辺1600px/品質80、サムネイル 長辺480px/品質75。サイズガード（品質80→65→50→長辺1280px/品質50）も書く |
| APP-04 | `pending` → `decided`。CameraX + LifecycleService / `FusedLocationProviderClient.lastLocation` / テストは JVM 4本のみで計装テストと Robolectric は書かない |

APP-03 には、サムネイルを 160px から 480px へ引き上げた理由（Web の
ダッシュボードが端末カードの画像に `latestThumbnailUrl` を使っており、
カード幅 280〜360px に対して 160px では明確にぼやける）を残す。

- [ ] **Step 4: `docs/mocks/native-mocks.html` から端末名を削る**

`POST /device/pair` の応答は `deviceId` と `deviceSecret` だけで端末名を含まず、
他に届く経路もない。**端末は自分の名前を知らないので、モックの表示は実装不能である。**

該当は3箇所。`paired` シナリオの見出しと、常駐通知2種。

Run: `grep -n "端末名\|玄関\|裏口" docs/mocks/native-mocks.html`
で残りが無いことを確認する。

- [ ] **Step 5: コミット**

```bash
git add docs/04-native.md docs/engineering/api.yml docs/01-openquestion.md docs/mocks/native-mocks.html
git commit -m "docs: Android の実装設計に合わせて要件と契約を揃える"
```

---

## Task 22: 実機での受け入れ

**Files:** なし（確認のみ）

**Interfaces:**
- Consumes: Task 1〜21 のすべて
- Produces: 受け入れ結果の報告

**これは個々の実装タスクの完了条件ではなく、統合後にまとめて行う**（spec §0・§11）。
実機を常時給電で設置して確認する。

**前提:** `android/gradle.properties` の `API_BASE_URL_DEV` が dev の API Gateway の
URL になっていること。Task 1 で仮値のままなら、ここで実値に差し替える。

- [ ] **Step 1: 端末へインストールする**

```bash
cd android && ./gradlew installDebug
```

- [ ] **Step 2: 手順1〜5 を確認する（バックエンド不要のものを含む）**

1. QR を読んでペアリングが成立する。QR 発行直後にスキャンし、404 のリトライが
   効いていること（成立まで最大4秒かかりうる）
2. 権限を拒否したときに設定への導線が出る。許可し直すと QR スキャナに入る
3. `[画角を合わせる]` でライブプレビューが出て、もう一度押すと戻る
4. **プレビューの ON / OFF を跨いでも送信が途切れない。**
   通知の「最終送信」が更新され続けること
5. **画面を消して3時間以上放置し、欠測がない。** Web の履歴で確認する

- [ ] **Step 3: 手順6〜10 を確認する（dev バックエンドが必要）**

6. 機内モードにして30分置き、`オフライン / N件待機中` に変わる
7. 機内モードを解除する。**Web のダッシュボードに最新の1枚が先に現れ**、
   そのあと古い分が埋まる
8. 端末を再起動する。復帰要求の通知が出て、タップで観測が再開する
   （Android 14 以上の経路。**13 以下は実機が無く未検証**）
9. Web から端末を切断する。次の送信で未ペアリング画面へ戻り、
   「この端末は接続を解除されました。」が出る
10. ログアウトの確認ダイアログを経て未ペアリング画面へ戻り、
    Web 側の状態が `DISCONNECTED` になる

- [ ] **Step 4: 結果を報告する**

通らなかった手順があれば、**何がどう違ったかをそのまま報告する。**
緑に見せるための調整をしない。

**手順8の Android 13 以下の経路は検証できない**（spec §11.2）。
これは既知であり、未検証のまま出荷すると spec に記録済み。報告でも同じ扱いにする。

---

## 補遺: タスクの依存関係

サブエージェントへ配る順序。同じ行のタスクは並行してよい。

```
Task 1  （骨組み）
  ↓
Task 2, 3            ← domain の土台。3 が他のすべての型を定める
  ↓
Task 4, 5, 6         ← domain の判断。並行可
  ↓
Task 7, 8            ← usecase。8 は 7 の RefreshSessionUseCase に依存
  ↓
Task 9, 10, 11, 12, 13   ← infrastructure。互いに独立。並行可
  ↓
Task 14, 15, 16, 17      ← service と DI。ここまでビルドは通らない
  ↓
Task 18              ← 画面。**ここで初めてビルドが通る**
  ↓
Task 19, 20, 21      ← 仕上げ。並行可
  ↓
Task 22              ← 実機受け入れ
```

**Task 14〜17 はビルドが通らない状態でコミットする。** 相互参照があり、
Task 18 まで揃わないとリンクできないため。この4タスクをレビューする際は、
ビルドの成否ではなくコードの内容で判断すること。
