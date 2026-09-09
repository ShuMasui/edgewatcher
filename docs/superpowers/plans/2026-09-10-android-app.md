# EdgeWatcher Android アプリ 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 余剰 Android 端末を定点観測デバイスとして動かすアプリを完成させる。QR でペアリングし、以後カメラ画像と位置情報を定期送信し続ける。

**Architecture:** `:domain`（純 Kotlin/JVM。Android SDK をクラスパスに持たない）に要件の判断をすべて置き、`:app`（Android）がポートの実物を実装する。カメラの所有権は Foreground Service が一本で持ち、画面は Surface を渡すだけ。タイマーはウェイクロックと `AlarmManager` の二重化で担保する。

**Tech Stack:** Kotlin / Gradle（Kotlin DSL・バージョンカタログ）/ Jetpack Compose / CameraX / ML Kit barcode-scanning（bundled）/ Room / EncryptedSharedPreferences / OkHttp / kotlinx.serialization / Coroutines / JUnit 4 / MockWebServer / Robolectric

**Spec:** `docs/superpowers/specs/2026-09-10-android-app-design.md`

## Global Constraints

- **TDD は絶対条件。** すべての本体コードは、先に書いて失敗を確認したテストがある状態でのみ書く。テスト結果は正直に報告する。緑に見せるためのテスト削除・skip・アサーション緩和は禁止（`~/.claude/CLAUDE.md`）
- **TDD の例外は設定ファイルとドキュメントのみ。** Gradle スクリプト、`AndroidManifest.xml`、バージョンカタログ、`docs/` 配下がこれに当たる
- `minSdk` = **26**、`compileSdk` / `targetSdk` = **36**。プレビュー SDK は使わない
- `applicationId` = **`com.edgewatcher`**。`mock` フレーバーは `applicationIdSuffix = ".mock"`
- 画面の向きは**縦固定**
- 本画像は**長辺 1600px / 品質 80**、サムネイルは**長辺 160px / 品質 70**。いずれも JPEG
- 本画像 + サムネイルの合計は **4,500,000 バイト**以下
- 送信間隔の初期値は **5分**。`nextConfig.intervalMinutes` は 5 / 10 / 15 のいずれか
- バッファ上限は **288件** または **524,288,000 バイト（500MiB）**
- ペアリングの 404 リトライは **初回1回 + リトライ3回 = 最大4リクエスト、間隔1秒**
- 送信のバックオフは連続失敗回数 n に対して **`min(5 * 2^(n-1), 300)` 秒**
- `Authorization` ヘッダは **`Bearer` を付けない素のトークン**
- **資格情報を消してよいのは `POST /device/token` が 401 を返したときだけ。** 5xx・ネットワーク断で消してはならない
- **`ACCESS_BACKGROUND_LOCATION` は要求しない**
- **端末名は画面にも通知にも出さない**（`POST /device/pair` の応答に含まれないため）
- 資格情報をログに出力しない
- 作業ディレクトリは `android/`。Gradle コマンドはすべて `android/` から実行する

---

## ファイル構成

### `android/domain/`（`kotlin("jvm")`。Android SDK 無し）

| ファイル | 責務 |
| --- | --- |
| `src/main/kotlin/com/edgewatcher/domain/Clock.kt` | 現在時刻のポート |
| `src/main/kotlin/com/edgewatcher/domain/Ulid.kt` | 指定時刻をタイムスタンプ部に持つ ULID の採番 |
| `src/main/kotlin/com/edgewatcher/domain/ImagePolicy.kt` | 符号化の段階とサイズガードの判定 |
| `src/main/kotlin/com/edgewatcher/domain/Api.kt` | `EdgeWatcherApi` ポートと結果型（`PairResult` / `TokenResult` / `UploadResult` / `LogoutResult`） |
| `src/main/kotlin/com/edgewatcher/domain/Credentials.kt` | `CredentialStore` ポートと `Credentials` / `Session` |
| `src/main/kotlin/com/edgewatcher/domain/Observations.kt` | `ObservationStore` ポート、`ObservationMeta`、`UploadFrame` |
| `src/main/kotlin/com/edgewatcher/domain/PairingService.kt` | QR ペイロードの解釈と 404 リトライ |
| `src/main/kotlin/com/edgewatcher/domain/SessionManager.kt` | トークンの取得・事前更新・401/403 起点の自己修復 |
| `src/main/kotlin/com/edgewatcher/domain/BufferPolicy.kt` | 288件 / 500MiB の退避判定 |
| `src/main/kotlin/com/edgewatcher/domain/UploadOrder.kt` | 送信対象の選択規則 |
| `src/main/kotlin/com/edgewatcher/domain/Backoff.kt` | 指数バックオフの待ち時間 |
| `src/main/kotlin/com/edgewatcher/domain/AppState.kt` | `AppState` / `RunState` / `UnpairReason` |
| `src/main/kotlin/com/edgewatcher/domain/StatusLine.kt` | 画面と通知が共有する状態1行の文言 |
| `src/main/kotlin/com/edgewatcher/domain/Ports.kt` | `CaptureSource` / `JpegEncoder` / `LocationSource` / `AlarmScheduler` |
| `src/main/kotlin/com/edgewatcher/domain/CaptureCoordinator.kt` | 撮影1周期のオーケストレーション |
| `src/main/kotlin/com/edgewatcher/domain/UploadWorkerLoop.kt` | 送信1回分のオーケストレーションと結果の写像 |

### `android/app/`（`com.android.application`）

| ファイル | 責務 |
| --- | --- |
| `src/main/kotlin/com/edgewatcher/app/net/HttpEdgeWatcherApi.kt` | OkHttp による `EdgeWatcherApi` 実装 |
| `src/main/kotlin/com/edgewatcher/app/net/Dto.kt` | kotlinx.serialization の DTO |
| `src/mock/kotlin/com/edgewatcher/app/net/ApiFactory.kt` | 偽バックエンドを返す |
| `src/live/kotlin/com/edgewatcher/app/net/ApiFactory.kt` | `HttpEdgeWatcherApi` を返す |
| `src/mock/kotlin/com/edgewatcher/app/net/FakeEdgeWatcherApi.kt` | アプリ内偽バックエンド（mock フレーバーにのみ存在） |
| `src/main/kotlin/com/edgewatcher/app/data/ObservationDb.kt` | Room の Entity / DAO / Database |
| `src/main/kotlin/com/edgewatcher/app/data/RoomObservationStore.kt` | `ObservationStore` 実装（行 + ファイル） |
| `src/main/kotlin/com/edgewatcher/app/data/EncryptedCredentialStore.kt` | `CredentialStore` 実装 |
| `src/main/kotlin/com/edgewatcher/app/device/AndroidJpegEncoder.kt` | `JpegEncoder` 実装 |
| `src/main/kotlin/com/edgewatcher/app/device/FusedLocationSource.kt` | `LocationSource` 実装 |
| `src/main/kotlin/com/edgewatcher/app/device/AlarmManagerScheduler.kt` | `AlarmScheduler` 実装 |
| `src/main/kotlin/com/edgewatcher/app/device/CameraXCaptureSource.kt` | `CaptureSource` 実装 |
| `src/main/kotlin/com/edgewatcher/app/service/ObservationService.kt` | Foreground Service。カメラ所有・ウェイクロック・状態の発信元 |
| `src/main/kotlin/com/edgewatcher/app/service/CaptureAlarmReceiver.kt` | アラーム受信 |
| `src/main/kotlin/com/edgewatcher/app/service/BootReceiver.kt` | `BOOT_COMPLETED` の SDK 分岐 |
| `src/main/kotlin/com/edgewatcher/app/service/Notifications.kt` | 2チャンネルと通知の組み立て |
| `src/main/kotlin/com/edgewatcher/app/AppContainer.kt` | 手書き DI |
| `src/main/kotlin/com/edgewatcher/app/ui/PermissionScreen.kt` | 権限ゲート |
| `src/main/kotlin/com/edgewatcher/app/ui/ScannerScreen.kt` | QR スキャナと理由バナー |
| `src/main/kotlin/com/edgewatcher/app/ui/RunningScreen.kt` | 稼働画面 |
| `src/main/kotlin/com/edgewatcher/app/ui/MainActivity.kt` | 画面の切り替え |

---

## フェーズ0: プロジェクトの骨組み

### Task 1: Gradle プロジェクトを作る

**この Task は設定ファイルのみを作る。** `~/.claude/CLAUDE.md` の TDD 例外（設定ファイル）に当たるため、Red から始めない。代わりに「ビルドが通ること」と「`live` フレーバーが環境変数無しで失敗すること」を実行して確認する。

**Files:**
- Create: `android/settings.gradle.kts`
- Create: `android/build.gradle.kts`
- Create: `android/gradle.properties`
- Create: `android/gradle/libs.versions.toml`
- Create: `android/domain/build.gradle.kts`
- Create: `android/app/build.gradle.kts`
- Create: `android/app/src/main/AndroidManifest.xml`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: なし
- Produces: `./gradlew :domain:test` と `./gradlew :app:assembleMockDebug` が実行できる状態

- [ ] **Step 1: Gradle 本体を取得して wrapper を生成する**

`gradle` は PATH に無いため、配布物を一度だけ取得して wrapper を作る。以後は wrapper が自分でバージョンを管理する。

PowerShell で実行:

```powershell
New-Item -ItemType Directory -Force android | Out-Null
$tmp = "$env:TEMP\gradle-dist"
New-Item -ItemType Directory -Force $tmp | Out-Null
Invoke-WebRequest -Uri "https://services.gradle.org/distributions/gradle-8.14-bin.zip" -OutFile "$tmp\gradle.zip"
Expand-Archive -Path "$tmp\gradle.zip" -DestinationPath $tmp -Force
Push-Location android
& "$tmp\gradle-8.14\bin\gradle.bat" wrapper --gradle-version 8.14
Pop-Location
```

期待: `android/gradlew`、`android/gradlew.bat`、`android/gradle/wrapper/gradle-wrapper.jar`、`android/gradle/wrapper/gradle-wrapper.properties` が生成される。

- [ ] **Step 2: バージョンカタログを書く**

`android/gradle/libs.versions.toml`:

```toml
[versions]
agp = "8.13.0"
kotlin = "2.2.0"
ksp = "2.2.0-2.0.2"
coroutines = "1.10.2"
serialization = "1.8.1"
okhttp = "4.12.0"
room = "2.7.1"
composeBom = "2025.06.00"
activityCompose = "1.10.1"
lifecycle = "2.9.1"
camerax = "1.4.2"
mlkitBarcode = "17.3.0"
playLocation = "21.3.0"
securityCrypto = "1.1.0-alpha06"
junit = "4.13.2"
robolectric = "4.14.1"
androidxTestCore = "1.6.1"

[libraries]
kotlinx-coroutines-core = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-core", version.ref = "coroutines" }
kotlinx-coroutines-android = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-android", version.ref = "coroutines" }
kotlinx-coroutines-test = { module = "org.jetbrains.kotlinx:kotlinx-coroutines-test", version.ref = "coroutines" }
kotlinx-serialization-json = { module = "org.jetbrains.kotlinx:kotlinx-serialization-json", version.ref = "serialization" }
okhttp = { module = "com.squareup.okhttp3:okhttp", version.ref = "okhttp" }
okhttp-mockwebserver = { module = "com.squareup.okhttp3:mockwebserver", version.ref = "okhttp" }
room-runtime = { module = "androidx.room:room-runtime", version.ref = "room" }
room-ktx = { module = "androidx.room:room-ktx", version.ref = "room" }
room-compiler = { module = "androidx.room:room-compiler", version.ref = "room" }
compose-bom = { module = "androidx.compose:compose-bom", version.ref = "composeBom" }
compose-material3 = { module = "androidx.compose.material3:material3" }
compose-ui = { module = "androidx.compose.ui:ui" }
compose-ui-tooling-preview = { module = "androidx.compose.ui:ui-tooling-preview" }
compose-ui-tooling = { module = "androidx.compose.ui:ui-tooling" }
compose-ui-test-junit4 = { module = "androidx.compose.ui:ui-test-junit4" }
compose-ui-test-manifest = { module = "androidx.compose.ui:ui-test-manifest" }
activity-compose = { module = "androidx.activity:activity-compose", version.ref = "activityCompose" }
lifecycle-runtime-ktx = { module = "androidx.lifecycle:lifecycle-runtime-ktx", version.ref = "lifecycle" }
lifecycle-service = { module = "androidx.lifecycle:lifecycle-service", version.ref = "lifecycle" }
camera-core = { module = "androidx.camera:camera-core", version.ref = "camerax" }
camera-camera2 = { module = "androidx.camera:camera-camera2", version.ref = "camerax" }
camera-lifecycle = { module = "androidx.camera:camera-lifecycle", version.ref = "camerax" }
camera-view = { module = "androidx.camera:camera-view", version.ref = "camerax" }
mlkit-barcode = { module = "com.google.mlkit:barcode-scanning", version.ref = "mlkitBarcode" }
play-services-location = { module = "com.google.android.gms:play-services-location", version.ref = "playLocation" }
security-crypto = { module = "androidx.security:security-crypto", version.ref = "securityCrypto" }
junit = { module = "junit:junit", version.ref = "junit" }
robolectric = { module = "org.robolectric:robolectric", version.ref = "robolectric" }
androidx-test-core = { module = "androidx.test:core", version.ref = "androidxTestCore" }

[plugins]
android-application = { id = "com.android.application", version.ref = "agp" }
kotlin-android = { id = "org.jetbrains.kotlin.android", version.ref = "kotlin" }
kotlin-jvm = { id = "org.jetbrains.kotlin.jvm", version.ref = "kotlin" }
kotlin-serialization = { id = "org.jetbrains.kotlin.plugin.serialization", version.ref = "kotlin" }
kotlin-compose = { id = "org.jetbrains.kotlin.plugin.compose", version.ref = "kotlin" }
ksp = { id = "com.google.devtools.ksp", version.ref = "ksp" }
```

**バージョンが解決できない場合の対処**: Gradle は「そのバージョンは存在しない」と具体名で報告する。AGP の入手可能なバージョンは次で確認し、カタログの `agp` を最新の安定版に上げる。

```powershell
(Invoke-WebRequest "https://dl.google.com/dl/android/maven2/com/android/tools/build/gradle/maven-metadata.xml").Content
```

Kotlin と KSP は**必ず組で上げる**（KSP のバージョンは `<kotlin>-<ksp>` の形で Kotlin に固定されている）。

- [ ] **Step 3: ルートの Gradle スクリプトを書く**

`android/settings.gradle.kts`:

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

rootProject.name = "edgewatcher-android"
include(":domain")
include(":app")
```

`android/build.gradle.kts`:

```kotlin
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.ksp) apply false
}
```

`android/gradle.properties`:

```properties
org.gradle.jvmargs=-Xmx3g -Dfile.encoding=UTF-8
org.gradle.caching=true
android.useAndroidX=true
kotlin.code.style=official
```

- [ ] **Step 4: `:domain` を書く**

`android/domain/build.gradle.kts`:

```kotlin
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.kotlin.jvm)
}

// ツールチェーンの自動取得は使わない。この環境の JAVA_HOME（JDK 20）で走らせ、
// 生成するバイトコードだけを 17 に揃える。
kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

dependencies {
    implementation(libs.kotlinx.coroutines.core)
    testImplementation(libs.junit)
    testImplementation(kotlin("test"))
    testImplementation(libs.kotlinx.coroutines.test)
}

tasks.test {
    useJUnit()
    testLogging { events("passed", "failed", "skipped") }
}
```

`kotlin("jvm")` プラグインを使うことに意味がある。Android SDK がクラスパスに載らないため、`:domain` に `Context` を書いた瞬間にコンパイルが落ちる。

- [ ] **Step 5: `:app` を書く**

`android/app/build.gradle.kts`:

```kotlin
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.ksp)
}

fun apiBaseUrl(): String =
    System.getenv("EW_API_BASE_URL")
        ?: (project.findProperty("ewApiBaseUrl") as String?)
        ?: ""

android {
    namespace = "com.edgewatcher.app"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.edgewatcher"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    flavorDimensions += "backend"
    productFlavors {
        create("mock") {
            dimension = "backend"
            applicationIdSuffix = ".mock"
            buildConfigField("String", "API_BASE_URL", "\"\"")
        }
        create("live") {
            dimension = "backend"
            buildConfigField("String", "API_BASE_URL", "\"${apiBaseUrl()}\"")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
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

    testOptions {
        unitTests {
            isIncludeAndroidResources = true
        }
    }
}

kotlin {
    compilerOptions { jvmTarget.set(JvmTarget.JVM_17) }
}

/**
 * 空の URL を焼いた APK が作れると、何も送っていないアプリを実物と誤認する事故が起きる。
 * ただし設定フェーズで throw すると live をビルドしない場面（:domain:test など）まで
 * 巻き添えで落ちるため、**live を実際にコンパイルするときだけ**走る検証タスクにする。
 */
val verifyLiveApiBaseUrl = tasks.register("verifyLiveApiBaseUrl") {
    doLast {
        if (apiBaseUrl().isBlank()) {
            throw GradleException(
                "live フレーバーには EW_API_BASE_URL が必要です。" +
                    "例: EW_API_BASE_URL=https://xxxx.execute-api.ap-northeast-1.amazonaws.com"
            )
        }
    }
}

tasks.matching { it.name.startsWith("compileLive") }.configureEach {
    dependsOn(verifyLiveApiBaseUrl)
}

dependencies {
    implementation(project(":domain"))

    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.okhttp)

    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.material3)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)
    implementation(libs.activity.compose)
    implementation(libs.lifecycle.runtime.ktx)
    implementation(libs.lifecycle.service)

    implementation(libs.camera.core)
    implementation(libs.camera.camera2)
    implementation(libs.camera.lifecycle)
    implementation(libs.camera.view)
    implementation(libs.mlkit.barcode)
    implementation(libs.play.services.location)
    implementation(libs.security.crypto)

    testImplementation(libs.junit)
    testImplementation(kotlin("test"))
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.robolectric)
    testImplementation(libs.androidx.test.core)
}
```

`android/app/src/main/AndroidManifest.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">

    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.CAMERA" />
    <uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_CAMERA" />
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_LOCATION" />
    <uses-permission android:name="android.permission.WAKE_LOCK" />
    <uses-permission android:name="android.permission.SCHEDULE_EXACT_ALARM" />
    <uses-permission android:name="android.permission.RECEIVE_BOOT_COMPLETED" />
    <uses-permission android:name="android.permission.REQUEST_IGNORE_BATTERY_OPTIMIZATIONS" />

    <application
        android:label="EdgeWatcher"
        android:supportsRtl="true"
        android:theme="@style/Theme.Material3.DayNight.NoActionBar" />

</manifest>
```

`ACCESS_BACKGROUND_LOCATION` は**書かない**。Foreground Service の `location` タイプが「使用中」を成立させるため不要であり、要求すると Play の審査で背景位置情報の正当化が必要になる。

- [ ] **Step 6: `.gitignore` に Android の生成物を足す**

`.gitignore` の末尾に追記:

```gitignore
# Android
android/.gradle/
android/build/
android/*/build/
android/local.properties
android/.kotlin/
*.apk
*.aab
```

`android/gradle/wrapper/gradle-wrapper.jar` は**コミットする**（wrapper は成果物ではなくビルドの入口であり、これが無いとクローンした人がビルドできない）。

- [ ] **Step 7: Android SDK の場所を通し、`android-36` があることを確認する**

```powershell
"sdk.dir=C\:\\Users\\81903\\AppData\\Local\\Android\\Sdk" | Out-File -Encoding utf8 android\local.properties
Get-ChildItem "$env:LOCALAPPDATA\Android\Sdk\platforms" -Directory | Select-Object -ExpandProperty Name
```

`android-36` が無ければ導入する:

```powershell
& "$env:LOCALAPPDATA\Android\Sdk\cmdline-tools\latest\bin\sdkmanager.bat" "platforms;android-36" "build-tools;36.0.0"
```

- [ ] **Step 8: ビルドが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test :app:assembleMockDebug --no-daemon; Pop-Location
```

期待: `BUILD SUCCESSFUL`。`:domain:test` はテストが0件でも成功する。

- [ ] **Step 9: `live` フレーバーが環境変数無しで失敗することを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:assembleLiveDebug --no-daemon; Pop-Location
```

期待: `BUILD FAILED`。失敗するのは `:app:verifyLiveApiBaseUrl` で、メッセージに `live フレーバーには EW_API_BASE_URL が必要です` が含まれる。

**この検証タスクを設定フェーズの `throw` にしてはならない。** 設定フェーズは live をビルドしない場面（`:domain:test` など）でも実行されるため、巻き添えでビルド全体が落ちる。

続いて、値を与えれば通ることを確認する:

```powershell
$env:EW_API_BASE_URL = "https://example.invalid"
Push-Location android; .\gradlew.bat :app:assembleLiveDebug --no-daemon; Pop-Location
Remove-Item Env:\EW_API_BASE_URL
```

期待: `BUILD SUCCESSFUL`。

- [ ] **Step 10: Commit**

```bash
git add android .gitignore
git commit -m "build(android): Gradle の骨組みと mock/live フレーバーを追加"
```

---

## フェーズ1: `:domain` の純ロジック

このフェーズが計画の中心である。ここが終わった時点で、要件に書かれた判断の大半は Android を一切起動せずに検証済みになる。

**このフェーズの全 Task で使うテスト実行コマンド:**

```powershell
Push-Location android; .\gradlew.bat :domain:test --no-daemon; Pop-Location
```

個別のテストだけを走らせる場合は `--tests` を付ける（例: `--tests "com.edgewatcher.domain.UlidTest"`）。

### Task 2: `Clock` と ULID の採番

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Clock.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Ulid.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/UlidTest.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `interface Clock { fun now(): java.time.Instant }`
  - `class FixedClock(var instant: Instant) : Clock`（テスト用。`:domain` の main に置く。`:app` のテストからも使うため）
  - `object Ulid { fun generate(at: Instant, random: Random = Random.Default): String }` — 26文字の Crockford base32
  - `object Ulid { fun timestampOf(ulid: String): Instant }`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/UlidTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.random.Random
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class UlidTest {

    @Test
    fun `26文字で Crockford base32 の文字だけを含む`() {
        val id = Ulid.generate(Instant.parse("2026-09-10T05:32:05Z"), Random(1))
        assertEquals(26, id.length)
        assertTrue(id.all { it in "0123456789ABCDEFGHJKMNPQRSTVWXYZ" }, "unexpected chars: $id")
    }

    @Test
    fun `タイムスタンプ部から採番時刻をミリ秒精度で復元できる`() {
        // capturedAt とずれると、サーバの日別クエリで観測が別の日に並ぶ
        val at = Instant.parse("2026-09-10T05:32:05.123Z")
        val id = Ulid.generate(at, Random(1))
        assertEquals(at, Ulid.timestampOf(id))
    }

    @Test
    fun `同じ時刻でも乱数が違えば別の ID になる`() {
        val at = Instant.parse("2026-09-10T05:32:05Z")
        assertTrue(Ulid.generate(at, Random(1)) != Ulid.generate(at, Random(2)))
    }

    @Test
    fun `時刻が後のものは辞書順でも後になる`() {
        val early = Ulid.generate(Instant.parse("2026-09-10T05:32:05Z"), Random(1))
        val late = Ulid.generate(Instant.parse("2026-09-10T05:37:05Z"), Random(1))
        assertTrue(early < late, "$early should sort before $late")
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UlidTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: Ulid`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Clock.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

interface Clock {
    fun now(): Instant
}

object SystemClock : Clock {
    override fun now(): Instant = Instant.now()
}

/** テスト用。時刻を進めたい場面では [instant] を差し替える。 */
class FixedClock(var instant: Instant) : Clock {
    override fun now(): Instant = instant
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/Ulid.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.random.Random

/**
 * ULID の採番。タイムスタンプ部は必ず capturedAt に一致させること。
 * サーバは日別クエリの範囲をここから導くため、ずれると観測が別の日に並ぶ。
 */
object Ulid {

    private const val ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
    private const val TIME_CHARS = 10
    private const val RANDOM_CHARS = 16

    fun generate(at: Instant, random: Random = Random.Default): String {
        val sb = StringBuilder(TIME_CHARS + RANDOM_CHARS)
        var millis = at.toEpochMilli()
        val time = CharArray(TIME_CHARS)
        for (i in TIME_CHARS - 1 downTo 0) {
            time[i] = ALPHABET[(millis % 32).toInt()]
            millis /= 32
        }
        sb.append(time)
        repeat(RANDOM_CHARS) { sb.append(ALPHABET[random.nextInt(32)]) }
        return sb.toString()
    }

    fun timestampOf(ulid: String): Instant {
        var millis = 0L
        for (i in 0 until TIME_CHARS) {
            millis = millis * 32 + ALPHABET.indexOf(ulid[i])
        }
        return Instant.ofEpochMilli(millis)
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UlidTest" --no-daemon; Pop-Location
```

期待: PASS（4件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): Clock と ULID の採番を追加"
```

---

### Task 3: 画像の符号化方針とサイズガード

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/ImagePolicy.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/ImagePolicyTest.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `data class EncodeStep(val longEdge: Int, val quality: Int)`
  - `object ImagePolicy` — `MAX_TOTAL_BYTES`, `THUMBNAIL`, `firstStep()`, `nextStep(current)`, `fits(imageBytes, thumbBytes)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/ImagePolicyTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ImagePolicyTest {

    @Test
    fun `最初の段階は長辺1600の品質80`() {
        assertEquals(EncodeStep(longEdge = 1600, quality = 80), ImagePolicy.firstStep())
    }

    @Test
    fun `サムネイルは長辺160の品質70で固定`() {
        assertEquals(EncodeStep(longEdge = 160, quality = 70), ImagePolicy.THUMBNAIL)
    }

    @Test
    fun `合計が上限以下なら収まったと判定する`() {
        assertTrue(ImagePolicy.fits(imageBytes = 4_400_000, thumbBytes = 100_000))
    }

    @Test
    fun `合計が上限を1バイト超えたら収まっていないと判定する`() {
        assertFalse(ImagePolicy.fits(imageBytes = 4_400_001, thumbBytes = 100_000))
    }

    @Test
    fun `段階は品質を下げ、最後に解像度を落とす`() {
        val steps = generateSequence(ImagePolicy.firstStep()) { ImagePolicy.nextStep(it) }.toList()
        assertEquals(
            listOf(
                EncodeStep(1600, 80),
                EncodeStep(1600, 65),
                EncodeStep(1600, 50),
                EncodeStep(1280, 50),
            ),
            steps,
        )
    }

    @Test
    fun `最後の段階の次は無い`() {
        assertNull(ImagePolicy.nextStep(EncodeStep(1280, 50)))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ImagePolicyTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: ImagePolicy`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/ImagePolicy.kt`:

```kotlin
package com.edgewatcher.domain

data class EncodeStep(val longEdge: Int, val quality: Int)

/**
 * 送っても必ず 400 になるフレームをバッファに積まないための方針。
 * 上限 4.5MB は Lambda の同期呼び出しペイロード 6MB を base64 のふくらみで割った値。
 */
object ImagePolicy {

    const val MAX_TOTAL_BYTES = 4_500_000

    val THUMBNAIL = EncodeStep(longEdge = 160, quality = 70)

    private val STEPS = listOf(
        EncodeStep(1600, 80),
        EncodeStep(1600, 65),
        EncodeStep(1600, 50),
        EncodeStep(1280, 50),
    )

    fun firstStep(): EncodeStep = STEPS.first()

    fun nextStep(current: EncodeStep): EncodeStep? {
        val i = STEPS.indexOf(current)
        return if (i < 0 || i == STEPS.lastIndex) null else STEPS[i + 1]
    }

    fun fits(imageBytes: Int, thumbBytes: Int): Boolean =
        imageBytes + thumbBytes <= MAX_TOTAL_BYTES
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ImagePolicyTest" --no-daemon; Pop-Location
```

期待: PASS（6件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 画像の符号化方針とサイズガードを追加"
```

---

### Task 4: API ポートとペアリングのリトライ

`EdgeWatcherApi` のポートと結果型をここで定義する。以後の Task はすべてこれを使う。
**結果型が HTTP のステータスを外へ漏らさない**のが要点で、`:domain` は「捨ててよいのか / 待つべきか / 取り直すべきか」だけを見る。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Api.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/PairingService.kt`
- Create: `android/domain/src/test/kotlin/com/edgewatcher/domain/FakeApi.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/PairingServiceTest.kt`

**Interfaces:**
- Consumes: `Clock`（Task 2）
- Produces:
  - `data class DeviceInfo(val model: String, val osVersion: String, val appVersion: String)`
  - `class UploadFrame(observationId: String, capturedAt: Instant, lat: Double?, lng: Double?, image: ByteArray, thumbnail: ByteArray)`
  - `sealed interface PairResult` — `Success(deviceId, deviceSecret)` / `NotFound` / `Unusable` / `Rejected` / `Unreachable`
  - `sealed interface TokenResult` — `Success(sessionToken, expiresAt)` / `Invalid` / `Retryable`
  - `sealed interface UploadResult` — `Success(intervalMinutes)` / `Discard` / `Unauthorized` / `Retryable`
  - `sealed interface LogoutResult` — `Success` / `Unauthorized` / `Retryable`
  - `interface EdgeWatcherApi { suspend fun pair(...); suspend fun token(...); suspend fun upload(...); suspend fun logout(...) }`
  - `class PairingService(api: EdgeWatcherApi, sleep: suspend (Long) -> Unit)` — `codeFrom(payload): String?`, `pair(pairingCode, deviceInfo): PairingOutcome`
  - `sealed interface PairingOutcome` — `Paired(deviceId, deviceSecret)` / `Unusable` / `NotFound` / `Rejected` / `Unreachable`
  - `class FakeApi : EdgeWatcherApi`（テスト専用。以後の Task でも使う）

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/FakeApi.kt`:

```kotlin
package com.edgewatcher.domain

/**
 * 応答を先入れ先出しで返す。**最後の1件は繰り返し返す**ので、
 * 「404 が続く」は NotFound を1件入れておけば表現できる。
 */
class FakeApi : EdgeWatcherApi {

    val pairCalls = mutableListOf<Pair<String, DeviceInfo>>()
    val tokenCalls = mutableListOf<Pair<String, String>>()
    val uploadCalls = mutableListOf<Pair<String, UploadFrame>>()
    val logoutCalls = mutableListOf<String>()

    val pairResults = mutableListOf<PairResult>()
    val tokenResults = mutableListOf<TokenResult>()
    val uploadResults = mutableListOf<UploadResult>()
    val logoutResults = mutableListOf<LogoutResult>()

    private fun <T> next(queue: MutableList<T>, name: String): T {
        check(queue.isNotEmpty()) { "$name の応答が用意されていない" }
        return if (queue.size > 1) queue.removeAt(0) else queue.first()
    }

    override suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): PairResult {
        pairCalls += pairingCode to deviceInfo
        return next(pairResults, "pair")
    }

    override suspend fun token(deviceId: String, deviceSecret: String): TokenResult {
        tokenCalls += deviceId to deviceSecret
        return next(tokenResults, "token")
    }

    override suspend fun upload(sessionToken: String, frame: UploadFrame): UploadResult {
        uploadCalls += sessionToken to frame
        return next(uploadResults, "upload")
    }

    override suspend fun logout(sessionToken: String): LogoutResult {
        logoutCalls += sessionToken
        return next(logoutResults, "logout")
    }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/PairingServiceTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class PairingServiceTest {

    private val info = DeviceInfo(model = "Pixel 6a", osVersion = "16", appVersion = "1.0.0")

    private fun service(api: FakeApi, sleeps: MutableList<Long> = mutableListOf()) =
        PairingService(api) { sleeps += it }

    @Test
    fun `QR ペイロードから接頭辞を剥がす`() {
        val s = service(FakeApi())
        assertEquals("PAIR_ABC123", s.codeFrom("ew1:PAIR_ABC123"))
    }

    @Test
    fun `接頭辞が無いペイロードは自分宛てではない`() {
        val s = service(FakeApi())
        assertNull(s.codeFrom("https://example.com"))
    }

    @Test
    fun `接頭辞だけで中身が無いものも自分宛てではない`() {
        val s = service(FakeApi())
        assertNull(s.codeFrom("ew1:"))
    }

    @Test
    fun `成功したら1リクエストで返る`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.Success("DEV1", "SECRET1") }
        val sleeps = mutableListOf<Long>()

        val outcome = service(api, sleeps).pair("CODE", info)

        assertEquals(PairingOutcome.Paired("DEV1", "SECRET1"), outcome)
        assertEquals(1, api.pairCalls.size)
        assertEquals(emptyList(), sleeps)
    }

    @Test
    fun `404 が続いたら初回を含めて4リクエストで諦める`() = runTest {
        // GSI2 は結果整合なので「待てば成功する空振り」がありうる。
        // ただし無限には待たない。
        val api = FakeApi().apply { pairResults += PairResult.NotFound }
        val sleeps = mutableListOf<Long>()

        val outcome = service(api, sleeps).pair("CODE", info)

        assertEquals(PairingOutcome.NotFound, outcome)
        assertEquals(4, api.pairCalls.size)
        assertEquals(listOf(1_000L, 1_000L, 1_000L), sleeps)
    }

    @Test
    fun `3リクエスト目で成功したらそこで止まる`() = runTest {
        val api = FakeApi().apply {
            pairResults += PairResult.NotFound
            pairResults += PairResult.NotFound
            pairResults += PairResult.Success("DEV1", "SECRET1")
        }
        val sleeps = mutableListOf<Long>()

        val outcome = service(api, sleeps).pair("CODE", info)

        assertEquals(PairingOutcome.Paired("DEV1", "SECRET1"), outcome)
        assertEquals(3, api.pairCalls.size)
        assertEquals(listOf(1_000L, 1_000L), sleeps)
    }

    @Test
    fun `409 は待っても解決しないので即座に諦める`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.Unusable }
        val sleeps = mutableListOf<Long>()

        val outcome = service(api, sleeps).pair("CODE", info)

        assertEquals(PairingOutcome.Unusable, outcome)
        assertEquals(1, api.pairCalls.size)
        assertEquals(emptyList(), sleeps)
    }

    @Test
    fun `400 はリトライしない`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.Rejected }

        assertEquals(PairingOutcome.Rejected, service(api).pair("CODE", info))
        assertEquals(1, api.pairCalls.size)
    }

    @Test
    fun `到達不能は404と混ぜず、独立した結果として返す`() = runTest {
        // 圏外の端末に「この QR は使えません」と出すと、
        // オーナーが QR を再発行しても直らない誤誘導になる。
        val api = FakeApi().apply { pairResults += PairResult.Unreachable }

        assertEquals(PairingOutcome.Unreachable, service(api).pair("CODE", info))
        assertEquals(1, api.pairCalls.size)
    }

    @Test
    fun `端末情報をそのまま送る`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.Success("DEV1", "SECRET1") }

        service(api).pair("CODE", info)

        assertEquals("CODE" to info, api.pairCalls.single())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PairingServiceTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: EdgeWatcherApi`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Api.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

data class DeviceInfo(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

/** 送信1件分。equals は使わないので data class にしない（ByteArray の同一性が紛らわしいため）。 */
class UploadFrame(
    val observationId: String,
    val capturedAt: Instant,
    val lat: Double?,
    val lng: Double?,
    val image: ByteArray,
    val thumbnail: ByteArray,
)

sealed interface PairResult {
    data class Success(val deviceId: String, val deviceSecret: String) : PairResult

    /** 404 PAIRING_NOT_FOUND。GSI2 の伝播待ちで、待てば成功しうる。 */
    data object NotFound : PairResult

    /** 409 消費済み / 期限切れ。待っても解決しない。 */
    data object Unusable : PairResult

    /** 400 VALIDATION_ERROR。 */
    data object Rejected : PairResult

    /** ネットワーク到達不能、および 5xx。 */
    data object Unreachable : PairResult
}

sealed interface TokenResult {
    data class Success(val sessionToken: String, val expiresAt: Instant) : TokenResult

    /**
     * 401。**資格情報を捨ててよい唯一の合図。**
     * `api.yml` が「失敗はすべて 401」と定めているため、
     * サーバが資格情報の無効を断言する応答はこれしかない。
     */
    data object Invalid : TokenResult

    /**
     * 5xx / ネットワーク断 / 400。
     * **ここで資格情報を捨ててはならない。** 圏外の端末が deviceSecret を失うと、
     * 屋外の設置場所まで行って QR を読み直す以外に復旧手段がなくなる。
     */
    data object Retryable : TokenResult
}

sealed interface UploadResult {
    data class Success(val intervalMinutes: Int) : UploadResult

    /** 400 / 413。リトライ不能。フレームをバッファから捨てる。 */
    data object Discard : UploadResult

    /** 401 / 403。セッションを取り直す。フレームは残す。 */
    data object Unauthorized : UploadResult

    /** 5xx / ネットワーク断。バックオフして待つ。フレームは残す。 */
    data object Retryable : UploadResult
}

sealed interface LogoutResult {
    data object Success : LogoutResult
    data object Unauthorized : LogoutResult
    data object Retryable : LogoutResult
}

interface EdgeWatcherApi {
    suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): PairResult
    suspend fun token(deviceId: String, deviceSecret: String): TokenResult
    suspend fun upload(sessionToken: String, frame: UploadFrame): UploadResult
    suspend fun logout(sessionToken: String): LogoutResult
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/PairingService.kt`:

```kotlin
package com.edgewatcher.domain

const val QR_PREFIX = "ew1:"

sealed interface PairingOutcome {
    data class Paired(val deviceId: String, val deviceSecret: String) : PairingOutcome

    /** 「この QR コードは使えません」 */
    data object Unusable : PairingOutcome

    /** リトライを尽くしても引けなかった。 */
    data object NotFound : PairingOutcome

    /** 「サーバーに接続できません」 */
    data object Unreachable : PairingOutcome

    data object Rejected : PairingOutcome
}

class PairingService(
    private val api: EdgeWatcherApi,
    private val sleep: suspend (millis: Long) -> Unit,
) {
    companion object {
        /** 初回1回 + リトライ3回。 */
        const val MAX_ATTEMPTS = 4
        const val RETRY_INTERVAL_MILLIS = 1_000L
    }

    /**
     * QR ペイロードから pairingCode を取り出す。自分宛てでなければ null。
     * **コードの形式は検証しない。** 妥当性を決めるのはサーバであり、
     * 端末が形式を強制するとモックの Web が発行するコードと噛み合わなくなる。
     */
    fun codeFrom(payload: String): String? =
        if (payload.startsWith(QR_PREFIX)) {
            payload.removePrefix(QR_PREFIX).takeIf { it.isNotEmpty() }
        } else {
            null
        }

    suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): PairingOutcome {
        repeat(MAX_ATTEMPTS) { attempt ->
            when (val result = api.pair(pairingCode, deviceInfo)) {
                is PairResult.Success ->
                    return PairingOutcome.Paired(result.deviceId, result.deviceSecret)
                PairResult.Unusable -> return PairingOutcome.Unusable
                PairResult.Rejected -> return PairingOutcome.Rejected
                PairResult.Unreachable -> return PairingOutcome.Unreachable
                PairResult.NotFound ->
                    if (attempt < MAX_ATTEMPTS - 1) sleep(RETRY_INTERVAL_MILLIS)
            }
        }
        return PairingOutcome.NotFound
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PairingServiceTest" --no-daemon; Pop-Location
```

期待: PASS（9件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): API ポートとペアリングのリトライを追加"
```

---

### Task 5: セッションと 401 起点の自己修復

**この計画でもっとも壊してはいけない Task。** 資格情報を消す条件が広がると、通信状態が悪いだけの端末が現地に行かないと復旧できなくなる。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Credentials.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/SessionManager.kt`
- Create: `android/domain/src/test/kotlin/com/edgewatcher/domain/InMemoryCredentialStore.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/SessionManagerTest.kt`

**Interfaces:**
- Consumes: `EdgeWatcherApi`, `TokenResult`（Task 4）、`Clock`（Task 2）
- Produces:
  - `data class Credentials(val deviceId: String, val deviceSecret: String)`
  - `data class Session(val token: String, val expiresAt: Instant)`
  - `interface CredentialStore` — `readCredentials()`, `writeCredentials(c)`, `readSession()`, `writeSession(s)`, `clear()`
  - `sealed interface AuthOutcome` — `Ready(sessionToken)` / `CredentialsInvalid` / `Retryable` / `NotPaired`
  - `class SessionManager(api, store, clock)` — `currentToken(): AuthOutcome`, `refresh(): AuthOutcome`
  - `class InMemoryCredentialStore : CredentialStore`（テスト専用。以後の Task でも使う）

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/InMemoryCredentialStore.kt`:

```kotlin
package com.edgewatcher.domain

class InMemoryCredentialStore(
    private var credentials: Credentials? = null,
    private var session: Session? = null,
) : CredentialStore {

    var clearCount = 0
        private set

    override fun readCredentials(): Credentials? = credentials
    override fun writeCredentials(credentials: Credentials) { this.credentials = credentials }
    override fun readSession(): Session? = session
    override fun writeSession(session: Session) { this.session = session }

    override fun clear() {
        credentials = null
        session = null
        clearCount++
    }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/SessionManagerTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull

class SessionManagerTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")
    private val creds = Credentials(deviceId = "DEV1", deviceSecret = "SECRET1")

    private fun manager(api: FakeApi, store: InMemoryCredentialStore) =
        SessionManager(api, store, FixedClock(now))

    @Test
    fun `ペアリングしていなければ NotPaired を返しサーバを呼ばない`() = runTest {
        val api = FakeApi()
        val store = InMemoryCredentialStore()

        assertEquals(AuthOutcome.NotPaired, manager(api, store).currentToken())
        assertEquals(0, api.tokenCalls.size)
    }

    @Test
    fun `有効なセッションがあればサーバを呼ばない`() = runTest {
        val api = FakeApi()
        val store = InMemoryCredentialStore(
            credentials = creds,
            session = Session("TOKEN1", now.plusSeconds(3600)),
        )

        assertEquals(AuthOutcome.Ready("TOKEN1"), manager(api, store).currentToken())
        assertEquals(0, api.tokenCalls.size)
    }

    @Test
    fun `期限が切れていれば先に取り直す`() = runTest {
        val api = FakeApi().apply {
            tokenResults += TokenResult.Success("TOKEN2", now.plusSeconds(43_200))
        }
        val store = InMemoryCredentialStore(
            credentials = creds,
            session = Session("TOKEN1", now.minusSeconds(1)),
        )

        assertEquals(AuthOutcome.Ready("TOKEN2"), manager(api, store).currentToken())
        assertEquals(listOf("DEV1" to "SECRET1"), api.tokenCalls)
        assertEquals("TOKEN2", store.readSession()?.token)
    }

    @Test
    fun `セッションが無ければ取りに行く`() = runTest {
        val api = FakeApi().apply {
            tokenResults += TokenResult.Success("TOKEN1", now.plusSeconds(43_200))
        }
        val store = InMemoryCredentialStore(credentials = creds)

        assertEquals(AuthOutcome.Ready("TOKEN1"), manager(api, store).currentToken())
    }

    @Test
    fun `refresh は期限内でも必ずサーバを呼ぶ`() = runTest {
        // 403 を受けた後に呼ばれる経路。手元の期限判定は当てにならない。
        val api = FakeApi().apply {
            tokenResults += TokenResult.Success("TOKEN2", now.plusSeconds(43_200))
        }
        val store = InMemoryCredentialStore(
            credentials = creds,
            session = Session("TOKEN1", now.plusSeconds(3600)),
        )

        assertEquals(AuthOutcome.Ready("TOKEN2"), manager(api, store).refresh())
        assertEquals(1, api.tokenCalls.size)
    }

    @Test
    fun `401 のときだけ資格情報を消す`() = runTest {
        val api = FakeApi().apply { tokenResults += TokenResult.Invalid }
        val store = InMemoryCredentialStore(credentials = creds, session = Session("T", now))

        assertEquals(AuthOutcome.CredentialsInvalid, manager(api, store).refresh())
        assertEquals(1, store.clearCount)
        assertEquals(null, store.readCredentials())
    }

    @Test
    fun `5xx やネットワーク断では資格情報を絶対に消さない`() = runTest {
        // ここが広がると、圏外の端末が deviceSecret を失い、
        // 屋外の設置場所まで行って QR を読み直す以外に復旧手段がなくなる。
        val api = FakeApi().apply { tokenResults += TokenResult.Retryable }
        val store = InMemoryCredentialStore(credentials = creds, session = Session("T", now))

        assertEquals(AuthOutcome.Retryable, manager(api, store).refresh())
        assertEquals(0, store.clearCount)
        assertNotNull(store.readCredentials())
    }

    @Test
    fun `Retryable が何度続いても資格情報は残る`() = runTest {
        val api = FakeApi().apply { tokenResults += TokenResult.Retryable }
        val store = InMemoryCredentialStore(credentials = creds)
        val m = manager(api, store)

        repeat(50) { assertEquals(AuthOutcome.Retryable, m.refresh()) }

        assertEquals(0, store.clearCount)
        assertNotNull(store.readCredentials())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.SessionManagerTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: SessionManager`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Credentials.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

data class Credentials(val deviceId: String, val deviceSecret: String)

data class Session(val token: String, val expiresAt: Instant)

/** 実装は暗号化して保存する。値をログに出してはならない。 */
interface CredentialStore {
    fun readCredentials(): Credentials?
    fun writeCredentials(credentials: Credentials)
    fun readSession(): Session?
    fun writeSession(session: Session)
    fun clear()
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/SessionManager.kt`:

```kotlin
package com.edgewatcher.domain

sealed interface AuthOutcome {
    data class Ready(val sessionToken: String) : AuthOutcome

    /** 資格情報が無効。全消去して未ペアリング画面へ戻る。 */
    data object CredentialsInvalid : AuthOutcome

    /** 一時的な失敗。待って再試行する。**資格情報は残す。** */
    data object Retryable : AuthOutcome

    /** そもそもペアリングしていない。 */
    data object NotPaired : AuthOutcome
}

class SessionManager(
    private val api: EdgeWatcherApi,
    private val store: CredentialStore,
    private val clock: Clock,
) {
    /**
     * 有効なトークンを返す。期限切れなら先に取り直す。
     * 期限の事前確認は往復を減らす最適化にすぎない。端末の時計はずれるため、
     * 判定が外れた場合は [refresh] が拾う。
     */
    suspend fun currentToken(): AuthOutcome {
        store.readCredentials() ?: return AuthOutcome.NotPaired
        val session = store.readSession()
        if (session != null && session.expiresAt.isAfter(clock.now())) {
            return AuthOutcome.Ready(session.token)
        }
        return refresh()
    }

    /** 401 / 403 を受けた後に呼ぶ。必ずサーバへ取りに行く。 */
    suspend fun refresh(): AuthOutcome {
        val credentials = store.readCredentials() ?: return AuthOutcome.NotPaired
        return when (val result = api.token(credentials.deviceId, credentials.deviceSecret)) {
            is TokenResult.Success -> {
                store.writeSession(Session(result.sessionToken, result.expiresAt))
                AuthOutcome.Ready(result.sessionToken)
            }
            TokenResult.Invalid -> {
                store.clear()
                AuthOutcome.CredentialsInvalid
            }
            TokenResult.Retryable -> AuthOutcome.Retryable
        }
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.SessionManagerTest" --no-daemon; Pop-Location
```

期待: PASS（8件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): セッション管理と401起点の自己修復を追加"
```

---

### Task 6: 観測バッファのポートと退避規則

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Observations.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/BufferPolicy.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/BufferPolicyTest.kt`

**Interfaces:**
- Consumes: `UploadFrame`（Task 4）
- Produces:
  - `data class ObservationMeta(observationId, capturedAt, imagePath, thumbPath, lat, lng, sizeBytes, attemptCount = 0, nextAttemptAt = null)`
  - `interface ObservationStore` — `insert(meta, image, thumbnail)`, `all()`（capturedAt 昇順）, `totalBytes()`, `load(id): UploadFrame?`, `delete(id)`, `recordFailure(id, attemptCount, nextAttemptAt)`
  - `object BufferPolicy` — `MAX_COUNT = 288`, `MAX_BYTES = 524_288_000L`, `evictions(all): List<String>`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/BufferPolicyTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals

class BufferPolicyTest {

    private val base = Instant.parse("2026-09-10T00:00:00Z")

    /** capturedAt 昇順で count 件。1件あたり sizeBytes バイト。 */
    private fun rows(count: Int, sizeBytes: Long): List<ObservationMeta> =
        (0 until count).map { i ->
            ObservationMeta(
                observationId = "OBS%04d".format(i),
                capturedAt = base.plusSeconds(i * 300L),
                imagePath = "img$i.jpg",
                thumbPath = "thumb$i.jpg",
                lat = null,
                lng = null,
                sizeBytes = sizeBytes,
            )
        }

    @Test
    fun `空なら何も削らない`() {
        assertEquals(emptyList(), BufferPolicy.evictions(emptyList()))
    }

    @Test
    fun `上限ちょうどなら何も削らない`() {
        assertEquals(emptyList(), BufferPolicy.evictions(rows(288, 1_000)))
    }

    @Test
    fun `件数が1件超えたら最古を1件だけ削る`() {
        assertEquals(listOf("OBS0000"), BufferPolicy.evictions(rows(289, 1_000)))
    }

    @Test
    fun `件数が大きく超えたら古い順に必要な数だけ削る`() {
        assertEquals(
            listOf("OBS0000", "OBS0001", "OBS0002"),
            BufferPolicy.evictions(rows(291, 1_000)),
        )
    }

    @Test
    fun `件数が上限内でも容量を超えたら削る`() {
        // 10件 × 60MiB = 600MiB > 500MiB。2件消せば 480MiB で収まる。
        val sixtyMiB = 60L * 1024 * 1024
        assertEquals(
            listOf("OBS0000", "OBS0001"),
            BufferPolicy.evictions(rows(10, sixtyMiB)),
        )
    }

    @Test
    fun `容量ちょうどなら削らない`() {
        val fiftyMiB = 50L * 1024 * 1024
        assertEquals(emptyList(), BufferPolicy.evictions(rows(10, fiftyMiB)))
    }

    @Test
    fun `件数と容量の両方を超えたら両方が収まるまで削る`() {
        val twoMiB = 2L * 1024 * 1024
        // 300件 × 2MiB = 600MiB。件数では12件、容量では50件削る必要がある。
        // 厳しい方（50件）まで削る。
        val victims = BufferPolicy.evictions(rows(300, twoMiB))
        assertEquals(50, victims.size)
        assertEquals("OBS0000", victims.first())
        assertEquals("OBS0049", victims.last())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.BufferPolicyTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: ObservationMeta`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Observations.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

/**
 * 画像本体はファイルに置き、ここにはメタデータだけを持つ。
 * BLOB を SQLite に入れると DB が肥大し、削除しても領域が戻りにくい。
 */
data class ObservationMeta(
    val observationId: String,
    val capturedAt: Instant,
    val imagePath: String,
    val thumbPath: String,
    val lat: Double?,
    val lng: Double?,
    val sizeBytes: Long,
    val attemptCount: Int = 0,
    val nextAttemptAt: Instant? = null,
)

interface ObservationStore {
    suspend fun insert(meta: ObservationMeta, image: ByteArray, thumbnail: ByteArray)

    /** **capturedAt 昇順**で返す。[BufferPolicy] と [UploadOrder] がこの順序を前提にする。 */
    suspend fun all(): List<ObservationMeta>

    suspend fun totalBytes(): Long

    /** 画像を読み出して送信用のフレームに組み立てる。行が無ければ null。 */
    suspend fun load(observationId: String): UploadFrame?

    /** 行と画像ファイルの両方を消す。 */
    suspend fun delete(observationId: String)

    suspend fun recordFailure(observationId: String, attemptCount: Int, nextAttemptAt: Instant)
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/BufferPolicy.kt`:

```kotlin
package com.edgewatcher.domain

/**
 * 件数だけで制限すると高解像度設定でストレージを圧迫し、
 * 容量だけで制限すると低解像度設定で保持期間を超えた画像を大量に抱える。
 * 両方置くことで、どちらの設定でも破綻しない。
 */
object BufferPolicy {

    /** 5分間隔で24時間相当。 */
    const val MAX_COUNT = 288

    /** 500MiB。 */
    const val MAX_BYTES = 524_288_000L

    /**
     * 削除すべき observationId を古い順に返す。
     * [all] は capturedAt 昇順であること。
     */
    fun evictions(all: List<ObservationMeta>): List<String> {
        var count = all.size
        var bytes = all.sumOf { it.sizeBytes }
        val victims = mutableListOf<String>()
        var i = 0
        while (i < all.size && (count > MAX_COUNT || bytes > MAX_BYTES)) {
            victims += all[i].observationId
            bytes -= all[i].sizeBytes
            count--
            i++
        }
        return victims
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.BufferPolicyTest" --no-daemon; Pop-Location
```

期待: PASS（7件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 観測バッファのポートと退避規則を追加"
```

---

### Task 7: 送信順序と指数バックオフ

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/UploadOrder.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Backoff.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/UploadOrderTest.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/BackoffTest.kt`

**Interfaces:**
- Consumes: `ObservationMeta`（Task 6）
- Produces:
  - `object UploadOrder { fun pick(all: List<ObservationMeta>): ObservationMeta? }`
  - `object Backoff { const val BASE_SECONDS = 5L; const val MAX_SECONDS = 300L; fun delaySeconds(consecutiveFailures: Int): Long }`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/UploadOrderTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class UploadOrderTest {

    private val base = Instant.parse("2026-09-10T00:00:00Z")

    private fun row(index: Int, attemptCount: Int = 0) = ObservationMeta(
        observationId = "OBS%04d".format(index),
        capturedAt = base.plusSeconds(index * 300L),
        imagePath = "img$index.jpg",
        thumbPath = "thumb$index.jpg",
        lat = null,
        lng = null,
        sizeBytes = 300_000,
        attemptCount = attemptCount,
    )

    @Test
    fun `空なら選ばない`() {
        assertNull(UploadOrder.pick(emptyList()))
    }

    @Test
    fun `1件しかなければそれを選ぶ`() {
        assertEquals("OBS0000", UploadOrder.pick(listOf(row(0)))?.observationId)
    }

    @Test
    fun `未試行の最新があればそれを最優先で選ぶ`() {
        // 長時間オフラインから復帰したとき、単純な FIFO だと
        // Web のダッシュボードに何時間も前の画像が出続ける。
        val all = listOf(row(0, attemptCount = 3), row(1, attemptCount = 3), row(2))
        assertEquals("OBS0002", UploadOrder.pick(all)?.observationId)
    }

    @Test
    fun `最新も試行済みなら最古を選ぶ`() {
        val all = listOf(row(0, attemptCount = 3), row(1, attemptCount = 3), row(2, attemptCount = 1))
        assertEquals("OBS0000", UploadOrder.pick(all)?.observationId)
    }

    @Test
    fun `全件が未試行なら最新を選ぶ`() {
        val all = listOf(row(0), row(1), row(2))
        assertEquals("OBS0002", UploadOrder.pick(all)?.observationId)
    }

    @Test
    fun `新しい撮影が入るたびに最新優先が再び効く`() {
        // 最新を送り終えて消えたあと、次は古い順に戻る
        val afterNewestSent = listOf(row(0, attemptCount = 3), row(1, attemptCount = 3))
        assertEquals("OBS0000", UploadOrder.pick(afterNewestSent)?.observationId)

        // そこへ新しい撮影が1件入ると、また最新が先になる
        val withNewCapture = afterNewestSent + row(9)
        assertEquals("OBS0009", UploadOrder.pick(withNewCapture)?.observationId)
    }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/BackoffTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class BackoffTest {

    @Test
    fun `1回目の失敗の後は5秒待つ`() {
        assertEquals(5L, Backoff.delaySeconds(1))
    }

    @Test
    fun `失敗のたびに倍になる`() {
        assertEquals(listOf(5L, 10L, 20L, 40L, 80L, 160L), (1..6).map { Backoff.delaySeconds(it) })
    }

    @Test
    fun `上限は300秒`() {
        // これ以上待っても次の撮影が来るだけ。
        assertEquals(300L, Backoff.delaySeconds(7))
        assertEquals(300L, Backoff.delaySeconds(8))
    }

    @Test
    fun `回数が大きくても桁あふれせず上限に留まる`() {
        assertEquals(300L, Backoff.delaySeconds(1000))
        assertEquals(300L, Backoff.delaySeconds(Int.MAX_VALUE))
    }

    @Test
    fun `失敗していなければ待たない`() {
        assertEquals(0L, Backoff.delaySeconds(0))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UploadOrderTest" --tests "com.edgewatcher.domain.BackoffTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: UploadOrder`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/UploadOrder.kt`:

```kotlin
package com.edgewatcher.domain

/**
 * 「最新の1枚を先に送り、そのあと残りを古い順に送る」を、
 * 復帰時だけの特別扱いではなく毎回評価する規則として表現する。
 * オーナーが最初に知りたいのは「今どうなっているか」である。
 */
object UploadOrder {

    /** [all] は capturedAt 昇順であること。 */
    fun pick(all: List<ObservationMeta>): ObservationMeta? {
        val newest = all.lastOrNull() ?: return null
        return if (newest.attemptCount == 0) newest else all.first()
    }
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/Backoff.kt`:

```kotlin
package com.edgewatcher.domain

object Backoff {

    const val BASE_SECONDS = 5L
    const val MAX_SECONDS = 300L

    /** 連続失敗回数 n に対する次回試行までの待ち時間。`min(5 * 2^(n-1), 300)` 秒。 */
    fun delaySeconds(consecutiveFailures: Int): Long {
        if (consecutiveFailures <= 0) return 0L
        // 2^(n-1) を直接計算すると桁あふれするので、上限に達した時点で打ち切る。
        var delay = BASE_SECONDS
        repeat(consecutiveFailures - 1) {
            if (delay >= MAX_SECONDS) return MAX_SECONDS
            delay *= 2
        }
        return minOf(delay, MAX_SECONDS)
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UploadOrderTest" --tests "com.edgewatcher.domain.BackoffTest" --no-daemon; Pop-Location
```

期待: PASS（11件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 送信順序と指数バックオフを追加"
```

---

### Task 8: 状態モデルと、画面と通知が共有する状態1行

画面と常駐通知に同じ情報を出すことを、実装の注意深さではなく**1か所からの派生**で担保する。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/AppState.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/StatusLine.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/StatusLineTest.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `sealed interface AppState` — `Unpaired(reason: UnpairReason?)` / `Paired(run: RunState)`
  - `sealed interface RunState` — `Observing(lastUploadAt: Instant?)` / `Offline(pending: Int, lastUploadAt: Instant?)` / `Stopped`
  - `enum class UnpairReason { DISCONNECTED_BY_SERVER, LOGGED_OUT }`
  - `data class StatusText(val title: String, val detail: String)`
  - `object StatusLine { fun of(run: RunState, intervalMinutes: Int, zone: ZoneId): StatusText }`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/StatusLineTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals

class StatusLineTest {

    private val tokyo = ZoneId.of("Asia/Tokyo")

    @Test
    fun `観測中は最終送信時刻と送信間隔を出す`() {
        val text = StatusLine.of(
            run = RunState.Observing(lastUploadAt = Instant.parse("2026-09-10T05:32:05Z")),
            intervalMinutes = 5,
            zone = tokyo,
        )
        assertEquals(StatusText("観測中", "最終送信 14:32 ・ 5分間隔"), text)
    }

    @Test
    fun `一度も送信していなければ間隔だけを出す`() {
        val text = StatusLine.of(RunState.Observing(lastUploadAt = null), 10, tokyo)
        assertEquals(StatusText("観測中", "10分間隔"), text)
    }

    @Test
    fun `オフラインは待機件数を先頭に出す`() {
        // 画面を見ただけでは通信の成否が分からない。
        // オーナーが端末を見に行く動機のほとんどが「本当に送れているのか」の確認であり、
        // その答えを最初に出す。
        val text = StatusLine.of(
            run = RunState.Offline(pending = 12, lastUploadAt = Instant.parse("2026-09-10T02:48:00Z")),
            intervalMinutes = 5,
            zone = tokyo,
        )
        assertEquals(StatusText("オフライン", "12件待機中 ・ 最終送信 11:48"), text)
    }

    @Test
    fun `オフラインで一度も送信していなければ待機件数だけを出す`() {
        val text = StatusLine.of(RunState.Offline(pending = 3, lastUploadAt = null), 5, tokyo)
        assertEquals(StatusText("オフライン", "3件待機中"), text)
    }

    @Test
    fun `停止中は理由を書かず状態だけを出す`() {
        val text = StatusLine.of(RunState.Stopped, 5, tokyo)
        assertEquals(StatusText("停止中", "観測は行われていません"), text)
    }

    @Test
    fun `時刻は端末のタイムゾーンで表示する`() {
        val utc = StatusLine.of(
            RunState.Observing(Instant.parse("2026-09-10T05:32:05Z")),
            5,
            ZoneId.of("UTC"),
        )
        assertEquals("最終送信 05:32 ・ 5分間隔", utc.detail)
    }

    @Test
    fun `0時台は2桁で表示する`() {
        val text = StatusLine.of(
            RunState.Observing(Instant.parse("2026-09-09T15:05:00Z")),
            5,
            tokyo,
        )
        assertEquals("最終送信 00:05 ・ 5分間隔", text.detail)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.StatusLineTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: StatusLine`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/AppState.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

sealed interface AppState {
    /** [reason] が非 null なら、なぜ戻されたのかを画面に残す。 */
    data class Unpaired(val reason: UnpairReason?) : AppState

    data class Paired(val run: RunState) : AppState
}

sealed interface RunState {
    data class Observing(val lastUploadAt: Instant?) : RunState

    /** 未送信が1件以上あり、かつ直近の送信試行が失敗している状態。 */
    data class Offline(val pending: Int, val lastUploadAt: Instant?) : RunState

    /** サービスが動いていない。この状態のときだけ [観測を再開] を出す。 */
    data object Stopped : RunState
}

enum class UnpairReason {
    /** Web から切断された、または端末が削除された。 */
    DISCONNECTED_BY_SERVER,

    /** 端末側でログアウトした。 */
    LOGGED_OUT,
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/StatusLine.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

data class StatusText(val title: String, val detail: String)

/**
 * 画面の状態行と常駐通知の本文は、必ずここから作る。
 * 表示の一致を実装の規律ではなく、1か所からの派生で担保する。
 */
object StatusLine {

    private val HHMM: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm")

    fun of(run: RunState, intervalMinutes: Int, zone: ZoneId): StatusText = when (run) {
        is RunState.Observing -> StatusText(
            title = "観測中",
            detail = listOfNotNull(
                run.lastUploadAt?.let { "最終送信 ${format(it, zone)}" },
                "${intervalMinutes}分間隔",
            ).joinToString(" ・ "),
        )

        is RunState.Offline -> StatusText(
            title = "オフライン",
            detail = listOfNotNull(
                "${run.pending}件待機中",
                run.lastUploadAt?.let { "最終送信 ${format(it, zone)}" },
            ).joinToString(" ・ "),
        )

        RunState.Stopped -> StatusText(
            title = "停止中",
            detail = "観測は行われていません",
        )
    }

    private fun format(instant: Instant, zone: ZoneId): String =
        HHMM.format(instant.atZone(zone))
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.StatusLineTest" --no-daemon; Pop-Location
```

期待: PASS（7件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 状態モデルと状態1行の文言を追加"
```

---

### Task 9: 撮影1周期のオーケストレーション

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Ports.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/CaptureCoordinator.kt`
- Create: `android/domain/src/test/kotlin/com/edgewatcher/domain/InMemoryObservationStore.kt`
- Create: `android/domain/src/test/kotlin/com/edgewatcher/domain/DeviceFakes.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/CaptureCoordinatorTest.kt`

**Interfaces:**
- Consumes: `ImagePolicy`/`EncodeStep`（Task 3）、`Ulid`/`Clock`（Task 2）、`ObservationStore`/`ObservationMeta`/`BufferPolicy`（Task 6）、`UploadFrame`（Task 4）
- Produces:
  - `data class GeoPoint(val lat: Double, val lng: Double)`
  - `interface CaptureSource { suspend fun capture(): ByteArray? }`
  - `interface JpegEncoder { fun encode(sourceJpeg: ByteArray, step: EncodeStep): ByteArray }`
  - `interface LocationSource { fun lastKnown(): GeoPoint? }`
  - `interface AlarmScheduler { fun scheduleAt(epochMillis: Long); fun cancel() }`
  - `sealed interface CaptureOutcome` — `Stored(observationId, evicted)` / `CaptureFailed` / `TooLarge`
  - `class CaptureCoordinator(camera, encoder, location, store, clock, random)` — `suspend fun captureOnce(): CaptureOutcome`
  - `class InMemoryObservationStore : ObservationStore`（テスト専用。Task 10 でも使う）

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/InMemoryObservationStore.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

class InMemoryObservationStore : ObservationStore {

    private val rows = mutableMapOf<String, ObservationMeta>()
    private val images = mutableMapOf<String, ByteArray>()
    private val thumbs = mutableMapOf<String, ByteArray>()

    /** ファイルが消えた状況を再現するためのフック。 */
    var dropFilesFor: String? = null

    override suspend fun insert(meta: ObservationMeta, image: ByteArray, thumbnail: ByteArray) {
        rows[meta.observationId] = meta
        images[meta.observationId] = image
        thumbs[meta.observationId] = thumbnail
    }

    override suspend fun all(): List<ObservationMeta> = rows.values.sortedBy { it.capturedAt }

    override suspend fun totalBytes(): Long = rows.values.sumOf { it.sizeBytes }

    override suspend fun load(observationId: String): UploadFrame? {
        if (observationId == dropFilesFor) return null
        val meta = rows[observationId] ?: return null
        return UploadFrame(
            observationId = meta.observationId,
            capturedAt = meta.capturedAt,
            lat = meta.lat,
            lng = meta.lng,
            image = images.getValue(observationId),
            thumbnail = thumbs.getValue(observationId),
        )
    }

    override suspend fun delete(observationId: String) {
        rows.remove(observationId)
        images.remove(observationId)
        thumbs.remove(observationId)
    }

    override suspend fun recordFailure(
        observationId: String,
        attemptCount: Int,
        nextAttemptAt: Instant,
    ) {
        rows[observationId]?.let {
            rows[observationId] = it.copy(attemptCount = attemptCount, nextAttemptAt = nextAttemptAt)
        }
    }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/DeviceFakes.kt`:

```kotlin
package com.edgewatcher.domain

class FakeCaptureSource(var jpeg: ByteArray? = ByteArray(10) { 1 }) : CaptureSource {
    var captureCount = 0
        private set

    override suspend fun capture(): ByteArray? {
        captureCount++
        return jpeg
    }
}

/**
 * 段階ごとの出力サイズを指定できる符号化器。
 * 実際の画素操作は Android 側の関心なので、ここでは大きさだけを模す。
 */
class FakeJpegEncoder(
    private val sizeFor: (EncodeStep) -> Int,
) : JpegEncoder {
    val steps = mutableListOf<EncodeStep>()

    override fun encode(sourceJpeg: ByteArray, step: EncodeStep): ByteArray {
        steps += step
        return ByteArray(sizeFor(step))
    }
}

class FakeLocationSource(var point: GeoPoint? = null) : LocationSource {
    override fun lastKnown(): GeoPoint? = point
}

class RecordingAlarmScheduler : AlarmScheduler {
    val scheduled = mutableListOf<Long>()
    var cancelCount = 0
        private set

    override fun scheduleAt(epochMillis: Long) { scheduled += epochMillis }
    override fun cancel() { cancelCount++ }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/CaptureCoordinatorTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.random.Random
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CaptureCoordinatorTest {

    private val now = Instant.parse("2026-09-10T05:32:05.123Z")

    private fun coordinator(
        camera: CaptureSource = FakeCaptureSource(),
        encoder: JpegEncoder = FakeJpegEncoder { 300_000 },
        location: LocationSource = FakeLocationSource(),
        store: ObservationStore = InMemoryObservationStore(),
    ) = CaptureCoordinator(camera, encoder, location, store, FixedClock(now), Random(1))

    @Test
    fun `撮影に失敗したら何も保存しない`() = runTest {
        val store = InMemoryObservationStore()
        val outcome = coordinator(camera = FakeCaptureSource(jpeg = null), store = store).captureOnce()

        assertEquals(CaptureOutcome.CaptureFailed, outcome)
        assertEquals(emptyList(), store.all())
    }

    @Test
    fun `本画像とサムネイルを保存し、合計サイズを記録する`() = runTest {
        val store = InMemoryObservationStore()
        val encoder = FakeJpegEncoder { step -> if (step == ImagePolicy.THUMBNAIL) 8_000 else 300_000 }

        val outcome = coordinator(encoder = encoder, store = store).captureOnce()

        assertTrue(outcome is CaptureOutcome.Stored)
        val meta = store.all().single()
        assertEquals(308_000L, meta.sizeBytes)
        assertEquals(now, meta.capturedAt)
    }

    @Test
    fun `observationId のタイムスタンプ部を capturedAt に一致させる`() = runTest {
        // ずれるとサーバの日別クエリで観測が別の日に並ぶ。
        val store = InMemoryObservationStore()
        coordinator(store = store).captureOnce()

        val meta = store.all().single()
        assertEquals(meta.capturedAt, Ulid.timestampOf(meta.observationId))
    }

    @Test
    fun `最後の既知位置を付与する`() = runTest {
        val store = InMemoryObservationStore()
        val location = FakeLocationSource(GeoPoint(35.681236, 139.767125))

        coordinator(location = location, store = store).captureOnce()

        val meta = store.all().single()
        assertEquals(35.681236, meta.lat)
        assertEquals(139.767125, meta.lng)
    }

    @Test
    fun `位置が取れなくても撮影は成立する`() = runTest {
        // 測位を待たない。定点観測デバイスは動かない。
        val store = InMemoryObservationStore()
        coordinator(location = FakeLocationSource(null), store = store).captureOnce()

        val meta = store.all().single()
        assertNull(meta.lat)
        assertNull(meta.lng)
    }

    @Test
    fun `上限を超えたら品質を落として再符号化する`() = runTest {
        val store = InMemoryObservationStore()
        val encoder = FakeJpegEncoder { step ->
            when {
                step == ImagePolicy.THUMBNAIL -> 8_000
                step.quality == 80 -> 5_000_000  // 上限超過
                else -> 3_000_000                 // 品質65で収まる
            }
        }

        val outcome = coordinator(encoder = encoder, store = store).captureOnce()

        assertTrue(outcome is CaptureOutcome.Stored)
        assertEquals(
            listOf(ImagePolicy.THUMBNAIL, EncodeStep(1600, 80), EncodeStep(1600, 65)),
            encoder.steps,
        )
        assertEquals(3_008_000L, store.all().single().sizeBytes)
    }

    @Test
    fun `どの段階でも収まらなければ保存しない`() = runTest {
        // 送っても必ず 400 になるフレームをバッファに積まない。
        val store = InMemoryObservationStore()
        val encoder = FakeJpegEncoder { step -> if (step == ImagePolicy.THUMBNAIL) 8_000 else 9_000_000 }

        val outcome = coordinator(encoder = encoder, store = store).captureOnce()

        assertEquals(CaptureOutcome.TooLarge, outcome)
        assertEquals(emptyList(), store.all())
    }

    @Test
    fun `上限を超えたら古いものから削除する`() = runTest {
        val store = InMemoryObservationStore()
        // 先に 288 件を詰めておく
        repeat(288) { i ->
            store.insert(
                ObservationMeta(
                    observationId = "OLD%04d".format(i),
                    capturedAt = Instant.parse("2026-09-09T00:00:00Z").plusSeconds(i * 300L),
                    imagePath = "old$i.jpg",
                    thumbPath = "oldt$i.jpg",
                    lat = null,
                    lng = null,
                    sizeBytes = 1_000,
                ),
                ByteArray(1), ByteArray(1),
            )
        }

        val outcome = coordinator(store = store).captureOnce()

        assertEquals(listOf("OLD0000"), (outcome as CaptureOutcome.Stored).evicted)
        assertEquals(288, store.all().size)
        assertTrue(store.all().none { it.observationId == "OLD0000" })
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.CaptureCoordinatorTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: CaptureSource`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Ports.kt`:

```kotlin
package com.edgewatcher.domain

data class GeoPoint(val lat: Double, val lng: Double)

/** カメラの所有権は Foreground Service が持つ。実装はそこから呼ばれる。 */
interface CaptureSource {
    /** 1枚撮って JPEG のバイト列を返す。撮れなければ null。 */
    suspend fun capture(): ByteArray?
}

interface JpegEncoder {
    /** [sourceJpeg] を [step] の長辺・品質で符号化し直す。 */
    fun encode(sourceJpeg: ByteArray, step: EncodeStep): ByteArray
}

interface LocationSource {
    /** 最後の既知位置。**測位は待たない。** */
    fun lastKnown(): GeoPoint?
}

interface AlarmScheduler {
    fun scheduleAt(epochMillis: Long)
    fun cancel()
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/CaptureCoordinator.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.random.Random

sealed interface CaptureOutcome {
    data class Stored(val observationId: String, val evicted: List<String>) : CaptureOutcome

    /** カメラが撮れなかった。次のアラームまで待つ。 */
    data object CaptureFailed : CaptureOutcome

    /** どの段階でも上限に収まらなかった。保存しない。 */
    data object TooLarge : CaptureOutcome
}

class CaptureCoordinator(
    private val camera: CaptureSource,
    private val encoder: JpegEncoder,
    private val location: LocationSource,
    private val store: ObservationStore,
    private val clock: Clock,
    private val random: Random = Random.Default,
) {
    suspend fun captureOnce(): CaptureOutcome {
        val source = camera.capture() ?: return CaptureOutcome.CaptureFailed
        val capturedAt = clock.now()

        val thumbnail = encoder.encode(source, ImagePolicy.THUMBNAIL)

        var step: EncodeStep? = ImagePolicy.firstStep()
        var image: ByteArray? = null
        while (step != null) {
            val candidate = encoder.encode(source, step)
            if (ImagePolicy.fits(candidate.size, thumbnail.size)) {
                image = candidate
                break
            }
            step = ImagePolicy.nextStep(step)
        }
        val encoded = image ?: return CaptureOutcome.TooLarge

        val observationId = Ulid.generate(capturedAt, random)
        val point = location.lastKnown()
        val meta = ObservationMeta(
            observationId = observationId,
            capturedAt = capturedAt,
            imagePath = "$observationId.jpg",
            thumbPath = "$observationId-thumb.jpg",
            lat = point?.lat,
            lng = point?.lng,
            sizeBytes = (encoded.size + thumbnail.size).toLong(),
        )
        store.insert(meta, encoded, thumbnail)

        val evicted = BufferPolicy.evictions(store.all())
        evicted.forEach { store.delete(it) }

        return CaptureOutcome.Stored(observationId, evicted)
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.CaptureCoordinatorTest" --no-daemon; Pop-Location
```

期待: PASS（8件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 撮影1周期のオーケストレーションを追加"
```

---

### Task 10: 送信1回分のオーケストレーション

エラーの写像を1か所に閉じ込める。端末が取りうる行動は「リトライする / フレームを捨てる / セッションを取り直す / 資格情報を捨てて戻る」の4つしかない。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/UploadPump.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/UploadPumpTest.kt`

**Interfaces:**
- Consumes: `EdgeWatcherApi`/`UploadResult`（Task 4）、`SessionManager`/`AuthOutcome`（Task 5）、`ObservationStore`（Task 6）、`UploadOrder`/`Backoff`（Task 7）、`Clock`（Task 2）
- Produces:
  - `sealed interface UploadStep` — `Idle` / `Sent(observationId, intervalMinutes)` / `Discarded(observationId)` / `Deferred(observationId, retryAfterSeconds)` / `Waiting` / `Unpaired`
  - `class UploadPump(api, sessions, store, clock)` — `suspend fun sendOne(): UploadStep`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/UploadPumpTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class UploadPumpTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")
    private val creds = Credentials("DEV1", "SECRET1")

    private fun store(vararg rows: ObservationMeta) = InMemoryObservationStore().also { s ->
        kotlinx.coroutines.runBlocking {
            rows.forEach { s.insert(it, ByteArray(4), ByteArray(2)) }
        }
    }

    private fun row(
        index: Int,
        attemptCount: Int = 0,
        nextAttemptAt: Instant? = null,
    ) = ObservationMeta(
        observationId = "OBS%04d".format(index),
        capturedAt = now.minusSeconds((10 - index) * 300L),
        imagePath = "img$index.jpg",
        thumbPath = "thumb$index.jpg",
        lat = null,
        lng = null,
        sizeBytes = 300_000,
        attemptCount = attemptCount,
        nextAttemptAt = nextAttemptAt,
    )

    private fun pump(
        api: FakeApi,
        store: ObservationStore,
        credentialStore: InMemoryCredentialStore = InMemoryCredentialStore(
            credentials = creds,
            session = Session("TOKEN1", now.plusSeconds(3600)),
        ),
    ): Pair<UploadPump, InMemoryCredentialStore> {
        val clock = FixedClock(now)
        val sessions = SessionManager(api, credentialStore, clock)
        return UploadPump(api, sessions, store, clock) to credentialStore
    }

    @Test
    fun `送るものが無ければ何もしない`() = runTest {
        val api = FakeApi()
        val (p, _) = pump(api, InMemoryObservationStore())

        assertEquals(UploadStep.Idle, p.sendOne())
        assertEquals(0, api.uploadCalls.size)
    }

    @Test
    fun `成功したら削除して次の送信間隔を返す`() = runTest {
        val s = store(row(0))
        val api = FakeApi().apply { uploadResults += UploadResult.Success(intervalMinutes = 10) }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Sent("OBS0000", 10), p.sendOne())
        assertEquals(emptyList(), s.all())
    }

    @Test
    fun `素のトークンをそのまま渡す`() = runTest {
        val s = store(row(0))
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (p, _) = pump(api, s)

        p.sendOne()

        assertEquals("TOKEN1", api.uploadCalls.single().first)
    }

    @Test
    fun `400 や 413 はフレームを捨てる`() = runTest {
        // リトライしても永久に通らない。
        val s = store(row(0))
        val api = FakeApi().apply { uploadResults += UploadResult.Discard }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Discarded("OBS0000"), p.sendOne())
        assertEquals(emptyList(), s.all())
    }

    @Test
    fun `5xx はフレームを残して次回試行時刻を記録する`() = runTest {
        val s = store(row(0))
        val api = FakeApi().apply { uploadResults += UploadResult.Retryable }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Deferred("OBS0000", 5L), p.sendOne())
        val meta = s.all().single()
        assertEquals(1, meta.attemptCount)
        assertEquals(now.plusSeconds(5), meta.nextAttemptAt)
    }

    @Test
    fun `連続失敗で待ち時間が伸びる`() = runTest {
        val s = store(row(0, attemptCount = 3))
        val api = FakeApi().apply { uploadResults += UploadResult.Retryable }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Deferred("OBS0000", 40L), p.sendOne())
        assertEquals(4, s.all().single().attemptCount)
    }

    @Test
    fun `次回試行時刻より前なら送信を試みない`() = runTest {
        val s = store(row(0, attemptCount = 1, nextAttemptAt = now.plusSeconds(3)))
        val api = FakeApi()
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Waiting, p.sendOne())
        assertEquals(0, api.uploadCalls.size)
    }

    @Test
    fun `403 を受けたらセッションを取り直して再送する`() = runTest {
        // 端末が実際に受け取るのは 401 ではなく 403 である。
        val s = store(row(0))
        val api = FakeApi().apply {
            uploadResults += UploadResult.Unauthorized
            uploadResults += UploadResult.Success(5)
            tokenResults += TokenResult.Success("TOKEN2", now.plusSeconds(43_200))
        }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Sent("OBS0000", 5), p.sendOne())
        assertEquals(listOf("TOKEN1", "TOKEN2"), api.uploadCalls.map { it.first })
        assertEquals(emptyList(), s.all())
    }

    @Test
    fun `取り直しが401なら資格情報を消して未ペアリングへ戻る`() = runTest {
        val s = store(row(0))
        val api = FakeApi().apply {
            uploadResults += UploadResult.Unauthorized
            tokenResults += TokenResult.Invalid
        }
        val (p, credentialStore) = pump(api, s)

        assertEquals(UploadStep.Unpaired, p.sendOne())
        assertNull(credentialStore.readCredentials())
    }

    @Test
    fun `取り直しが5xxなら資格情報を残してフレームも残す`() = runTest {
        // ここで捨てると、圏外の端末が現地に行かないと復旧できなくなる。
        val s = store(row(0))
        val api = FakeApi().apply {
            uploadResults += UploadResult.Unauthorized
            tokenResults += TokenResult.Retryable
        }
        val (p, credentialStore) = pump(api, s)

        assertEquals(UploadStep.Deferred("OBS0000", 5L), p.sendOne())
        assertNotNull(credentialStore.readCredentials())
        assertEquals(1, s.all().size)
    }

    @Test
    fun `再送でも403なら無限に取り直さず次回へ回す`() = runTest {
        val s = store(row(0))
        val api = FakeApi().apply {
            uploadResults += UploadResult.Unauthorized
            tokenResults += TokenResult.Success("TOKEN2", now.plusSeconds(43_200))
        }
        val (p, credentialStore) = pump(api, s)

        assertEquals(UploadStep.Deferred("OBS0000", 5L), p.sendOne())
        assertEquals(2, api.uploadCalls.size)
        assertEquals(1, api.tokenCalls.size)
        assertNotNull(credentialStore.readCredentials())
    }

    @Test
    fun `画像ファイルが失われていたら行ごと捨てる`() = runTest {
        val s = store(row(0))
        s.dropFilesFor = "OBS0000"
        val api = FakeApi()
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Discarded("OBS0000"), p.sendOne())
        assertEquals(emptyList(), s.all())
        assertEquals(0, api.uploadCalls.size)
    }

    @Test
    fun `ペアリングしていなければ未ペアリングとして返す`() = runTest {
        val s = store(row(0))
        val api = FakeApi()
        val (p, _) = pump(api, s, InMemoryCredentialStore())

        assertEquals(UploadStep.Unpaired, p.sendOne())
        assertEquals(0, api.uploadCalls.size)
    }

    @Test
    fun `未試行の最新から先に送る`() = runTest {
        val s = store(row(0, attemptCount = 2), row(1, attemptCount = 2), row(2))
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (p, _) = pump(api, s)

        assertEquals(UploadStep.Sent("OBS0002", 5), p.sendOne())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UploadPumpTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: UploadPump`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/UploadPump.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

sealed interface UploadStep {
    /** 送るものが無い。 */
    data object Idle : UploadStep

    data class Sent(val observationId: String, val intervalMinutes: Int) : UploadStep

    /** リトライ不能だったので捨てた。 */
    data class Discarded(val observationId: String) : UploadStep

    /** 送信を試みて失敗し、次回時刻を記録した。 */
    data class Deferred(val observationId: String, val retryAfterSeconds: Long) : UploadStep

    /** 何もしなかった。次回試行時刻前、またはセッションが一時的に取れない。 */
    data object Waiting : UploadStep

    /** 資格情報が失われた。未ペアリング画面へ戻る。 */
    data object Unpaired : UploadStep
}

class UploadPump(
    private val api: EdgeWatcherApi,
    private val sessions: SessionManager,
    private val store: ObservationStore,
    private val clock: Clock,
) {
    suspend fun sendOne(): UploadStep {
        val candidate = UploadOrder.pick(store.all()) ?: return UploadStep.Idle
        val now = clock.now()

        candidate.nextAttemptAt?.let { if (now.isBefore(it)) return UploadStep.Waiting }

        val token = when (val auth = sessions.currentToken()) {
            is AuthOutcome.Ready -> auth.sessionToken
            AuthOutcome.CredentialsInvalid, AuthOutcome.NotPaired -> return UploadStep.Unpaired
            AuthOutcome.Retryable -> return UploadStep.Waiting
        }

        val frame = store.load(candidate.observationId)
        if (frame == null) {
            // 行はあるのに画像が無い。再送しても永久に成立しないので捨てる。
            store.delete(candidate.observationId)
            return UploadStep.Discarded(candidate.observationId)
        }

        return when (val result = api.upload(token, frame)) {
            is UploadResult.Success -> {
                store.delete(candidate.observationId)
                UploadStep.Sent(candidate.observationId, result.intervalMinutes)
            }
            UploadResult.Discard -> {
                store.delete(candidate.observationId)
                UploadStep.Discarded(candidate.observationId)
            }
            UploadResult.Retryable -> defer(candidate, now)
            UploadResult.Unauthorized -> when (val auth = sessions.refresh()) {
                is AuthOutcome.Ready -> retryOnce(auth.sessionToken, candidate, frame, now)
                AuthOutcome.CredentialsInvalid, AuthOutcome.NotPaired -> UploadStep.Unpaired
                AuthOutcome.Retryable -> defer(candidate, now)
            }
        }
    }

    /** 取り直した直後の1回だけ再送する。ここで再び 403 でも修復に戻らない（無限ループを避ける）。 */
    private suspend fun retryOnce(
        token: String,
        candidate: ObservationMeta,
        frame: UploadFrame,
        now: Instant,
    ): UploadStep = when (val result = api.upload(token, frame)) {
        is UploadResult.Success -> {
            store.delete(candidate.observationId)
            UploadStep.Sent(candidate.observationId, result.intervalMinutes)
        }
        UploadResult.Discard -> {
            store.delete(candidate.observationId)
            UploadStep.Discarded(candidate.observationId)
        }
        UploadResult.Unauthorized, UploadResult.Retryable -> defer(candidate, now)
    }

    private suspend fun defer(candidate: ObservationMeta, now: Instant): UploadStep {
        val attempts = candidate.attemptCount + 1
        val delay = Backoff.delaySeconds(attempts)
        store.recordFailure(candidate.observationId, attempts, now.plusSeconds(delay))
        return UploadStep.Deferred(candidate.observationId, delay)
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.UploadPumpTest" --no-daemon; Pop-Location
```

期待: PASS（14件）。

- [ ] **Step 5: Commit**

```bash
git add android/domain
git commit -m "feat(domain): 送信1回分のオーケストレーションとエラー写像を追加"
```

---

### Task 11: 送信間隔の適用と、次回撮影時刻・状態の導出

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/ObservationEngine.kt`
- Modify: `android/domain/src/main/kotlin/com/edgewatcher/domain/Ports.kt`（`SettingsStore` を追加）
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/ObservationEngineTest.kt`

**Interfaces:**
- Consumes: `RunState`（Task 8）
- Produces:
  - `interface SettingsStore { fun intervalMinutes(): Int; fun setIntervalMinutes(minutes: Int) }`
  - `object ObservationEngine` — `DEFAULT_INTERVAL_MINUTES = 5`, `ALLOWED_INTERVAL_MINUTES = setOf(5, 10, 15)`, `applyNextConfig(current, received)`, `nextCaptureAt(completedAt, intervalMinutes)`, `deriveRunState(pending, lastUploadAt, lastAttemptFailed)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/ObservationEngineTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals

class ObservationEngineTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")

    @Test
    fun `既定の送信間隔は5分`() {
        // バッファ上限の288件が「5分間隔で24時間相当」として決まっているため、
        // 初期値を5分に置くと最も負荷の高い条件が既定になる。
        assertEquals(5, ObservationEngine.DEFAULT_INTERVAL_MINUTES)
    }

    @Test
    fun `サーバが配ってきた値を適用する`() {
        assertEquals(10, ObservationEngine.applyNextConfig(current = 5, received = 10))
        assertEquals(15, ObservationEngine.applyNextConfig(current = 5, received = 15))
    }

    @Test
    fun `想定外の値は無視して現状を保つ`() {
        // サーバとクライアントで選択肢がずれたときに、
        // 端末が異常な間隔で動き出すことを防ぐ。
        assertEquals(5, ObservationEngine.applyNextConfig(current = 5, received = 7))
        assertEquals(5, ObservationEngine.applyNextConfig(current = 5, received = 0))
        assertEquals(10, ObservationEngine.applyNextConfig(current = 10, received = -1))
    }

    @Test
    fun `次回撮影時刻は撮影周期の完了時点から間隔ぶん後`() {
        // 等間隔を守ろうとすると、撮影が詰まったときに連射になる。
        assertEquals(now.plusSeconds(300), ObservationEngine.nextCaptureAt(now, 5))
        assertEquals(now.plusSeconds(900), ObservationEngine.nextCaptureAt(now, 15))
    }

    @Test
    fun `未送信が無ければ観測中`() {
        assertEquals(
            RunState.Observing(now),
            ObservationEngine.deriveRunState(pending = 0, lastUploadAt = now, lastAttemptFailed = false),
        )
    }

    @Test
    fun `未送信があり直近の試行が失敗していればオフライン`() {
        assertEquals(
            RunState.Offline(pending = 12, lastUploadAt = now),
            ObservationEngine.deriveRunState(pending = 12, lastUploadAt = now, lastAttemptFailed = true),
        )
    }

    @Test
    fun `未送信があっても失敗していなければ観測中のまま`() {
        // 撮ったばかりでこれから送る、という状態をオフラインと呼ばない。
        assertEquals(
            RunState.Observing(now),
            ObservationEngine.deriveRunState(pending = 1, lastUploadAt = now, lastAttemptFailed = false),
        )
    }

    @Test
    fun `未送信が無ければ失敗フラグが立っていても観測中`() {
        assertEquals(
            RunState.Observing(now),
            ObservationEngine.deriveRunState(pending = 0, lastUploadAt = now, lastAttemptFailed = true),
        )
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ObservationEngineTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: ObservationEngine`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Ports.kt` の末尾に追記:

```kotlin
interface SettingsStore {
    /** 現在の送信間隔（分）。まだ受け取っていなければ既定値。 */
    fun intervalMinutes(): Int

    fun setIntervalMinutes(minutes: Int)
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/ObservationEngine.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

object ObservationEngine {

    const val DEFAULT_INTERVAL_MINUTES = 5

    val ALLOWED_INTERVAL_MINUTES = setOf(5, 10, 15)

    /**
     * サーバは設定を push しない。端末はアップロードの応答から受け取って適用する。
     * 選択肢の外の値は無視する。サーバとクライアントで一覧がずれたときに、
     * 端末が異常な間隔で動き出すことを避けるため。
     */
    fun applyNextConfig(current: Int, received: Int): Int =
        if (received in ALLOWED_INTERVAL_MINUTES) received else current

    /** 撮影周期の完了時点から数える。ドリフトの補正はしない。 */
    fun nextCaptureAt(completedAt: Instant, intervalMinutes: Int): Instant =
        completedAt.plusSeconds(intervalMinutes * 60L)

    fun deriveRunState(pending: Int, lastUploadAt: Instant?, lastAttemptFailed: Boolean): RunState =
        if (pending > 0 && lastAttemptFailed) {
            RunState.Offline(pending = pending, lastUploadAt = lastUploadAt)
        } else {
            RunState.Observing(lastUploadAt = lastUploadAt)
        }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ObservationEngineTest" --no-daemon; Pop-Location
```

期待: PASS（8件）。

- [ ] **Step 5: フェーズ1 全体が緑であることを確認して Commit**

```powershell
Push-Location android; .\gradlew.bat :domain:test --no-daemon; Pop-Location
```

期待: 全テスト PASS（合計 82件）。

```bash
git add android/domain
git commit -m "feat(domain): 送信間隔の適用と状態導出を追加"
```

---

## フェーズ2: API の実装と偽バックエンド

**このフェーズ以降の `:app` のテスト実行コマンド:**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --no-daemon; Pop-Location
```

**必ず `mock` バリアントで走らせる。** `live` は `EW_API_BASE_URL` を要求するため、テストのたびに接続先を用意する羽目になる。

### Task 12: ペアリングとトークン取得の HTTP 実装

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/net/Dto.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/net/HttpEdgeWatcherApi.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/net/HttpPairAndTokenTest.kt`

**Interfaces:**
- Consumes: `EdgeWatcherApi` とその結果型（Task 4）
- Produces: `class HttpEdgeWatcherApi(baseUrl: String, client: OkHttpClient, json: Json) : EdgeWatcherApi` — この Task では `pair` と `token` のみ実装し、`upload` / `logout` は Task 13 で埋める

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/net/HttpPairAndTokenTest.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.DeviceInfo
import com.edgewatcher.domain.PairResult
import com.edgewatcher.domain.TokenResult
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals

class HttpPairAndTokenTest {

    private lateinit var server: MockWebServer
    private lateinit var api: HttpEdgeWatcherApi

    private val info = DeviceInfo(model = "Pixel 6a", osVersion = "16", appVersion = "1.0.0")

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        api = HttpEdgeWatcherApi(
            baseUrl = server.url("/").toString().removeSuffix("/"),
            client = OkHttpClient(),
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun respond(code: Int, body: String = "") {
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))
    }

    @Test
    fun `pair は 200 を Success に写す`() = runTest {
        respond(200, """{"deviceId":"DEV1","deviceSecret":"SECRET1"}""")

        val result = api.pair("CODE1", info)

        assertEquals(PairResult.Success("DEV1", "SECRET1"), result)
    }

    @Test
    fun `pair は契約どおりのパスと本文を送る`() = runTest {
        respond(200, """{"deviceId":"DEV1","deviceSecret":"SECRET1"}""")

        api.pair("CODE1", info)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/device/pair", request.path)

        val body = Json.parseToJsonElement(request.body.readUtf8()).jsonObject
        assertEquals("CODE1", body["pairingCode"]?.jsonPrimitive?.content)
        val deviceInfo = body["deviceInfo"]!!.jsonObject
        assertEquals("Pixel 6a", deviceInfo["model"]?.jsonPrimitive?.content)
        assertEquals("16", deviceInfo["osVersion"]?.jsonPrimitive?.content)
        assertEquals("1.0.0", deviceInfo["appVersion"]?.jsonPrimitive?.content)
    }

    @Test
    fun `pair は 404 だけをリトライ可能として区別する`() = runTest {
        respond(404, """{"error":{"code":"PAIRING_NOT_FOUND","message":"見つかりません"}}""")
        assertEquals(PairResult.NotFound, api.pair("CODE1", info))
    }

    @Test
    fun `pair は 409 を使用不能として写す`() = runTest {
        respond(409, """{"error":{"code":"PAIRING_CODE_CONSUMED","message":"使用済みです"}}""")
        assertEquals(PairResult.Unusable, api.pair("CODE1", info))
    }

    @Test
    fun `pair は 400 を拒否として写す`() = runTest {
        respond(400, """{"error":{"code":"VALIDATION_ERROR","message":"不正です"}}""")
        assertEquals(PairResult.Rejected, api.pair("CODE1", info))
    }

    @Test
    fun `pair は 5xx を到達不能として写す`() = runTest {
        respond(500, """{"error":{"code":"INTERNAL_ERROR","message":"障害"}}""")
        assertEquals(PairResult.Unreachable, api.pair("CODE1", info))
    }

    @Test
    fun `pair はネットワーク断を到達不能として写す`() = runTest {
        server.shutdown()
        assertEquals(PairResult.Unreachable, api.pair("CODE1", info))
    }

    @Test
    fun `token は 200 を Success に写し、失効時刻を秒から復元する`() = runTest {
        respond(200, """{"sessionToken":"DEV1.ABC","expiresAt":1789000000}""")

        val result = api.token("DEV1", "SECRET1")

        assertEquals(
            TokenResult.Success("DEV1.ABC", Instant.ofEpochSecond(1789000000L)),
            result,
        )
    }

    @Test
    fun `token は 401 だけを Invalid に写す`() = runTest {
        // ここだけが資格情報を捨ててよい合図。
        respond(401, """{"error":{"code":"UNAUTHORIZED","message":"無効です"}}""")
        assertEquals(TokenResult.Invalid, api.token("DEV1", "SECRET1"))
    }

    @Test
    fun `token は 5xx を Retryable に写す`() = runTest {
        respond(503, "")
        assertEquals(TokenResult.Retryable, api.token("DEV1", "SECRET1"))
    }

    @Test
    fun `token は 400 も Retryable に写す`() = runTest {
        // 400 で資格情報を捨てると、屋外の端末が現地に行かないと復旧できなくなる。
        // 実際には起こらない経路だが、捨てる側に倒さない。
        respond(400, """{"error":{"code":"VALIDATION_ERROR","message":"不正です"}}""")
        assertEquals(TokenResult.Retryable, api.token("DEV1", "SECRET1"))
    }

    @Test
    fun `token はネットワーク断を Retryable に写す`() = runTest {
        server.shutdown()
        assertEquals(TokenResult.Retryable, api.token("DEV1", "SECRET1"))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.net.HttpPairAndTokenTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: HttpEdgeWatcherApi`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/net/Dto.kt`:

```kotlin
package com.edgewatcher.app.net

import kotlinx.serialization.Serializable

@Serializable
internal data class DeviceInfoDto(
    val model: String,
    val osVersion: String,
    val appVersion: String,
)

@Serializable
internal data class PairRequestDto(
    val pairingCode: String,
    val deviceInfo: DeviceInfoDto,
)

@Serializable
internal data class PairResponseDto(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
internal data class TokenRequestDto(
    val deviceId: String,
    val deviceSecret: String,
)

@Serializable
internal data class TokenResponseDto(
    val sessionToken: String,
    val expiresAt: Long,
)

@Serializable
internal data class NextConfigDto(val intervalMinutes: Int)

@Serializable
internal data class UploadResponseDto(val nextConfig: NextConfigDto)

@Serializable
internal data class UploadMetadataDto(
    val observationId: String,
    val capturedAt: String,
    val lat: Double? = null,
    val lng: Double? = null,
)
```

`android/app/src/main/kotlin/com/edgewatcher/app/net/HttpEdgeWatcherApi.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.DeviceInfo
import com.edgewatcher.domain.EdgeWatcherApi
import com.edgewatcher.domain.LogoutResult
import com.edgewatcher.domain.PairResult
import com.edgewatcher.domain.TokenResult
import com.edgewatcher.domain.UploadFrame
import com.edgewatcher.domain.UploadResult
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.time.Instant

/**
 * `docs/engineering/api.yml` の端末群4本。
 * **HTTP のステータスをドメインへ漏らさない**のがこのクラスの仕事で、
 * 返すのは「捨ててよいか / 待つべきか / 取り直すべきか」だけ。
 */
class HttpEdgeWatcherApi(
    private val baseUrl: String,
    private val client: OkHttpClient,
    private val json: Json = Json { ignoreUnknownKeys = true },
) : EdgeWatcherApi {

    private val jsonMedia = "application/json".toMediaType()

    override suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): PairResult =
        withContext(Dispatchers.IO) {
            val body = json.encodeToString(
                PairRequestDto(
                    pairingCode = pairingCode,
                    deviceInfo = DeviceInfoDto(
                        model = deviceInfo.model,
                        osVersion = deviceInfo.osVersion,
                        appVersion = deviceInfo.appVersion,
                    ),
                )
            )
            val request = Request.Builder()
                .url("$baseUrl/device/pair")
                .post(body.toRequestBody(jsonMedia))
                .build()

            try {
                client.newCall(request).execute().use { response ->
                    when (response.code) {
                        200 -> {
                            val dto = json.decodeFromString<PairResponseDto>(bodyOf(response))
                            PairResult.Success(dto.deviceId, dto.deviceSecret)
                        }
                        // GSI2 は結果整合。待てば成功しうる唯一の失敗。
                        404 -> PairResult.NotFound
                        409 -> PairResult.Unusable
                        400 -> PairResult.Rejected
                        else -> PairResult.Unreachable
                    }
                }
            } catch (e: IOException) {
                PairResult.Unreachable
            }
        }

    override suspend fun token(deviceId: String, deviceSecret: String): TokenResult =
        withContext(Dispatchers.IO) {
            val body = json.encodeToString(TokenRequestDto(deviceId, deviceSecret))
            val request = Request.Builder()
                .url("$baseUrl/device/token")
                .post(body.toRequestBody(jsonMedia))
                .build()

            try {
                client.newCall(request).execute().use { response ->
                    when (response.code) {
                        200 -> {
                            val dto = json.decodeFromString<TokenResponseDto>(bodyOf(response))
                            TokenResult.Success(
                                sessionToken = dto.sessionToken,
                                expiresAt = Instant.ofEpochSecond(dto.expiresAt),
                            )
                        }
                        // 401 だけが「資格情報が無効」の断言。それ以外で捨ててはならない。
                        401 -> TokenResult.Invalid
                        else -> TokenResult.Retryable
                    }
                }
            } catch (e: IOException) {
                TokenResult.Retryable
            }
        }

    override suspend fun upload(sessionToken: String, frame: UploadFrame): UploadResult =
        TODO("Task 13 で実装する")

    override suspend fun logout(sessionToken: String): LogoutResult =
        TODO("Task 13 で実装する")

    private fun bodyOf(response: Response): String = response.body?.string().orEmpty()
}
```

`TODO(...)` を2つ残すが、これは**次の Task で必ず埋める前提の足場**であり、計画上のプレースホルダではない。Task 13 の完了条件がこの2つの除去である。

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.net.HttpPairAndTokenTest" --no-daemon; Pop-Location
```

期待: PASS（12件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): ペアリングとトークン取得の HTTP 実装を追加"
```

---

### Task 13: アップロードとログアウトの HTTP 実装

**Files:**
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/net/HttpEdgeWatcherApi.kt`（`upload` と `logout` の `TODO` を置き換える）
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/net/HttpUploadAndLogoutTest.kt`

**Interfaces:**
- Consumes: `HttpEdgeWatcherApi`（Task 12）、`UploadFrame`/`UploadResult`/`LogoutResult`（Task 4）
- Produces: `HttpEdgeWatcherApi` の全メソッドが実装済みになる

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/net/HttpUploadAndLogoutTest.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.LogoutResult
import com.edgewatcher.domain.UploadFrame
import com.edgewatcher.domain.UploadResult
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class HttpUploadAndLogoutTest {

    private lateinit var server: MockWebServer
    private lateinit var api: HttpEdgeWatcherApi

    private val frame = UploadFrame(
        observationId = "01J8Z3K2QW7M9V0N4P5R6T7Y8B",
        capturedAt = Instant.parse("2026-09-10T05:32:05.123Z"),
        lat = 35.681236,
        lng = 139.767125,
        image = ByteArray(16) { 7 },
        thumbnail = ByteArray(8) { 3 },
    )

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        api = HttpEdgeWatcherApi(
            baseUrl = server.url("/").toString().removeSuffix("/"),
            client = OkHttpClient(),
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun respond(code: Int, body: String = "") {
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))
    }

    @Test
    fun `upload は 200 の nextConfig を返す`() = runTest {
        respond(200, """{"nextConfig":{"intervalMinutes":10}}""")

        assertEquals(UploadResult.Success(10), api.upload("DEV1.ABC", frame))
    }

    @Test
    fun `upload は Bearer を付けない素のトークンを送る`() = runTest {
        respond(200, """{"nextConfig":{"intervalMinutes":5}}""")

        api.upload("DEV1.ABC", frame)

        assertEquals("DEV1.ABC", server.takeRequest().getHeader("Authorization"))
    }

    @Test
    fun `upload は契約どおりのパートを multipart で送る`() = runTest {
        respond(200, """{"nextConfig":{"intervalMinutes":5}}""")

        api.upload("DEV1.ABC", frame)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/device/uploads", request.path)
        assertTrue(request.getHeader("Content-Type")!!.startsWith("multipart/form-data"))

        val body = request.body.readUtf8()
        assertTrue(body.contains("""name="image""""), "image パートが無い")
        assertTrue(body.contains("""name="thumbnail""""), "thumbnail パートが無い")
        assertTrue(body.contains("""name="metadata""""), "metadata パートが無い")
        assertTrue(body.contains("image/jpeg"), "JPEG の Content-Type が無い")
        assertTrue(body.contains("application/json"), "metadata の Content-Type が無い")
    }

    @Test
    fun `upload の metadata は observationId と capturedAt と位置を含む`() = runTest {
        respond(200, """{"nextConfig":{"intervalMinutes":5}}""")

        api.upload("DEV1.ABC", frame)

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains(""""observationId":"01J8Z3K2QW7M9V0N4P5R6T7Y8B""""))
        assertTrue(body.contains(""""capturedAt":"2026-09-10T05:32:05.123Z""""))
        assertTrue(body.contains(""""lat":35.681236"""))
        assertTrue(body.contains(""""lng":139.767125"""))
    }

    @Test
    fun `upload の metadata に deviceId は含めない`() = runTest {
        // 端末の同一性はオーソライザだけが決める。送ってもサーバは読まない。
        respond(200, """{"nextConfig":{"intervalMinutes":5}}""")

        api.upload("DEV1.ABC", frame)

        assertTrue(!server.takeRequest().body.readUtf8().contains("deviceId"))
    }

    @Test
    fun `位置が無ければ lat と lng を省く`() = runTest {
        respond(200, """{"nextConfig":{"intervalMinutes":5}}""")

        api.upload("DEV1.ABC", UploadFrame(frame.observationId, frame.capturedAt, null, null, frame.image, frame.thumbnail))

        val body = server.takeRequest().body.readUtf8()
        assertTrue(!body.contains("\"lat\""), "lat を送ってはならない")
        assertTrue(!body.contains("\"lng\""), "lng を送ってはならない")
    }

    @Test
    fun `upload は 400 を破棄に写す`() = runTest {
        respond(400, """{"error":{"code":"VALIDATION_ERROR","message":"不正です"}}""")
        assertEquals(UploadResult.Discard, api.upload("DEV1.ABC", frame))
    }

    @Test
    fun `upload は 413 も破棄に写す`() = runTest {
        // どちらもリトライしても永久に通らない。
        respond(413, "")
        assertEquals(UploadResult.Discard, api.upload("DEV1.ABC", frame))
    }

    @Test
    fun `upload は 401 と 403 の両方をセッション切れに写す`() = runTest {
        respond(401, """{"message":"Unauthorized"}""")
        assertEquals(UploadResult.Unauthorized, api.upload("DEV1.ABC", frame))

        respond(403, """{"message":"Forbidden"}""")
        assertEquals(UploadResult.Unauthorized, api.upload("DEV1.ABC", frame))
    }

    @Test
    fun `upload は 5xx とネットワーク断を Retryable に写す`() = runTest {
        respond(500, "")
        assertEquals(UploadResult.Retryable, api.upload("DEV1.ABC", frame))

        server.shutdown()
        assertEquals(UploadResult.Retryable, api.upload("DEV1.ABC", frame))
    }

    @Test
    fun `logout は 204 を成功に写し、本文を送らない`() = runTest {
        respond(204, "")

        assertEquals(LogoutResult.Success, api.logout("DEV1.ABC"))

        val request = server.takeRequest()
        assertEquals("/device/logout", request.path)
        assertEquals("DEV1.ABC", request.getHeader("Authorization"))
        assertEquals(0L, request.bodySize)
    }

    @Test
    fun `logout は 403 をセッション切れに写す`() = runTest {
        respond(403, """{"message":"Forbidden"}""")
        assertEquals(LogoutResult.Unauthorized, api.logout("DEV1.ABC"))
    }

    @Test
    fun `logout は 5xx を Retryable に写す`() = runTest {
        respond(500, "")
        assertEquals(LogoutResult.Retryable, api.logout("DEV1.ABC"))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.net.HttpUploadAndLogoutTest" --no-daemon; Pop-Location
```

期待: `NotImplementedError`（`TODO("Task 13 で実装する")`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`HttpEdgeWatcherApi.kt` の `upload` と `logout` を置き換える。あわせて import を追加する。

```kotlin
// 追加する import
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import java.time.format.DateTimeFormatter
```

```kotlin
    override suspend fun upload(sessionToken: String, frame: UploadFrame): UploadResult =
        withContext(Dispatchers.IO) {
            val metadata = json.encodeToString(
                UploadMetadataDto(
                    observationId = frame.observationId,
                    capturedAt = DateTimeFormatter.ISO_INSTANT.format(frame.capturedAt),
                    lat = frame.lat,
                    lng = frame.lng,
                )
            )
            val body = MultipartBody.Builder()
                .setType(MultipartBody.FORM)
                .addFormDataPart("image", "image.jpg", frame.image.toRequestBody(jpegMedia))
                .addFormDataPart("thumbnail", "thumb.jpg", frame.thumbnail.toRequestBody(jpegMedia))
                .addFormDataPart("metadata", null, metadata.toRequestBody(jsonMedia))
                .build()

            val request = Request.Builder()
                .url("$baseUrl/device/uploads")
                // 契約は Bearer を付けない素のトークン。
                .header("Authorization", sessionToken)
                .post(body)
                .build()

            try {
                client.newCall(request).execute().use { response ->
                    when (response.code) {
                        200 -> {
                            val dto = json.decodeFromString<UploadResponseDto>(bodyOf(response))
                            UploadResult.Success(dto.nextConfig.intervalMinutes)
                        }
                        // どちらも再送しても永久に通らない。バッファから捨てる。
                        400, 413 -> UploadResult.Discard
                        // オーソライザが拒否すると 403。ヘッダが無いだけなら 401。
                        401, 403 -> UploadResult.Unauthorized
                        else -> UploadResult.Retryable
                    }
                }
            } catch (e: IOException) {
                UploadResult.Retryable
            }
        }

    override suspend fun logout(sessionToken: String): LogoutResult =
        withContext(Dispatchers.IO) {
            val request = Request.Builder()
                .url("$baseUrl/device/logout")
                .header("Authorization", sessionToken)
                .post(ByteArray(0).toRequestBody(null))
                .build()

            try {
                client.newCall(request).execute().use { response ->
                    when (response.code) {
                        204, 200 -> LogoutResult.Success
                        401, 403 -> LogoutResult.Unauthorized
                        else -> LogoutResult.Retryable
                    }
                }
            } catch (e: IOException) {
                LogoutResult.Retryable
            }
        }
```

クラスの先頭のフィールドに JPEG のメディア型を足す:

```kotlin
    private val jpegMedia = "image/jpeg".toMediaType()
```

`UploadMetadataDto` の `lat` / `lng` は既定値 `null` なので、**位置が無いときはフィールドごと出力されない**（kotlinx.serialization は既定で既定値を省略する）。契約上 `lat` / `lng` は任意項目なので、これで正しい。

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.net.HttpUploadAndLogoutTest" --no-daemon; Pop-Location
```

期待: PASS（13件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): アップロードとログアウトの HTTP 実装を追加"
```

---

### Task 14: mock フレーバーの偽バックエンドと API の切り替え

**Files:**
- Create: `android/app/src/mock/kotlin/com/edgewatcher/app/net/FakeEdgeWatcherApi.kt`
- Create: `android/app/src/mock/kotlin/com/edgewatcher/app/net/ApiFactory.kt`
- Create: `android/app/src/live/kotlin/com/edgewatcher/app/net/ApiFactory.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/net/FakeEdgeWatcherApiTest.kt`

**Interfaces:**
- Consumes: `EdgeWatcherApi`（Task 4）、`HttpEdgeWatcherApi`（Task 13）
- Produces:
  - `fun createEdgeWatcherApi(): EdgeWatcherApi`（両フレーバーに同名で存在する）
  - `class FakeEdgeWatcherApi : EdgeWatcherApi`（mock フレーバーにのみ存在する）

**注意**: このテストは `mock` フレーバーのソースセットを参照するため、`live` バリアントでは実行されない。これは意図した挙動である。

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/net/FakeEdgeWatcherApiTest.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.DeviceInfo
import com.edgewatcher.domain.LogoutResult
import com.edgewatcher.domain.PairResult
import com.edgewatcher.domain.TokenResult
import com.edgewatcher.domain.UploadFrame
import com.edgewatcher.domain.UploadResult
import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class FakeEdgeWatcherApiTest {

    private val info = DeviceInfo("Pixel 6a", "16", "1.0.0")

    private fun frame(id: String) = UploadFrame(
        observationId = id,
        capturedAt = Instant.parse("2026-09-10T05:32:05Z"),
        lat = null,
        lng = null,
        image = ByteArray(4),
        thumbnail = ByteArray(2),
    )

    @Test
    fun `どのコードでもペアリングは成立する`() = runTest {
        // モックは常に成功する。確率的な失敗は入れない。
        // 再現しない失敗はデバッグの役に立たない。
        val api = FakeEdgeWatcherApi()

        val result = api.pair("PAIR_ANYTHING", info)

        assertTrue(result is PairResult.Success)
    }

    @Test
    fun `ペアリングのたびに違う deviceSecret を発行する`() = runTest {
        val api = FakeEdgeWatcherApi()
        val first = api.pair("A", info) as PairResult.Success
        val second = api.pair("B", info) as PairResult.Success

        assertTrue(first.deviceSecret != second.deviceSecret)
    }

    @Test
    fun `トークンは deviceId を前半に持つ形で発行される`() = runTest {
        val api = FakeEdgeWatcherApi()

        val result = api.token("DEV1", "SECRET1") as TokenResult.Success

        assertTrue(result.sessionToken.startsWith("DEV1."))
    }

    @Test
    fun `アップロードは常に成功し、既定の送信間隔を返す`() = runTest {
        val api = FakeEdgeWatcherApi()

        assertEquals(UploadResult.Success(5), api.upload("DEV1.ABC", frame("OBS1")))
    }

    @Test
    fun `受け取ったフレームを記録する`() = runTest {
        // 実機で「本当に送られたか」を確認する手掛かりにする。
        val api = FakeEdgeWatcherApi()
        api.upload("DEV1.ABC", frame("OBS1"))
        api.upload("DEV1.ABC", frame("OBS2"))

        assertEquals(listOf("OBS1", "OBS2"), api.received)
    }

    @Test
    fun `ログアウトは成功する`() = runTest {
        assertEquals(LogoutResult.Success, FakeEdgeWatcherApi().logout("DEV1.ABC"))
    }

    @Test
    fun `切断された後はトークンを発行しない`() = runTest {
        // Web からの切断を実機で再現するためのフック。
        val api = FakeEdgeWatcherApi()
        api.logout("DEV1.ABC")

        assertEquals(TokenResult.Invalid, api.token("DEV1", "SECRET1"))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.net.FakeEdgeWatcherApiTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: FakeEdgeWatcherApi`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/mock/kotlin/com/edgewatcher/app/net/FakeEdgeWatcherApi.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.DeviceInfo
import com.edgewatcher.domain.EdgeWatcherApi
import com.edgewatcher.domain.LogoutResult
import com.edgewatcher.domain.ObservationEngine
import com.edgewatcher.domain.PairResult
import com.edgewatcher.domain.TokenResult
import com.edgewatcher.domain.UploadFrame
import com.edgewatcher.domain.UploadResult
import java.time.Instant
import java.util.concurrent.atomic.AtomicInteger

/**
 * mock フレーバーにのみ存在する偽バックエンド。ネットワークには一切出ない。
 * live ビルドにはコンパイルすらされないので、本番 APK に混入しない。
 *
 * **常に成功する。** 確率的な失敗は入れない。再現しない失敗はデバッグの役に立たない。
 * オフライン時の挙動は live フレーバーで実機を機内モードにして確認する。
 */
class FakeEdgeWatcherApi : EdgeWatcherApi {

    private val counter = AtomicInteger(0)
    private var disconnected = false

    /** 受け取った observationId。実機で送信の到達を確認する手掛かり。 */
    val received = mutableListOf<String>()

    override suspend fun pair(pairingCode: String, deviceInfo: DeviceInfo): PairResult {
        disconnected = false
        val n = counter.incrementAndGet()
        return PairResult.Success(
            deviceId = "MOCKDEVICE%08d".format(n),
            deviceSecret = "MOCKSECRET%08d".format(n),
        )
    }

    override suspend fun token(deviceId: String, deviceSecret: String): TokenResult =
        if (disconnected) {
            TokenResult.Invalid
        } else {
            TokenResult.Success(
                sessionToken = "$deviceId.MOCKSESSION",
                expiresAt = Instant.now().plusSeconds(43_200),
            )
        }

    override suspend fun upload(sessionToken: String, frame: UploadFrame): UploadResult {
        received += frame.observationId
        return UploadResult.Success(ObservationEngine.DEFAULT_INTERVAL_MINUTES)
    }

    override suspend fun logout(sessionToken: String): LogoutResult {
        disconnected = true
        return LogoutResult.Success
    }
}
```

`android/app/src/mock/kotlin/com/edgewatcher/app/net/ApiFactory.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.domain.EdgeWatcherApi

fun createEdgeWatcherApi(): EdgeWatcherApi = FakeEdgeWatcherApi()
```

`android/app/src/live/kotlin/com/edgewatcher/app/net/ApiFactory.kt`:

```kotlin
package com.edgewatcher.app.net

import com.edgewatcher.app.BuildConfig
import com.edgewatcher.domain.EdgeWatcherApi
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

fun createEdgeWatcherApi(): EdgeWatcherApi = HttpEdgeWatcherApi(
    baseUrl = BuildConfig.API_BASE_URL,
    client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        // 画像2枚を送るので書き込みは長めに取る。
        .writeTimeout(60, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build(),
)
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --no-daemon; Pop-Location
```

期待: PASS（フェーズ2 の合計 32件）。

- [ ] **Step 5: live ビルドが通ることも確認する**

```powershell
$env:EW_API_BASE_URL = "https://example.invalid"
Push-Location android; .\gradlew.bat :app:assembleLiveDebug --no-daemon; Pop-Location
Remove-Item Env:\EW_API_BASE_URL
```

期待: `BUILD SUCCESSFUL`。`live` 側の `ApiFactory` が `HttpEdgeWatcherApi` を返す形でコンパイルできている。

- [ ] **Step 6: Commit**

```bash
git add android/app
git commit -m "feat(app): mock フレーバーの偽バックエンドと API の切り替えを追加"
```

---

## フェーズ3: 永続化

Robolectric を使う。実機やエミュレータは不要で、JVM 上で Android フレームワークを模して走る。

### Task 15: Room によるバッファの実装

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/data/ObservationDb.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/data/RoomObservationStore.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/data/RoomObservationStoreTest.kt`

**Interfaces:**
- Consumes: `ObservationStore` / `ObservationMeta` / `UploadFrame`（Task 4・6）
- Produces:
  - `@Entity data class ObservationRow(...)`
  - `@Dao interface ObservationDao`
  - `@Database abstract class ObservationDb : RoomDatabase()`
  - `class RoomObservationStore(dao: ObservationDao, imagesDir: File) : ObservationStore`

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/data/RoomObservationStoreTest.kt`:

```kotlin
package com.edgewatcher.app.data

import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.edgewatcher.domain.ObservationMeta
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import java.io.File
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

@RunWith(RobolectricTestRunner::class)
class RoomObservationStoreTest {

    private lateinit var db: ObservationDb
    private lateinit var imagesDir: File
    private lateinit var store: RoomObservationStore

    private val base = Instant.parse("2026-09-10T00:00:00Z")

    @Before
    fun setUp() {
        val context = ApplicationProvider.getApplicationContext<android.content.Context>()
        db = Room.inMemoryDatabaseBuilder(context, ObservationDb::class.java).build()
        imagesDir = File(context.cacheDir, "observations-test").apply {
            deleteRecursively()
            mkdirs()
        }
        store = RoomObservationStore(db.observations(), imagesDir)
    }

    @After
    fun tearDown() {
        db.close()
        imagesDir.deleteRecursively()
    }

    private fun meta(index: Int, sizeBytes: Long = 1_000) = ObservationMeta(
        observationId = "OBS%04d".format(index),
        capturedAt = base.plusSeconds(index * 300L),
        imagePath = "OBS%04d.jpg".format(index),
        thumbPath = "OBS%04d-thumb.jpg".format(index),
        lat = 35.681236,
        lng = 139.767125,
        sizeBytes = sizeBytes,
    )

    @Test
    fun `保存した観測を capturedAt 昇順で返す`() = runTest {
        store.insert(meta(2), ByteArray(4), ByteArray(2))
        store.insert(meta(0), ByteArray(4), ByteArray(2))
        store.insert(meta(1), ByteArray(4), ByteArray(2))

        assertEquals(listOf("OBS0000", "OBS0001", "OBS0002"), store.all().map { it.observationId })
    }

    @Test
    fun `メタデータを往復させても値が変わらない`() = runTest {
        store.insert(meta(0), ByteArray(4), ByteArray(2))

        val row = store.all().single()
        assertEquals("OBS0000", row.observationId)
        assertEquals(base, row.capturedAt)
        assertEquals(35.681236, row.lat)
        assertEquals(139.767125, row.lng)
        assertEquals(1_000L, row.sizeBytes)
        assertEquals(0, row.attemptCount)
        assertNull(row.nextAttemptAt)
    }

    @Test
    fun `位置が無くても保存できる`() = runTest {
        store.insert(meta(0).copy(lat = null, lng = null), ByteArray(4), ByteArray(2))

        val row = store.all().single()
        assertNull(row.lat)
        assertNull(row.lng)
    }

    @Test
    fun `画像はファイルに置き、読み戻せる`() = runTest {
        // BLOB を SQLite に入れると DB が肥大し、削除しても領域が戻りにくい。
        val image = ByteArray(16) { 7 }
        val thumb = ByteArray(8) { 3 }
        store.insert(meta(0), image, thumb)

        assertTrue(File(imagesDir, "OBS0000.jpg").exists())
        assertTrue(File(imagesDir, "OBS0000-thumb.jpg").exists())

        val frame = store.load("OBS0000")!!
        assertContentEquals(image, frame.image)
        assertContentEquals(thumb, frame.thumbnail)
        assertEquals(base, frame.capturedAt)
        assertEquals(35.681236, frame.lat)
    }

    @Test
    fun `削除すると行と画像ファイルの両方が消える`() = runTest {
        store.insert(meta(0), ByteArray(4), ByteArray(2))

        store.delete("OBS0000")

        assertEquals(emptyList(), store.all())
        assertFalse(File(imagesDir, "OBS0000.jpg").exists())
        assertFalse(File(imagesDir, "OBS0000-thumb.jpg").exists())
    }

    @Test
    fun `合計バイト数を返す`() = runTest {
        store.insert(meta(0, sizeBytes = 1_000), ByteArray(4), ByteArray(2))
        store.insert(meta(1, sizeBytes = 2_500), ByteArray(4), ByteArray(2))

        assertEquals(3_500L, store.totalBytes())
    }

    @Test
    fun `空のときの合計バイト数は0`() = runTest {
        assertEquals(0L, store.totalBytes())
    }

    @Test
    fun `失敗を記録すると試行回数と次回時刻が残る`() = runTest {
        store.insert(meta(0), ByteArray(4), ByteArray(2))

        store.recordFailure("OBS0000", attemptCount = 3, nextAttemptAt = base.plusSeconds(40))

        val row = store.all().single()
        assertEquals(3, row.attemptCount)
        assertEquals(base.plusSeconds(40), row.nextAttemptAt)
    }

    @Test
    fun `行が無ければ load は null`() = runTest {
        assertNull(store.load("MISSING"))
    }

    @Test
    fun `画像ファイルが失われていたら load は null`() = runTest {
        store.insert(meta(0), ByteArray(4), ByteArray(2))
        File(imagesDir, "OBS0000.jpg").delete()

        assertNull(store.load("OBS0000"))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.data.RoomObservationStoreTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: ObservationDb`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/data/ObservationDb.kt`:

```kotlin
package com.edgewatcher.app.data

import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.RoomDatabase

/** 画像本体はファイル。ここにはメタデータだけを持つ。 */
@Entity(tableName = "observations")
data class ObservationRow(
    @PrimaryKey val observationId: String,
    val capturedAtMillis: Long,
    val imagePath: String,
    val thumbPath: String,
    val lat: Double?,
    val lng: Double?,
    val sizeBytes: Long,
    val attemptCount: Int,
    val nextAttemptAtMillis: Long?,
)

@Dao
interface ObservationDao {

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insert(row: ObservationRow)

    @Query("SELECT * FROM observations ORDER BY capturedAtMillis ASC")
    suspend fun all(): List<ObservationRow>

    @Query("SELECT * FROM observations WHERE observationId = :id")
    suspend fun byId(id: String): ObservationRow?

    @Query("SELECT COALESCE(SUM(sizeBytes), 0) FROM observations")
    suspend fun totalBytes(): Long

    @Query("DELETE FROM observations WHERE observationId = :id")
    suspend fun delete(id: String)

    @Query(
        "UPDATE observations SET attemptCount = :attemptCount, " +
            "nextAttemptAtMillis = :nextAttemptAtMillis WHERE observationId = :id"
    )
    suspend fun recordFailure(id: String, attemptCount: Int, nextAttemptAtMillis: Long)
}

@Database(entities = [ObservationRow::class], version = 1, exportSchema = false)
abstract class ObservationDb : RoomDatabase() {
    abstract fun observations(): ObservationDao
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/data/RoomObservationStore.kt`:

```kotlin
package com.edgewatcher.app.data

import com.edgewatcher.domain.ObservationMeta
import com.edgewatcher.domain.ObservationStore
import com.edgewatcher.domain.UploadFrame
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.time.Instant

class RoomObservationStore(
    private val dao: ObservationDao,
    private val imagesDir: File,
) : ObservationStore {

    init {
        imagesDir.mkdirs()
    }

    override suspend fun insert(meta: ObservationMeta, image: ByteArray, thumbnail: ByteArray) {
        withContext(Dispatchers.IO) {
            File(imagesDir, meta.imagePath).writeBytes(image)
            File(imagesDir, meta.thumbPath).writeBytes(thumbnail)
        }
        dao.insert(meta.toRow())
    }

    override suspend fun all(): List<ObservationMeta> = dao.all().map { it.toMeta() }

    override suspend fun totalBytes(): Long = dao.totalBytes()

    override suspend fun load(observationId: String): UploadFrame? {
        val row = dao.byId(observationId) ?: return null
        return withContext(Dispatchers.IO) {
            val image = File(imagesDir, row.imagePath)
            val thumb = File(imagesDir, row.thumbPath)
            // 行はあるのに画像が無い状態は、送っても永久に成立しない。
            // 呼び出し側（UploadPump）が行ごと捨てる。
            if (!image.exists() || !thumb.exists()) return@withContext null
            UploadFrame(
                observationId = row.observationId,
                capturedAt = Instant.ofEpochMilli(row.capturedAtMillis),
                lat = row.lat,
                lng = row.lng,
                image = image.readBytes(),
                thumbnail = thumb.readBytes(),
            )
        }
    }

    override suspend fun delete(observationId: String) {
        val row = dao.byId(observationId)
        dao.delete(observationId)
        if (row != null) {
            withContext(Dispatchers.IO) {
                File(imagesDir, row.imagePath).delete()
                File(imagesDir, row.thumbPath).delete()
            }
        }
    }

    override suspend fun recordFailure(
        observationId: String,
        attemptCount: Int,
        nextAttemptAt: Instant,
    ) {
        dao.recordFailure(observationId, attemptCount, nextAttemptAt.toEpochMilli())
    }
}

private fun ObservationMeta.toRow() = ObservationRow(
    observationId = observationId,
    capturedAtMillis = capturedAt.toEpochMilli(),
    imagePath = imagePath,
    thumbPath = thumbPath,
    lat = lat,
    lng = lng,
    sizeBytes = sizeBytes,
    attemptCount = attemptCount,
    nextAttemptAtMillis = nextAttemptAt?.toEpochMilli(),
)

private fun ObservationRow.toMeta() = ObservationMeta(
    observationId = observationId,
    capturedAt = Instant.ofEpochMilli(capturedAtMillis),
    imagePath = imagePath,
    thumbPath = thumbPath,
    lat = lat,
    lng = lng,
    sizeBytes = sizeBytes,
    attemptCount = attemptCount,
    nextAttemptAt = nextAttemptAtMillis?.let { Instant.ofEpochMilli(it) },
)
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.data.RoomObservationStoreTest" --no-daemon; Pop-Location
```

期待: PASS（10件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): Room によるバッファの実装を追加"
```

---

### Task 16: 資格情報と設定の保存

**暗号化そのものは Android Keystore の仕事で、Robolectric では検証できない。** そこで「何をどの鍵で保存するか」というロジックを `SharedPreferences` を受け取るクラスに閉じ込め、**暗号化された `SharedPreferences` を渡すのは組み立て側の責務**にする。テストは平文の `SharedPreferences` を渡して振る舞いだけを確かめる。

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/data/PrefsCredentialStore.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/data/PrefsSettingsStore.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/data/SecurePrefs.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/data/PrefsStoresTest.kt`

**Interfaces:**
- Consumes: `CredentialStore` / `Credentials` / `Session`（Task 5）、`SettingsStore` / `ObservationEngine`（Task 11）
- Produces:
  - `class PrefsCredentialStore(prefs: SharedPreferences) : CredentialStore`
  - `class PrefsSettingsStore(prefs: SharedPreferences) : SettingsStore`
  - `object SecurePrefs { fun create(context: Context, name: String): SharedPreferences }`

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/data/PrefsStoresTest.kt`:

```kotlin
package com.edgewatcher.app.data

import android.content.Context
import android.content.SharedPreferences
import androidx.test.core.app.ApplicationProvider
import com.edgewatcher.domain.Credentials
import com.edgewatcher.domain.ObservationEngine
import com.edgewatcher.domain.Session
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

@RunWith(RobolectricTestRunner::class)
class PrefsStoresTest {

    private lateinit var prefs: SharedPreferences

    @Before
    fun setUp() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        prefs = context.getSharedPreferences("test-prefs", Context.MODE_PRIVATE)
        prefs.edit().clear().commit()
    }

    @Test
    fun `保存していなければ資格情報は null`() {
        assertNull(PrefsCredentialStore(prefs).readCredentials())
    }

    @Test
    fun `資格情報を往復させられる`() {
        val store = PrefsCredentialStore(prefs)
        store.writeCredentials(Credentials("DEV1", "SECRET1"))

        assertEquals(Credentials("DEV1", "SECRET1"), store.readCredentials())
    }

    @Test
    fun `書き込みは即座に確定する`() {
        // deviceSecret は再取得できない。保存できたことを確かめてから
        // トークン取得へ進めるよう、非同期の apply ではなく commit を使う。
        PrefsCredentialStore(prefs).writeCredentials(Credentials("DEV1", "SECRET1"))

        val reopened = PrefsCredentialStore(prefs)
        assertEquals("DEV1", reopened.readCredentials()?.deviceId)
    }

    @Test
    fun `セッションを往復させられる`() {
        val store = PrefsCredentialStore(prefs)
        val session = Session("DEV1.ABC", Instant.ofEpochSecond(1789000000L))
        store.writeSession(session)

        assertEquals(session, store.readSession())
    }

    @Test
    fun `保存していなければセッションは null`() {
        assertNull(PrefsCredentialStore(prefs).readSession())
    }

    @Test
    fun `clear は資格情報とセッションの両方を消す`() {
        val store = PrefsCredentialStore(prefs)
        store.writeCredentials(Credentials("DEV1", "SECRET1"))
        store.writeSession(Session("DEV1.ABC", Instant.now()))

        store.clear()

        assertNull(store.readCredentials())
        assertNull(store.readSession())
    }

    @Test
    fun `clear は他の設定を巻き添えにしない`() {
        // 送信間隔まで消すと、再ペアリング後の初回が既定値に戻る。
        // 実害は小さいが、消す範囲は明示的にする。
        val credentials = PrefsCredentialStore(prefs)
        val settings = PrefsSettingsStore(prefs)
        settings.setIntervalMinutes(15)
        credentials.writeCredentials(Credentials("DEV1", "SECRET1"))

        credentials.clear()

        assertEquals(15, settings.intervalMinutes())
    }

    @Test
    fun `送信間隔の初期値は5分`() {
        assertEquals(
            ObservationEngine.DEFAULT_INTERVAL_MINUTES,
            PrefsSettingsStore(prefs).intervalMinutes(),
        )
    }

    @Test
    fun `送信間隔を往復させられる`() {
        val settings = PrefsSettingsStore(prefs)
        settings.setIntervalMinutes(10)

        assertEquals(10, PrefsSettingsStore(prefs).intervalMinutes())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.data.PrefsStoresTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: PrefsCredentialStore`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/data/PrefsCredentialStore.kt`:

```kotlin
package com.edgewatcher.app.data

import android.content.SharedPreferences
import com.edgewatcher.domain.CredentialStore
import com.edgewatcher.domain.Credentials
import com.edgewatcher.domain.Session
import java.time.Instant

/**
 * 暗号化は [SecurePrefs] が用意した [SharedPreferences] に委ねる。
 * このクラスは「何をどの鍵で保存するか」だけを持つ。
 *
 * **値をログに出してはならない。**
 */
class PrefsCredentialStore(private val prefs: SharedPreferences) : CredentialStore {

    private companion object {
        const val KEY_DEVICE_ID = "deviceId"
        const val KEY_DEVICE_SECRET = "deviceSecret"
        const val KEY_SESSION_TOKEN = "sessionToken"
        const val KEY_SESSION_EXPIRES_AT = "sessionExpiresAt"
    }

    override fun readCredentials(): Credentials? {
        val id = prefs.getString(KEY_DEVICE_ID, null) ?: return null
        val secret = prefs.getString(KEY_DEVICE_SECRET, null) ?: return null
        return Credentials(id, secret)
    }

    override fun writeCredentials(credentials: Credentials) {
        // deviceSecret は再取得できない。書けたことを確認してから次へ進めるよう commit を使う。
        prefs.edit()
            .putString(KEY_DEVICE_ID, credentials.deviceId)
            .putString(KEY_DEVICE_SECRET, credentials.deviceSecret)
            .commit()
    }

    override fun readSession(): Session? {
        val token = prefs.getString(KEY_SESSION_TOKEN, null) ?: return null
        val expiresAt = prefs.getLong(KEY_SESSION_EXPIRES_AT, -1L)
        if (expiresAt < 0) return null
        return Session(token, Instant.ofEpochSecond(expiresAt))
    }

    override fun writeSession(session: Session) {
        prefs.edit()
            .putString(KEY_SESSION_TOKEN, session.token)
            .putLong(KEY_SESSION_EXPIRES_AT, session.expiresAt.epochSecond)
            .commit()
    }

    override fun clear() {
        prefs.edit()
            .remove(KEY_DEVICE_ID)
            .remove(KEY_DEVICE_SECRET)
            .remove(KEY_SESSION_TOKEN)
            .remove(KEY_SESSION_EXPIRES_AT)
            .commit()
    }
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/data/PrefsSettingsStore.kt`:

```kotlin
package com.edgewatcher.app.data

import android.content.SharedPreferences
import com.edgewatcher.domain.ObservationEngine
import com.edgewatcher.domain.SettingsStore

class PrefsSettingsStore(private val prefs: SharedPreferences) : SettingsStore {

    private companion object {
        const val KEY_INTERVAL_MINUTES = "intervalMinutes"
    }

    override fun intervalMinutes(): Int =
        prefs.getInt(KEY_INTERVAL_MINUTES, ObservationEngine.DEFAULT_INTERVAL_MINUTES)

    override fun setIntervalMinutes(minutes: Int) {
        prefs.edit().putInt(KEY_INTERVAL_MINUTES, minutes).apply()
    }
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/data/SecurePrefs.kt`:

```kotlin
package com.edgewatcher.app.data

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

object SecurePrefs {

    fun create(context: Context, name: String): SharedPreferences {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        return EncryptedSharedPreferences.create(
            context,
            name,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }
}
```

`SecurePrefs` には自動テストを書かない。Android Keystore に依存し、Robolectric では検証できないため。**実機スモークの手順1（ペアリングが成立し、アプリを再起動しても観測が続く）が、この経路が生きていることの確認になる。**

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.data.PrefsStoresTest" --no-daemon; Pop-Location
```

期待: PASS（9件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): 資格情報と設定の保存を追加"
```

---

## フェーズ4: 端末アダプタと Foreground Service

このフェーズの方針: **判断はすべて `:domain` に置き、`:app` 側は分岐を持たない糊にする。** カメラやアラームは自動テストで確信が得にくいので、テストできる形の部分を先に切り出す。

### Task 17: JPEG の符号化

**Files:**
- Modify: `android/domain/src/main/kotlin/com/edgewatcher/domain/ImagePolicy.kt`（`targetSize` を追加）
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/device/AndroidJpegEncoder.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/ImagePolicyTargetSizeTest.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/device/AndroidJpegEncoderTest.kt`

**Interfaces:**
- Consumes: `JpegEncoder` / `EncodeStep`（Task 3・9）
- Produces:
  - `ImagePolicy.targetSize(width: Int, height: Int, longEdge: Int): Pair<Int, Int>`
  - `class AndroidJpegEncoder : JpegEncoder`

- [ ] **Step 1: 縮尺の計算に対する失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/ImagePolicyTargetSizeTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class ImagePolicyTargetSizeTest {

    @Test
    fun `横長は幅を長辺に合わせる`() {
        assertEquals(1600 to 1200, ImagePolicy.targetSize(width = 4000, height = 3000, longEdge = 1600))
    }

    @Test
    fun `縦長は高さを長辺に合わせる`() {
        assertEquals(1200 to 1600, ImagePolicy.targetSize(width = 3000, height = 4000, longEdge = 1600))
    }

    @Test
    fun `正方形は両辺が長辺になる`() {
        assertEquals(160 to 160, ImagePolicy.targetSize(width = 2000, height = 2000, longEdge = 160))
    }

    @Test
    fun `元が長辺より小さければ拡大しない`() {
        // 引き伸ばしても情報は増えず、容量だけ増える。
        assertEquals(800 to 600, ImagePolicy.targetSize(width = 800, height = 600, longEdge = 1600))
    }

    @Test
    fun `短辺は最低1ピクセルを保つ`() {
        // 極端に細長い入力で 0 になると Bitmap の生成が落ちる。
        assertEquals(160 to 1, ImagePolicy.targetSize(width = 4000, height = 3, longEdge = 160))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ImagePolicyTargetSizeTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: targetSize`）で FAIL。

- [ ] **Step 3: `targetSize` を実装する**

`ImagePolicy` に追記:

```kotlin
    /**
     * 長辺を [longEdge] に合わせた寸法を返す。元が小さければ拡大しない。
     * 画素の操作は Android の関心だが、**どの寸法にするかは方針**なのでここに置く。
     */
    fun targetSize(width: Int, height: Int, longEdge: Int): Pair<Int, Int> {
        val source = maxOf(width, height)
        if (source <= longEdge) return width to height
        val scale = longEdge.toDouble() / source
        return maxOf(1, Math.round(width * scale).toInt()) to
            maxOf(1, Math.round(height * scale).toInt())
    }
```

- [ ] **Step 4: 縮尺のテストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ImagePolicyTargetSizeTest" --no-daemon; Pop-Location
```

期待: PASS（5件）。

- [ ] **Step 5: 符号化器に対する失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/device/AndroidJpegEncoderTest.kt`:

```kotlin
package com.edgewatcher.app.device

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.edgewatcher.domain.EncodeStep
import com.edgewatcher.domain.ImagePolicy
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.GraphicsMode
import java.io.ByteArrayOutputStream
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AndroidJpegEncoderTest {

    private fun sourceJpeg(width: Int, height: Int): ByteArray {
        val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
        // 一様な色だと圧縮が効きすぎるので、簡単な模様を入れる。
        for (x in 0 until width step 7) {
            for (y in 0 until height step 5) {
                bitmap.setPixel(x, y, 0xFF3366AA.toInt())
            }
        }
        return ByteArrayOutputStream().use { out ->
            bitmap.compress(Bitmap.CompressFormat.JPEG, 100, out)
            out.toByteArray()
        }
    }

    private fun sizeOf(jpeg: ByteArray): Pair<Int, Int> {
        val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(jpeg, 0, jpeg.size, options)
        return options.outWidth to options.outHeight
    }

    @Test
    fun `本画像は長辺1600に縮小される`() {
        val encoded = AndroidJpegEncoder().encode(sourceJpeg(2400, 1800), EncodeStep(1600, 80))

        assertEquals(1600 to 1200, sizeOf(encoded))
    }

    @Test
    fun `サムネイルは長辺160に縮小される`() {
        val encoded = AndroidJpegEncoder().encode(sourceJpeg(2400, 1800), ImagePolicy.THUMBNAIL)

        assertEquals(160 to 120, sizeOf(encoded))
    }

    @Test
    fun `品質を下げると小さくなる`() {
        val encoder = AndroidJpegEncoder()
        val source = sourceJpeg(1600, 1200)

        val high = encoder.encode(source, EncodeStep(1600, 80))
        val low = encoder.encode(source, EncodeStep(1600, 50))

        assertTrue(low.size < high.size, "品質50が品質80より小さくない: ${low.size} vs ${high.size}")
    }

    @Test
    fun `出力は JPEG である`() {
        val encoded = AndroidJpegEncoder().encode(sourceJpeg(400, 300), EncodeStep(1600, 80))

        // JPEG は必ず FF D8 で始まる。
        assertEquals(0xFF.toByte(), encoded[0])
        assertEquals(0xD8.toByte(), encoded[1])
    }
}
```

- [ ] **Step 6: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.AndroidJpegEncoderTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: AndroidJpegEncoder`）で FAIL。

**`@GraphicsMode(NATIVE)` が動かない場合**: Robolectric が実際の画素操作を行えないと、寸法のアサーションが落ちる。その場合は `android/gradle.properties` に `robolectric.graphicsMode=NATIVE` を足して再実行する。それでも動かなければ、この4件のテストは削除せず `@Ignore` を付けずに残したまま、**Robolectric のバージョンを上げて対処する**（この経路をテスト無しにしない）。

- [ ] **Step 7: 符号化器を実装する**

`android/app/src/main/kotlin/com/edgewatcher/app/device/AndroidJpegEncoder.kt`:

```kotlin
package com.edgewatcher.app.device

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.edgewatcher.domain.EncodeStep
import com.edgewatcher.domain.ImagePolicy
import com.edgewatcher.domain.JpegEncoder
import java.io.ByteArrayOutputStream

/** 画素の操作だけを行う。どの寸法・品質にするかは [ImagePolicy] が決める。 */
class AndroidJpegEncoder : JpegEncoder {

    override fun encode(sourceJpeg: ByteArray, step: EncodeStep): ByteArray {
        val source = BitmapFactory.decodeByteArray(sourceJpeg, 0, sourceJpeg.size)
            ?: error("JPEG を復号できない")
        val (width, height) = ImagePolicy.targetSize(source.width, source.height, step.longEdge)
        val scaled = if (width == source.width && height == source.height) {
            source
        } else {
            Bitmap.createScaledBitmap(source, width, height, true)
        }
        return ByteArrayOutputStream().use { out ->
            scaled.compress(Bitmap.CompressFormat.JPEG, step.quality, out)
            if (scaled !== source) scaled.recycle()
            source.recycle()
            out.toByteArray()
        }
    }
}
```

- [ ] **Step 8: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.AndroidJpegEncoderTest" --no-daemon; Pop-Location
```

期待: PASS（4件）。

- [ ] **Step 9: Commit**

```bash
git add android/domain android/app
git commit -m "feat: JPEG の符号化と縮尺の計算を追加"
```

---

### Task 18: 位置情報の取得

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/device/FusedLocationSource.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/device/LocationMappingTest.kt`

**Interfaces:**
- Consumes: `LocationSource` / `GeoPoint`（Task 9）
- Produces:
  - `internal fun Location?.toGeoPoint(): GeoPoint?`
  - `class FusedLocationSource(context: Context) : LocationSource`

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/device/LocationMappingTest.kt`:

```kotlin
package com.edgewatcher.app.device

import android.location.Location
import com.edgewatcher.domain.GeoPoint
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

@RunWith(RobolectricTestRunner::class)
class LocationMappingTest {

    @Test
    fun `位置が無ければ null`() {
        // 測位を待たないので、まだ一度も測れていない状態は普通に起こる。
        assertNull((null as Location?).toGeoPoint())
    }

    @Test
    fun `緯度経度をそのまま写す`() {
        val location = Location("fused").apply {
            latitude = 35.681236
            longitude = 139.767125
        }

        assertEquals(GeoPoint(35.681236, 139.767125), location.toGeoPoint())
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.LocationMappingTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: toGeoPoint`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/device/FusedLocationSource.kt`:

```kotlin
package com.edgewatcher.app.device

import android.annotation.SuppressLint
import android.content.Context
import android.location.Location
import com.edgewatcher.domain.GeoPoint
import com.edgewatcher.domain.LocationSource
import com.google.android.gms.location.LocationServices
import com.google.android.gms.tasks.Tasks
import java.util.concurrent.TimeUnit

internal fun Location?.toGeoPoint(): GeoPoint? =
    this?.let { GeoPoint(lat = it.latitude, lng = it.longitude) }

/**
 * **測位を待たない。** 最後の既知位置で十分であり、定点観測デバイスは動かない。
 * `getCurrentLocation` を使うと測位を待つことになるので使わない。
 */
class FusedLocationSource(context: Context) : LocationSource {

    private val client = LocationServices.getFusedLocationProviderClient(context)

    @SuppressLint("MissingPermission")
    override fun lastKnown(): GeoPoint? = try {
        // lastLocation は既にキャッシュされた値を返すため即座に完了する。
        // 念のため短い上限を置き、ここで撮影周期を止めない。
        Tasks.await(client.lastLocation, 2, TimeUnit.SECONDS).toGeoPoint()
    } catch (e: Exception) {
        // 権限が無い / Play 開発者サービスが無い / タイムアウト。
        // いずれも位置なしで送るだけで、観測そのものは成立する。
        null
    }
}
```

`FusedLocationSource` 自体には自動テストを書かない。Play 開発者サービスに依存し、Robolectric では意味のある Red を作れないため。**位置が取れなくても撮影が成立することは Task 9 の `CaptureCoordinator` のテストで既に押さえてある。**

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.LocationMappingTest" --no-daemon; Pop-Location
```

期待: PASS（2件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): 最後の既知位置の取得を追加"
```

---

### Task 19: アラームによる起床

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/device/AlarmManagerScheduler.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/service/CaptureAlarmReceiver.kt`
- Modify: `android/app/src/main/AndroidManifest.xml`（レシーバを登録）
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/device/AlarmManagerSchedulerTest.kt`

**Interfaces:**
- Consumes: `AlarmScheduler`（Task 9）
- Produces:
  - `class AlarmManagerScheduler(context: Context, exactAllowed: () -> Boolean) : AlarmScheduler`
  - `class CaptureAlarmReceiver : BroadcastReceiver`
  - `const val ACTION_CAPTURE = "com.edgewatcher.action.CAPTURE"`

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/device/AlarmManagerSchedulerTest.kt`:

```kotlin
package com.edgewatcher.app.device

import android.app.AlarmManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

@RunWith(RobolectricTestRunner::class)
class AlarmManagerSchedulerTest {

    private lateinit var context: Context
    private lateinit var alarmManager: AlarmManager

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        alarmManager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
    }

    @Test
    fun `指定した時刻にアラームを予約する`() {
        AlarmManagerScheduler(context) { true }.scheduleAt(1_789_000_000_000L)

        val scheduled = shadowOf(alarmManager).nextScheduledAlarm
        assertEquals(1_789_000_000_000L, scheduled.triggerAtTime)
    }

    @Test
    fun `予約し直すと1本だけになる`() {
        // 周期アラームではなく都度予約なので、二重に積まれると連射になる。
        val scheduler = AlarmManagerScheduler(context) { true }
        scheduler.scheduleAt(1_789_000_000_000L)
        scheduler.scheduleAt(1_789_000_300_000L)

        val shadow = shadowOf(alarmManager)
        assertEquals(1, shadow.scheduledAlarms.size)
        assertEquals(1_789_000_300_000L, shadow.nextScheduledAlarm.triggerAtTime)
    }

    @Test
    fun `正確なアラームが許可されていなくても予約はする`() {
        // 不正確になるが、止まるよりはるかにましである。
        AlarmManagerScheduler(context) { false }.scheduleAt(1_789_000_000_000L)

        assertEquals(1_789_000_000_000L, shadowOf(alarmManager).nextScheduledAlarm.triggerAtTime)
    }

    @Test
    fun `取り消すと予約が消える`() {
        val scheduler = AlarmManagerScheduler(context) { true }
        scheduler.scheduleAt(1_789_000_000_000L)

        scheduler.cancel()

        assertNull(shadowOf(alarmManager).nextScheduledAlarm)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.AlarmManagerSchedulerTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: AlarmManagerScheduler`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/service/CaptureAlarmReceiver.kt`:

```kotlin
package com.edgewatcher.app.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

const val ACTION_CAPTURE = "com.edgewatcher.action.CAPTURE"

/**
 * アラームで起こされる。ここから Foreground Service を新規に開始するのではなく、
 * **既に前面で動いているサービスに撮影を指示する**。
 * 新規開始ならバックグラウンド起動の制限に掛かるが、指示は掛からない。
 */
class CaptureAlarmReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_CAPTURE) return
        ObservationService.requestCapture(context)
    }
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/device/AlarmManagerScheduler.kt`:

```kotlin
package com.edgewatcher.app.device

import android.app.AlarmManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import com.edgewatcher.app.service.ACTION_CAPTURE
import com.edgewatcher.app.service.CaptureAlarmReceiver
import com.edgewatcher.domain.AlarmScheduler

/**
 * ウェイクロックと二重化するための足場。
 * ウェイクロックがメーカー独自の省電力機構に剥がされた場合や、
 * プロセスが落ちて作り直された場合に、次の撮影時刻から復帰できるようにする。
 */
class AlarmManagerScheduler(
    private val context: Context,
    private val exactAllowed: () -> Boolean = { defaultExactAllowed(context) },
) : AlarmScheduler {

    companion object {
        private const val REQUEST_CODE = 1

        fun defaultExactAllowed(context: Context): Boolean {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return true
            val manager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
            return manager.canScheduleExactAlarms()
        }
    }

    private val manager = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager

    private val pendingIntent: PendingIntent
        get() = PendingIntent.getBroadcast(
            context,
            REQUEST_CODE,
            Intent(context, CaptureAlarmReceiver::class.java).setAction(ACTION_CAPTURE),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

    override fun scheduleAt(epochMillis: Long) {
        if (exactAllowed()) {
            manager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, epochMillis, pendingIntent)
        } else {
            // 許可が無ければ不正確なアラームに落とす。止まるよりましである。
            manager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, epochMillis, pendingIntent)
        }
    }

    override fun cancel() {
        manager.cancel(pendingIntent)
    }
}
```

`AndroidManifest.xml` の `<application>` 内に追記:

```xml
        <receiver
            android:name=".service.CaptureAlarmReceiver"
            android:exported="false" />
```

**`setExactAndAllowWhileIdle` と `setAndAllowWhileIdle` の違いは Robolectric からは観測できない。** テストで押さえられるのは「どちらの経路でも予約されること」までであり、精度の差は実機スモークの手順5（画面を消して15分放置）で確認する。

- [ ] **Step 4: テストが通ることを確認する**

`ObservationService` はまだ存在しないので、この時点ではコンパイルが通らない。**先に受け口だけを置く。**

`android/app/src/main/kotlin/com/edgewatcher/app/service/ObservationService.kt`（この Task では受け口のみ。中身は Task 21）:

```kotlin
package com.edgewatcher.app.service

import android.content.Context
import android.content.Intent
import androidx.lifecycle.LifecycleService

const val ACTION_START = "com.edgewatcher.action.START"
const val ACTION_STOP = "com.edgewatcher.action.STOP"
const val ACTION_REQUEST_CAPTURE = "com.edgewatcher.action.REQUEST_CAPTURE"

class ObservationService : LifecycleService() {

    companion object {
        fun start(context: Context) {
            context.startForegroundService(
                Intent(context, ObservationService::class.java).setAction(ACTION_START)
            )
        }

        fun stop(context: Context) {
            context.startService(
                Intent(context, ObservationService::class.java).setAction(ACTION_STOP)
            )
        }

        /** 既に動いているサービスに撮影を指示する。新規開始ではない。 */
        fun requestCapture(context: Context) {
            context.startService(
                Intent(context, ObservationService::class.java).setAction(ACTION_REQUEST_CAPTURE)
            )
        }
    }
}
```

`AndroidManifest.xml` の `<application>` 内に追記:

```xml
        <service
            android:name=".service.ObservationService"
            android:exported="false"
            android:foregroundServiceType="camera|location" />
```

再度テストを実行する。

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.device.AlarmManagerSchedulerTest" --no-daemon; Pop-Location
```

期待: PASS（4件）。

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): アラームによる起床とサービスの受け口を追加"
```

---

### Task 20: カメラのバインド方針と CameraX 実装

`04-native.md` §2.2（発熱を避けるため常時は開かない）と §3.2（プレビュー中も同じセッションから撮り、送信に穴を開けない）は、素直に読むと衝突する。**両立する規則を `:domain` の純関数として書き、テストで固定する。**

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/CameraBinding.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/device/CameraXCaptureSource.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/CameraBindingTest.kt`

**Interfaces:**
- Consumes: `CaptureSource`（Task 9）
- Produces:
  - `enum class CameraUseCase { PREVIEW, CAPTURE }`
  - `object CameraBinding { fun desired(previewOn: Boolean, capturing: Boolean): Set<CameraUseCase> }`
  - `class CameraXCaptureSource(context, lifecycleOwner) : CaptureSource` — `fun setPreview(provider: Preview.SurfaceProvider?)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/CameraBindingTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class CameraBindingTest {

    @Test
    fun `通常時はカメラを閉じておく`() {
        // カメラを開き続けると発熱し、屋外長期設置に向かない。
        assertEquals(emptySet(), CameraBinding.desired(previewOn = false, capturing = false))
    }

    @Test
    fun `撮影の瞬間だけ撮影用をバインドする`() {
        assertEquals(
            setOf(CameraUseCase.CAPTURE),
            CameraBinding.desired(previewOn = false, capturing = true),
        )
    }

    @Test
    fun `プレビュー中は開いたまま保つ`() {
        assertEquals(
            setOf(CameraUseCase.PREVIEW, CameraUseCase.CAPTURE),
            CameraBinding.desired(previewOn = true, capturing = false),
        )
    }

    @Test
    fun `プレビュー中は撮影の前後でバインドが変わらない`() {
        // ここが変わると、プレビューのたびにセッションを張り直すことになり、
        // 「プレビューを開いている間だけ観測に穴が開く」挙動に戻ってしまう。
        assertEquals(
            CameraBinding.desired(previewOn = true, capturing = false),
            CameraBinding.desired(previewOn = true, capturing = true),
        )
    }

    @Test
    fun `プレビューを閉じたらカメラも閉じる`() {
        assertEquals(emptySet(), CameraBinding.desired(previewOn = false, capturing = false))
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.CameraBindingTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: CameraBinding`）で FAIL。

- [ ] **Step 3: バインド方針を実装する**

`android/domain/src/main/kotlin/com/edgewatcher/domain/CameraBinding.kt`:

```kotlin
package com.edgewatcher.domain

enum class CameraUseCase { PREVIEW, CAPTURE }

/**
 * カメラの所有権は Foreground Service が一本で持つ。
 * Android のカメラは同時に2つのクライアントが開けないため、
 * 画面は Surface を渡すだけで、開閉の判断はここに集約する。
 */
object CameraBinding {

    fun desired(previewOn: Boolean, capturing: Boolean): Set<CameraUseCase> = when {
        // プレビュー中は開いたまま維持し、定期撮影も同じセッションから行う。
        previewOn -> setOf(CameraUseCase.PREVIEW, CameraUseCase.CAPTURE)
        capturing -> setOf(CameraUseCase.CAPTURE)
        else -> emptySet()
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.CameraBindingTest" --no-daemon; Pop-Location
```

期待: PASS（5件）。

- [ ] **Step 5: CameraX の実装を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/device/CameraXCaptureSource.kt`:

```kotlin
package com.edgewatcher.app.device

import android.content.Context
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import com.edgewatcher.domain.CameraBinding
import com.edgewatcher.domain.CameraUseCase
import com.edgewatcher.domain.CaptureSource
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.Dispatchers
import kotlin.coroutines.resume

/**
 * 開閉の判断は [CameraBinding] が持つ。このクラスはそれを CameraX に写すだけの糊で、
 * 自前の分岐を持たない。自動テストを書かないのはそのためで、
 * 実際の撮影は実機スモークで確認する。
 */
class CameraXCaptureSource(
    private val context: Context,
    private val lifecycleOwner: LifecycleOwner,
) : CaptureSource {

    private var provider: ProcessCameraProvider? = null
    private var previewSurfaceProvider: Preview.SurfaceProvider? = null
    private val imageCapture = ImageCapture.Builder()
        .setCaptureMode(ImageCapture.CAPTURE_MODE_MINIMIZE_LATENCY)
        .build()

    /** 画面から Surface を受け取る。null でプレビューを畳む。 */
    suspend fun setPreview(surfaceProvider: Preview.SurfaceProvider?) {
        previewSurfaceProvider = surfaceProvider
        rebind(capturing = false)
    }

    override suspend fun capture(): ByteArray? {
        rebind(capturing = true)
        val bytes = takePicture()
        // プレビュー中なら desired は変わらないので、ここで閉じられることはない。
        rebind(capturing = false)
        return bytes
    }

    private suspend fun rebind(capturing: Boolean) = withContext(Dispatchers.Main) {
        val cameraProvider = provider ?: awaitProvider().also { provider = it }
        val desired = CameraBinding.desired(
            previewOn = previewSurfaceProvider != null,
            capturing = capturing,
        )

        cameraProvider.unbindAll()
        if (desired.isEmpty()) return@withContext

        val useCases = buildList {
            if (CameraUseCase.PREVIEW in desired) {
                add(Preview.Builder().build().also { it.surfaceProvider = previewSurfaceProvider })
            }
            if (CameraUseCase.CAPTURE in desired) add(imageCapture)
        }
        cameraProvider.bindToLifecycle(
            lifecycleOwner,
            CameraSelector.DEFAULT_BACK_CAMERA,
            *useCases.toTypedArray(),
        )
    }

    private suspend fun awaitProvider(): ProcessCameraProvider =
        suspendCancellableCoroutine { cont ->
            val future = ProcessCameraProvider.getInstance(context)
            future.addListener(
                { cont.resume(future.get()) },
                ContextCompat.getMainExecutor(context),
            )
        }

    private suspend fun takePicture(): ByteArray? = suspendCancellableCoroutine { cont ->
        imageCapture.takePicture(
            ContextCompat.getMainExecutor(context),
            object : ImageCapture.OnImageCapturedCallback() {
                override fun onCaptureSuccess(image: ImageProxy) {
                    val buffer = image.planes[0].buffer
                    val bytes = ByteArray(buffer.remaining())
                    buffer.get(bytes)
                    image.close()
                    cont.resume(bytes)
                }

                override fun onError(exception: ImageCaptureException) {
                    // 撮れなかった回は飛ばす。次のアラームで再挑戦する。
                    cont.resume(null)
                }
            },
        )
    }
}
```

- [ ] **Step 6: ビルドが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:assembleMockDebug --no-daemon; Pop-Location
```

期待: `BUILD SUCCESSFUL`。

- [ ] **Step 7: Commit**

```bash
git add android/domain android/app
git commit -m "feat: カメラのバインド方針と CameraX 実装を追加"
```

---

### Task 21: 観測1周期の組み立てと Foreground Service

サービスの中身をそのまま書くとテストできないので、**1周期の手順を `:domain` の `ObservationCycle` に出す。** サービスはウェイクロックと通知を持ち、周期の実行を委ねるだけにする。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/ObservationCycle.kt`
- Modify: `android/domain/src/main/kotlin/com/edgewatcher/domain/Ports.kt`（`SettingsStore` に最終送信時刻を追加）
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/data/PrefsSettingsStore.kt`
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/service/ObservationService.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/ObservationCycleTest.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/data/PrefsStoresTest.kt`（最終送信時刻の往復を追加）

**Interfaces:**
- Consumes: `CaptureCoordinator`（Task 9）、`UploadPump`（Task 10）、`ObservationEngine`（Task 11）、`AlarmScheduler`（Task 9）
- Produces:
  - `SettingsStore` に `fun lastUploadAt(): Instant?` と `fun setLastUploadAt(at: Instant)` を追加
  - `sealed interface CycleResult` — `Continued(run: RunState, nextCaptureAt: Instant)` / `Unpaired`
  - `class ObservationCycle(capture, pump, settings, alarms, store, clock)` — `suspend fun runOnce(): CycleResult`
  - `ObservationService` の完成形

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/ObservationCycleTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.random.Random
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ObservationCycleTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")

    private class InMemorySettings(
        private var interval: Int = ObservationEngine.DEFAULT_INTERVAL_MINUTES,
        private var last: Instant? = null,
    ) : SettingsStore {
        override fun intervalMinutes(): Int = interval
        override fun setIntervalMinutes(minutes: Int) { interval = minutes }
        override fun lastUploadAt(): Instant? = last
        override fun setLastUploadAt(at: Instant) { last = at }
    }

    private fun cycle(
        api: FakeApi,
        store: InMemoryObservationStore = InMemoryObservationStore(),
        credentials: InMemoryCredentialStore = InMemoryCredentialStore(
            credentials = Credentials("DEV1", "SECRET1"),
            session = Session("TOKEN1", now.plusSeconds(3600)),
        ),
        settings: SettingsStore = InMemorySettings(),
        alarms: RecordingAlarmScheduler = RecordingAlarmScheduler(),
        camera: CaptureSource = FakeCaptureSource(),
    ): Triple<ObservationCycle, RecordingAlarmScheduler, InMemoryObservationStore> {
        val clock = FixedClock(now)
        val coordinator = CaptureCoordinator(
            camera = camera,
            encoder = FakeJpegEncoder { 100_000 },
            location = FakeLocationSource(),
            store = store,
            clock = clock,
            random = Random(1),
        )
        val pump = UploadPump(api, SessionManager(api, credentials, clock), store, clock)
        return Triple(
            ObservationCycle(coordinator, pump, settings, alarms, store, clock),
            alarms,
            store,
        )
    }

    @Test
    fun `撮影して送信し、次のアラームを予約する`() = runTest {
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (c, alarms, store) = cycle(api)

        val result = c.runOnce()

        assertEquals(RunState.Observing(now), (result as CycleResult.Continued).run)
        assertEquals(emptyList(), store.all())
        assertEquals(listOf(now.plusSeconds(300).toEpochMilli()), alarms.scheduled)
    }

    @Test
    fun `nextConfig を次の周期から適用する`() = runTest {
        // サーバは設定を push しない。この経路が唯一の伝達路。
        val api = FakeApi().apply { uploadResults += UploadResult.Success(15) }
        val settings = InMemorySettings()
        val (c, alarms, _) = cycle(api, settings = settings)

        c.runOnce()

        assertEquals(15, settings.intervalMinutes())
        assertEquals(listOf(now.plusSeconds(900).toEpochMilli()), alarms.scheduled)
    }

    @Test
    fun `想定外の間隔は無視して現状を保つ`() = runTest {
        val api = FakeApi().apply { uploadResults += UploadResult.Success(7) }
        val settings = InMemorySettings(interval = 10)
        val (c, alarms, _) = cycle(api, settings = settings)

        c.runOnce()

        assertEquals(10, settings.intervalMinutes())
        assertEquals(listOf(now.plusSeconds(600).toEpochMilli()), alarms.scheduled)
    }

    @Test
    fun `送信に失敗したらオフラインとして次の周期を予約する`() = runTest {
        // リトライ中も定期撮影は止めない。
        val api = FakeApi().apply { uploadResults += UploadResult.Retryable }
        val (c, alarms, store) = cycle(api)

        val result = c.runOnce()

        assertEquals(RunState.Offline(pending = 1, lastUploadAt = null), (result as CycleResult.Continued).run)
        assertEquals(1, store.all().size)
        assertEquals(1, alarms.scheduled.size)
    }

    @Test
    fun `資格情報が失われたら周期を止めてアラームを取り消す`() = runTest {
        val api = FakeApi().apply {
            uploadResults += UploadResult.Unauthorized
            tokenResults += TokenResult.Invalid
        }
        val (c, alarms, _) = cycle(api)

        assertEquals(CycleResult.Unpaired, c.runOnce())
        assertEquals(emptyList(), alarms.scheduled)
        assertEquals(1, alarms.cancelCount)
    }

    @Test
    fun `溜まっている分を1周期でまとめて送る`() = runTest {
        val store = InMemoryObservationStore()
        repeat(3) { i ->
            store.insert(
                ObservationMeta(
                    observationId = "OLD$i",
                    capturedAt = now.minusSeconds((3 - i) * 300L),
                    imagePath = "o$i.jpg",
                    thumbPath = "t$i.jpg",
                    lat = null,
                    lng = null,
                    sizeBytes = 1_000,
                ),
                ByteArray(2), ByteArray(1),
            )
        }
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (c, _, s) = cycle(api, store = store)

        c.runOnce()

        // 撮った1件 + 溜まっていた3件 = 4件すべて送り切る
        assertEquals(4, api.uploadCalls.size)
        assertEquals(emptyList(), s.all())
    }

    @Test
    fun `1周期の送信数に上限を設けて周期を止めない`() = runTest {
        val store = InMemoryObservationStore()
        repeat(60) { i ->
            store.insert(
                ObservationMeta(
                    observationId = "OLD%02d".format(i),
                    capturedAt = now.minusSeconds((60 - i) * 300L),
                    imagePath = "o$i.jpg",
                    thumbPath = "t$i.jpg",
                    lat = null,
                    lng = null,
                    sizeBytes = 1_000,
                ),
                ByteArray(2), ByteArray(1),
            )
        }
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (c, alarms, s) = cycle(api, store = store)

        c.runOnce()

        assertEquals(ObservationCycle.MAX_UPLOADS_PER_CYCLE, api.uploadCalls.size)
        assertTrue(s.all().isNotEmpty())
        assertEquals(1, alarms.scheduled.size)
    }

    @Test
    fun `撮影に失敗しても送信と予約は行う`() = runTest {
        val store = InMemoryObservationStore()
        store.insert(
            ObservationMeta("OLD0", now.minusSeconds(300), "o.jpg", "t.jpg", null, null, 1_000),
            ByteArray(2), ByteArray(1),
        )
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (c, alarms, s) = cycle(api, store = store, camera = FakeCaptureSource(jpeg = null))

        val result = c.runOnce()

        assertEquals(1, api.uploadCalls.size)
        assertEquals(emptyList(), s.all())
        assertEquals(1, alarms.scheduled.size)
        assertTrue(result is CycleResult.Continued)
    }

    @Test
    fun `最終送信時刻を記録する`() = runTest {
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val settings = InMemorySettings()
        val (c, _, _) = cycle(api, settings = settings)

        c.runOnce()

        assertEquals(now, settings.lastUploadAt())
    }

    @Test
    fun `退避が起きていなければバナーを立てない`() = runTest {
        val api = FakeApi().apply { uploadResults += UploadResult.Success(5) }
        val (c, _, _) = cycle(api)

        assertEquals(false, (c.runOnce() as CycleResult.Continued).trimmed)
    }

    @Test
    fun `退避が起きたらバナーを立てる`() = runTest {
        // 「保存できる上限に達したため、古い画像から削除しています。」を出す根拠。
        val store = InMemoryObservationStore()
        repeat(288) { i ->
            store.insert(
                ObservationMeta(
                    observationId = "OLD%04d".format(i),
                    capturedAt = now.minusSeconds((300 - i) * 300L),
                    imagePath = "o$i.jpg",
                    thumbPath = "t$i.jpg",
                    lat = null,
                    lng = null,
                    sizeBytes = 1_000,
                ),
                ByteArray(2), ByteArray(1),
            )
        }
        // 送信は失敗させてバッファを減らさない
        val api = FakeApi().apply { uploadResults += UploadResult.Retryable }
        val (c, _, _) = cycle(api, store = store)

        assertEquals(true, (c.runOnce() as CycleResult.Continued).trimmed)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ObservationCycleTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: ObservationCycle`）で FAIL。

- [ ] **Step 3: `SettingsStore` を拡張して周期を実装する**

`Ports.kt` の `SettingsStore` を差し替える:

```kotlin
interface SettingsStore {
    /** 現在の送信間隔（分）。まだ受け取っていなければ既定値。 */
    fun intervalMinutes(): Int

    fun setIntervalMinutes(minutes: Int)

    /** 最後に送信が成功した時刻。状態1行の「最終送信」に使う。 */
    fun lastUploadAt(): Instant?

    fun setLastUploadAt(at: Instant)
}
```

`Ports.kt` の先頭に `import java.time.Instant` を追加する。

`android/domain/src/main/kotlin/com/edgewatcher/domain/ObservationCycle.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant

sealed interface CycleResult {
    data class Continued(
        val run: RunState,
        val nextCaptureAt: Instant,
        /** この周期でバッファ上限に達し、古いものを削除したか。画面のバナーに使う。 */
        val trimmed: Boolean,
    ) : CycleResult

    /** 資格情報が失われた。未ペアリング画面へ戻る。 */
    data object Unpaired : CycleResult
}

/**
 * 観測1周期。撮って、送れるだけ送って、次のアラームを置く。
 * サービスから切り出してあるのは、この手順が要件のほぼ全部だからで、
 * Android を起動せずにテストできる形にしておく。
 */
class ObservationCycle(
    private val capture: CaptureCoordinator,
    private val pump: UploadPump,
    private val settings: SettingsStore,
    private val alarms: AlarmScheduler,
    private val store: ObservationStore,
    private val clock: Clock,
) {
    companion object {
        /**
         * 1周期で送る上限。長時間オフラインから復帰したときに
         * 数百件を一度に流し込んで周期を止めないための歯止め。
         */
        const val MAX_UPLOADS_PER_CYCLE = 20
    }

    suspend fun runOnce(): CycleResult {
        // 撮影に失敗しても送信は続ける。次のアラームで撮り直せばよい。
        val captured = capture.captureOnce()
        val trimmed = captured is CaptureOutcome.Stored && captured.evicted.isNotEmpty()

        var interval = settings.intervalMinutes()
        var lastAttemptFailed = false
        var handled = 0
        var stop = false

        // repeat で書くと `return@repeat` が「次の反復へ進む」意味になり、
        // 送れない状態のまま上限まで空回りする。明示的に打ち切る。
        while (handled < MAX_UPLOADS_PER_CYCLE && !stop) {
            when (val step = pump.sendOne()) {
                is UploadStep.Sent -> {
                    interval = ObservationEngine.applyNextConfig(interval, step.intervalMinutes)
                    settings.setIntervalMinutes(interval)
                    settings.setLastUploadAt(clock.now())
                    handled++
                }
                is UploadStep.Discarded -> handled++
                is UploadStep.Deferred -> {
                    lastAttemptFailed = true
                    stop = true
                }
                UploadStep.Waiting -> {
                    lastAttemptFailed = true
                    stop = true
                }
                UploadStep.Idle -> stop = true
                UploadStep.Unpaired -> {
                    alarms.cancel()
                    return CycleResult.Unpaired
                }
            }
        }

        val run = ObservationEngine.deriveRunState(
            pending = store.all().size,
            lastUploadAt = settings.lastUploadAt(),
            lastAttemptFailed = lastAttemptFailed,
        )
        val next = ObservationEngine.nextCaptureAt(clock.now(), interval)
        alarms.scheduleAt(next.toEpochMilli())
        return CycleResult.Continued(run, next, trimmed)
    }
}
```

`PrefsSettingsStore` に追記:

```kotlin
    override fun lastUploadAt(): Instant? {
        val seconds = prefs.getLong(KEY_LAST_UPLOAD_AT, -1L)
        return if (seconds < 0) null else Instant.ofEpochSecond(seconds)
    }

    override fun setLastUploadAt(at: Instant) {
        prefs.edit().putLong(KEY_LAST_UPLOAD_AT, at.epochSecond).apply()
    }
```

`companion object` に `const val KEY_LAST_UPLOAD_AT = "lastUploadAt"` を、ファイル先頭に `import java.time.Instant` を追加する。

`PrefsStoresTest` に追記:

```kotlin
    @Test
    fun `最終送信時刻を往復させられる`() {
        val settings = PrefsSettingsStore(prefs)
        assertNull(settings.lastUploadAt())

        settings.setLastUploadAt(Instant.ofEpochSecond(1789000000L))

        assertEquals(Instant.ofEpochSecond(1789000000L), PrefsSettingsStore(prefs).lastUploadAt())
    }
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.ObservationCycleTest" --no-daemon; Pop-Location
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.data.PrefsStoresTest" --no-daemon; Pop-Location
```

期待: それぞれ PASS（11件 / 10件）。

- [ ] **Step 5: Foreground Service を実装する**

`android/app/src/main/kotlin/com/edgewatcher/app/service/ObservationService.kt` を完成させる。

```kotlin
package com.edgewatcher.app.service

import android.content.Context
import android.content.Intent
import android.os.PowerManager
import androidx.camera.core.Preview
import androidx.lifecycle.LifecycleService
import androidx.lifecycle.lifecycleScope
import com.edgewatcher.app.AppContainer
import com.edgewatcher.domain.CycleResult
import com.edgewatcher.domain.RunState
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

const val ACTION_START = "com.edgewatcher.action.START"
const val ACTION_STOP = "com.edgewatcher.action.STOP"
const val ACTION_REQUEST_CAPTURE = "com.edgewatcher.action.REQUEST_CAPTURE"

class ObservationService : LifecycleService() {

    companion object {
        fun start(context: Context) {
            context.startForegroundService(
                Intent(context, ObservationService::class.java).setAction(ACTION_START)
            )
        }

        fun stop(context: Context) {
            context.startService(
                Intent(context, ObservationService::class.java).setAction(ACTION_STOP)
            )
        }

        /** 既に動いているサービスに撮影を指示する。新規開始ではない。 */
        fun requestCapture(context: Context) {
            context.startService(
                Intent(context, ObservationService::class.java).setAction(ACTION_REQUEST_CAPTURE)
            )
        }
    }

    private lateinit var container: AppContainer
    private var wakeLock: PowerManager.WakeLock? = null
    private val cycleLock = Mutex()

    // カメラはこのサービスの Lifecycle に縛る。所有権を一本化するため、
    // 画面側からは決して生成しない。
    private val camera by lazy { container.cameraFor(this) }
    private val cycle by lazy { container.cycleFor(camera) }

    override fun onCreate() {
        super.onCreate()
        container = AppContainer.of(this)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)
        when (intent?.action) {
            ACTION_START -> startObserving()
            ACTION_REQUEST_CAPTURE -> runCycle()
            ACTION_STOP -> stopObserving()
        }
        // プロセスが落とされたら作り直させる。
        // ただし Android 14 以上では camera 型の再生成が失敗しうる（§12）。
        return START_STICKY
    }

    private fun startObserving() {
        startForeground(
            Notifications.ONGOING_ID,
            container.notifications.ongoing(RunState.Observing(container.settings.lastUploadAt())),
        )
        acquireWakeLock()
        runCycle()
    }

    private fun stopObserving() {
        container.alarms.cancel()
        releaseWakeLock()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun runCycle(): Job = lifecycleScope.launch {
        // アラームとユーザー操作が重なっても、周期は同時に走らせない。
        cycleLock.withLock {
            when (val result = cycle.runOnce()) {
                is CycleResult.Continued -> {
                    container.publish(result.run, result.trimmed)
                    // 常駐通知も同じ状態へ更新する。画面を点けなくても異常に気づけるようにする。
                    getSystemService(NotificationManager::class.java)
                        .notify(Notifications.ONGOING_ID, container.notifications.ongoing(result.run))
                }
                CycleResult.Unpaired -> {
                    // 黙って QR スキャナに戻すと、端末の故障と区別がつかない。
                    container.publishUnpaired(UnpairReason.DISCONNECTED_BY_SERVER)
                    stopObserving()
                }
            }
        }
    }

    /**
     * Foreground Service は CPU のサスペンドを妨げない。
     * 画面 OFF 中もタイマーを動かすため、稼働中は保持し続ける。
     * 常時給電が前提なので選べる設計である。
     */
    private fun acquireWakeLock() {
        if (wakeLock != null) return
        val power = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "EdgeWatcher::observation")
            .also { it.acquire() }
    }

    private fun releaseWakeLock() {
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
    }

    /** 画面からプレビュー用の Surface を受け取る。カメラ本体は画面に渡さない。 */
    fun setPreview(surfaceProvider: Preview.SurfaceProvider?) {
        lifecycleScope.launch { camera.setPreview(surfaceProvider) }
    }

    override fun onDestroy() {
        releaseWakeLock()
        super.onDestroy()
    }
}
```

`AppContainer` と `Notifications` はまだ無いので、この時点ではコンパイルが通らない。**Task 22 と Task 26 で埋める。** それまでこの Task は完了としない。

- [ ] **Step 6: Commit（Task 22 まで進めてからビルドを確認する）**

```bash
git add android/domain android/app
git commit -m "feat: 観測1周期の組み立てと Foreground Service を追加"
```

---

## フェーズ5: UI と通知

### Task 22: 手書き DI と通知

`ObservationService`（Task 21）がコンパイルできるようになるのがこの Task の完了条件。

**Files:**
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/AppContainer.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/service/Notifications.kt`
- Create: `android/app/src/main/res/values/strings.xml`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/service/NotificationsTest.kt`

**Interfaces:**
- Consumes: Task 4〜21 のすべて
- Produces:
  - `class AppContainer` — `credentials` / `settings` / `observations` / `api` / `alarms` / `notifications` / `pairing` / `sessions` / `state: StateFlow<AppState>` / `publish(run)` / `publishUnpaired(reason)` / `cameraFor(owner)` / `cycleFor(camera)`
  - `class Notifications(context)` — `ONGOING_ID` / `RECOVERY_ID` / `ongoing(run: RunState): Notification` / `recoveryRequired(): Notification`

- [ ] **Step 1: 失敗するテストを書く**

`android/app/src/test/kotlin/com/edgewatcher/app/service/NotificationsTest.kt`:

```kotlin
package com.edgewatcher.app.service

import android.app.Notification
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.edgewatcher.domain.RunState
import org.junit.Before
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

@RunWith(RobolectricTestRunner::class)
class NotificationsTest {

    private lateinit var notifications: Notifications

    @Before
    fun setUp() {
        notifications = Notifications(ApplicationProvider.getApplicationContext<Context>())
    }

    private fun title(n: Notification) = n.extras.getString(Notification.EXTRA_TITLE)
    private fun text(n: Notification) = n.extras.getString(Notification.EXTRA_TEXT)

    @Test
    fun `常駐通知は画面と同じ状態1行を出す`() {
        // 一致を実装の注意深さではなく、StatusLine からの派生で担保する。
        val n = notifications.ongoing(RunState.Observing(Instant.parse("2026-09-10T05:32:05Z")))

        assertEquals("観測中", title(n))
        assertTrue(text(n)!!.startsWith("最終送信 "))
    }

    @Test
    fun `オフラインの常駐通知は待機件数を出す`() {
        // 画面を点けなくても異常に気づけるようにする。
        val n = notifications.ongoing(RunState.Offline(pending = 12, lastUploadAt = null))

        assertEquals("オフライン", title(n))
        assertEquals("12件待機中", text(n))
    }

    @Test
    fun `常駐通知は消せない`() {
        val n = notifications.ongoing(RunState.Observing(null))

        assertTrue(n.flags and Notification.FLAG_ONGOING_EVENT != 0)
    }

    @Test
    fun `復帰要求の通知は再開を促す`() {
        // Android 14 以上では再起動後に自動復帰できない。
        val n = notifications.recoveryRequired()

        assertEquals("EdgeWatcher が停止しています", title(n))
        assertTrue(text(n)!!.contains("タップして観測を再開"))
    }

    @Test
    fun `常駐と復帰要求は別のチャンネルを使う`() {
        // 常駐は無音、復帰要求は気づかせる必要がある。
        assertTrue(
            notifications.ongoing(RunState.Observing(null)).channelId !=
                notifications.recoveryRequired().channelId
        )
    }

    @Test
    fun `常駐と復帰要求は別の通知IDを使う`() {
        assertTrue(Notifications.ONGOING_ID != Notifications.RECOVERY_ID)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.service.NotificationsTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: Notifications`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/app/src/main/res/values/strings.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="app_name">EdgeWatcher</string>
    <string name="channel_ongoing">観測の稼働状態</string>
    <string name="channel_recovery">観測の再開が必要</string>
</resources>
```

`android/app/src/main/kotlin/com/edgewatcher/app/service/Notifications.kt`:

```kotlin
package com.edgewatcher.app.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.edgewatcher.app.R
import com.edgewatcher.app.ui.MainActivity
import com.edgewatcher.domain.RunState
import com.edgewatcher.domain.StatusLine
import java.time.ZoneId

class Notifications(
    private val context: Context,
    private val intervalMinutesProvider: () -> Int = { 5 },
) {
    companion object {
        const val ONGOING_ID = 1
        const val RECOVERY_ID = 2
        const val CHANNEL_ONGOING = "observation-ongoing"
        const val CHANNEL_RECOVERY = "observation-recovery"
    }

    init {
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ONGOING,
                context.getString(R.string.channel_ongoing),
                // 常時稼働の前提なので、通知が出続けることは許容する。ただし無音にする。
                NotificationManager.IMPORTANCE_LOW,
            )
        )
        manager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_RECOVERY,
                context.getString(R.string.channel_recovery),
                NotificationManager.IMPORTANCE_HIGH,
            )
        )
    }

    /** 本文は画面の状態行と同じ [StatusLine] から作る。端末名は出さない。 */
    fun ongoing(run: RunState): Notification {
        val text = StatusLine.of(run, intervalMinutesProvider(), ZoneId.systemDefault())
        return NotificationCompat.Builder(context, CHANNEL_ONGOING)
            .setSmallIcon(android.R.drawable.presence_video_online)
            .setContentTitle(text.title)
            .setContentText(text.detail)
            .setOngoing(true)
            .setContentIntent(openApp())
            .build()
    }

    fun recoveryRequired(): Notification =
        NotificationCompat.Builder(context, CHANNEL_RECOVERY)
            .setSmallIcon(android.R.drawable.stat_notify_error)
            .setContentTitle("EdgeWatcher が停止しています")
            .setContentText("タップして観測を再開してください。端末の再起動後は自動で再開できません。")
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setAutoCancel(true)
            .setContentIntent(openApp())
            .build()

    private fun openApp(): PendingIntent = PendingIntent.getActivity(
        context,
        0,
        Intent(context, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/AppContainer.kt`:

```kotlin
package com.edgewatcher.app

import android.content.Context
import android.os.Build
import androidx.lifecycle.LifecycleOwner
import androidx.room.Room
import com.edgewatcher.app.data.ObservationDb
import com.edgewatcher.app.data.PrefsCredentialStore
import com.edgewatcher.app.data.PrefsSettingsStore
import com.edgewatcher.app.data.RoomObservationStore
import com.edgewatcher.app.data.SecurePrefs
import com.edgewatcher.app.device.AlarmManagerScheduler
import com.edgewatcher.app.device.AndroidJpegEncoder
import com.edgewatcher.app.device.CameraXCaptureSource
import com.edgewatcher.app.device.FusedLocationSource
import com.edgewatcher.app.net.createEdgeWatcherApi
import com.edgewatcher.app.service.Notifications
import com.edgewatcher.domain.AppState
import com.edgewatcher.domain.CaptureCoordinator
import com.edgewatcher.domain.CaptureSource
import com.edgewatcher.domain.DeviceInfo
import com.edgewatcher.domain.ObservationCycle
import com.edgewatcher.domain.PairingService
import com.edgewatcher.domain.RunState
import com.edgewatcher.domain.SessionManager
import com.edgewatcher.domain.SystemClock
import com.edgewatcher.domain.UnpairReason
import com.edgewatcher.domain.UploadPump
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File

/**
 * 手書きの DI。全てコンストラクタ注入にしてあるので、
 * テストに必要なものは既に揃っており、DI フレームワークを足す理由がない。
 */
class AppContainer private constructor(private val context: Context) {

    companion object {
        @Volatile
        private var instance: AppContainer? = null

        fun of(context: Context): AppContainer =
            instance ?: synchronized(this) {
                instance ?: AppContainer(context.applicationContext).also { instance = it }
            }
    }

    private val securePrefs = SecurePrefs.create(context, "edgewatcher-credentials")

    val credentials = PrefsCredentialStore(securePrefs)
    val settings = PrefsSettingsStore(securePrefs)

    private val db = Room.databaseBuilder(context, ObservationDb::class.java, "observations.db").build()

    val observations = RoomObservationStore(
        dao = db.observations(),
        imagesDir = File(context.filesDir, "observations"),
    )

    val api = createEdgeWatcherApi()

    val alarms = AlarmManagerScheduler(context)

    val notifications = Notifications(context) { settings.intervalMinutes() }

    val sessions = SessionManager(api, credentials, SystemClock)

    val pairing = PairingService(api) { delay(it) }

    val deviceInfo = DeviceInfo(
        model = Build.MODEL,
        osVersion = Build.VERSION.RELEASE,
        appVersion = BuildConfig.VERSION_NAME,
    )

    private val _state = MutableStateFlow<AppState>(
        if (credentials.readCredentials() == null) {
            AppState.Unpaired(reason = null)
        } else {
            AppState.Paired(RunState.Stopped)
        }
    )
    val state: StateFlow<AppState> = _state.asStateFlow()

    private val _bufferTrimmed = MutableStateFlow(false)

    /** バッファ上限に達して古い画像を削除したか。稼働画面のバナーに使う。 */
    val bufferTrimmed: StateFlow<Boolean> = _bufferTrimmed.asStateFlow()

    fun publish(run: RunState, trimmed: Boolean = false) {
        _state.value = AppState.Paired(run)
        // 一度立ったら、次に退避なしの周期が来るまで出し続ける。
        _bufferTrimmed.value = trimmed
    }

    fun publishUnpaired(reason: UnpairReason?) {
        _state.value = AppState.Unpaired(reason)
        _bufferTrimmed.value = false
    }

    /** カメラの所有権はサービスの Lifecycle に縛る。 */
    fun cameraFor(owner: LifecycleOwner): CameraXCaptureSource =
        CameraXCaptureSource(context, owner)

    fun cycleFor(camera: CaptureSource): ObservationCycle = ObservationCycle(
        capture = CaptureCoordinator(
            camera = camera,
            encoder = AndroidJpegEncoder(),
            location = FusedLocationSource(context),
            store = observations,
            clock = SystemClock,
        ),
        pump = UploadPump(api, sessions, observations, SystemClock),
        settings = settings,
        alarms = alarms,
        store = observations,
        clock = SystemClock,
    )
}
```

- [ ] **Step 4: テストが通ることを確認する**

`MainActivity` はまだ無いので、先に空の Activity を置く（Task 26 で中身を書く）。

`android/app/src/main/kotlin/com/edgewatcher/app/ui/MainActivity.kt`:

```kotlin
package com.edgewatcher.app.ui

import android.os.Bundle
import androidx.activity.ComponentActivity

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
    }
}
```

`AndroidManifest.xml` の `<application>` 内に追記:

```xml
        <activity
            android:name=".ui.MainActivity"
            android:exported="true"
            android:screenOrientation="portrait">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
```

```powershell
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.service.NotificationsTest" --no-daemon; Pop-Location
Push-Location android; .\gradlew.bat :app:assembleMockDebug --no-daemon; Pop-Location
```

期待: テスト PASS（6件）、`BUILD SUCCESSFUL`。**ここで Task 21 の `ObservationService` が初めてコンパイルできる。**

- [ ] **Step 5: Commit**

```bash
git add android/app
git commit -m "feat(app): 手書き DI と通知を追加"
```

---

### Task 23: 権限ゲートの判定

権限の**判定**を純粋な関数として切り出し、テストで固定する。Compose の描画そのものはテストしない。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/Permissions.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/ui/PermissionScreen.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/PermissionsTest.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `enum class RequiredPermission { CAMERA, LOCATION, NOTIFICATIONS }`
  - `sealed interface PermissionGate` — `Granted` / `NeedsRequest(missing: Set<RequiredPermission>)` / `Blocked(missing: Set<RequiredPermission>)`
  - `object Permissions { fun required(sdkInt: Int): Set<RequiredPermission>; fun gate(granted, permanentlyDenied, sdkInt): PermissionGate }`
  - `@Composable fun PermissionScreen(gate: PermissionGate, onRequest: () -> Unit, onOpenSettings: () -> Unit)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/PermissionsTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class PermissionsTest {

    @Test
    fun `Android 12 以下では通知権限を要求しない`() {
        assertEquals(
            setOf(RequiredPermission.CAMERA, RequiredPermission.LOCATION),
            Permissions.required(sdkInt = 32),
        )
    }

    @Test
    fun `Android 13 以上では通知権限も要求する`() {
        assertEquals(
            setOf(
                RequiredPermission.CAMERA,
                RequiredPermission.LOCATION,
                RequiredPermission.NOTIFICATIONS,
            ),
            Permissions.required(sdkInt = 33),
        )
    }

    @Test
    fun `必要なものが揃っていれば通す`() {
        val gate = Permissions.gate(
            granted = Permissions.required(36),
            permanentlyDenied = emptySet(),
            sdkInt = 36,
        )
        assertEquals(PermissionGate.Granted, gate)
    }

    @Test
    fun `足りなければ要求する`() {
        val gate = Permissions.gate(
            granted = setOf(RequiredPermission.LOCATION, RequiredPermission.NOTIFICATIONS),
            permanentlyDenied = emptySet(),
            sdkInt = 36,
        )
        assertEquals(PermissionGate.NeedsRequest(setOf(RequiredPermission.CAMERA)), gate)
    }

    @Test
    fun `恒久的に拒否されていたら設定へ誘導する`() {
        // 権限なしで動作する縮退モードは持たない。
        // カメラか位置情報が欠けた時点でこのアプリの目的が成立しない。
        val gate = Permissions.gate(
            granted = setOf(RequiredPermission.LOCATION, RequiredPermission.NOTIFICATIONS),
            permanentlyDenied = setOf(RequiredPermission.CAMERA),
            sdkInt = 36,
        )
        assertEquals(PermissionGate.Blocked(setOf(RequiredPermission.CAMERA)), gate)
    }

    @Test
    fun `Android 12 以下では通知権限が無くても通す`() {
        val gate = Permissions.gate(
            granted = setOf(RequiredPermission.CAMERA, RequiredPermission.LOCATION),
            permanentlyDenied = emptySet(),
            sdkInt = 32,
        )
        assertEquals(PermissionGate.Granted, gate)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PermissionsTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: Permissions`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/Permissions.kt`:

```kotlin
package com.edgewatcher.domain

enum class RequiredPermission { CAMERA, LOCATION, NOTIFICATIONS }

sealed interface PermissionGate {
    data object Granted : PermissionGate
    data class NeedsRequest(val missing: Set<RequiredPermission>) : PermissionGate

    /** 恒久的に拒否された。設定へ誘導するしかない。 */
    data class Blocked(val missing: Set<RequiredPermission>) : PermissionGate
}

object Permissions {

    /** `ACCESS_BACKGROUND_LOCATION` は含めない。FGS の location タイプが「使用中」を成立させる。 */
    fun required(sdkInt: Int): Set<RequiredPermission> = buildSet {
        add(RequiredPermission.CAMERA)
        add(RequiredPermission.LOCATION)
        if (sdkInt >= 33) add(RequiredPermission.NOTIFICATIONS)
    }

    fun gate(
        granted: Set<RequiredPermission>,
        permanentlyDenied: Set<RequiredPermission>,
        sdkInt: Int,
    ): PermissionGate {
        val missing = required(sdkInt) - granted
        return when {
            missing.isEmpty() -> PermissionGate.Granted
            (missing intersect permanentlyDenied).isNotEmpty() ->
                PermissionGate.Blocked(missing intersect permanentlyDenied)
            else -> PermissionGate.NeedsRequest(missing)
        }
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PermissionsTest" --no-daemon; Pop-Location
```

期待: PASS（6件）。

- [ ] **Step 5: 画面を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/ui/PermissionScreen.kt`:

```kotlin
package com.edgewatcher.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.weight
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.edgewatcher.domain.PermissionGate

@Composable
fun PermissionScreen(
    gate: PermissionGate,
    onRequest: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        when (gate) {
            is PermissionGate.Blocked -> {
                Text("カメラと位置情報の権限がありません")
                Text("EdgeWatcher はカメラと位置情報がないと観測を行えません。設定から許可してください。")
                Spacer(Modifier.weight(1f))
                Button(onClick = onOpenSettings, modifier = Modifier.fillMaxWidth()) {
                    Text("設定を開く")
                }
            }
            else -> {
                Text("観測に必要な権限")
                Text("この端末を定点観測デバイスとして動かすために、以下を許可してください。")
                Text("・カメラ — 定期的に撮影します")
                Text("・位置情報 — 観測地点を記録します。バックグラウンドでの位置情報は要求しません")
                Text("・通知 — 観測中であることを常に表示します")
                Spacer(Modifier.weight(1f))
                Button(onClick = onRequest, modifier = Modifier.fillMaxWidth()) {
                    Text("許可する")
                }
            }
        }
    }
}
```

- [ ] **Step 6: Commit**

```bash
git add android/domain android/app
git commit -m "feat: 権限ゲートの判定と画面を追加"
```

---

### Task 24: ペアリングの手順とスキャナ画面

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/PairingFlow.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/ui/ScannerScreen.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/PairingFlowTest.kt`

**Interfaces:**
- Consumes: `PairingService`（Task 4）、`CredentialStore`（Task 5）、`SessionManager`（Task 5）
- Produces:
  - `enum class PairingFailure { UNUSABLE, NOT_FOUND, UNREACHABLE, REJECTED, STORAGE_FAILED }`
  - `sealed interface PairingStep` — `NotOurQr` / `Paired` / `Failed(reason: PairingFailure)`
  - `class PairingFlow(pairing, credentials, sessions)` — `suspend fun consume(payload: String, deviceInfo: DeviceInfo): PairingStep`
  - `@Composable fun ScannerScreen(...)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/PairingFlowTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class PairingFlowTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")
    private val info = DeviceInfo("Pixel 6a", "16", "1.0.0")

    private fun flow(
        api: FakeApi,
        store: InMemoryCredentialStore = InMemoryCredentialStore(),
    ): Pair<PairingFlow, InMemoryCredentialStore> {
        val clock = FixedClock(now)
        return PairingFlow(
            pairing = PairingService(api) { },
            credentials = store,
            sessions = SessionManager(api, store, clock),
        ) to store
    }

    @Test
    fun `自分宛てでない QR は無視してスキャンを続ける`() = runTest {
        val api = FakeApi()
        val (f, _) = flow(api)

        assertEquals(PairingStep.NotOurQr, f.consume("https://example.com", info))
        assertEquals(0, api.pairCalls.size)
    }

    @Test
    fun `成功したら資格情報を保存してセッションを取る`() = runTest {
        val api = FakeApi().apply {
            pairResults += PairResult.Success("DEV1", "SECRET1")
            tokenResults += TokenResult.Success("DEV1.ABC", now.plusSeconds(43_200))
        }
        val (f, store) = flow(api)

        assertEquals(PairingStep.Paired, f.consume("ew1:CODE", info))
        assertEquals(Credentials("DEV1", "SECRET1"), store.readCredentials())
        assertEquals("DEV1.ABC", store.readSession()?.token)
    }

    @Test
    fun `保存できなければペアリングを失敗として扱う`() = runTest {
        // deviceSecret は再取得できない。保存に失敗したまま進むと、
        // その端末は QR を再発行するまで永久に復帰できない。
        val api = FakeApi().apply { pairResults += PairResult.Success("DEV1", "SECRET1") }
        val store = object : InMemoryCredentialStore() {
            override fun writeCredentials(credentials: Credentials) { /* 書けないディスク */ }
        }
        val f = PairingFlow(PairingService(api) { }, store, SessionManager(api, store, FixedClock(now)))

        assertEquals(PairingStep.Failed(PairingFailure.STORAGE_FAILED), f.consume("ew1:CODE", info))
    }

    @Test
    fun `409 は使えない QR として返す`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.Unusable }
        val (f, store) = flow(api)

        assertEquals(PairingStep.Failed(PairingFailure.UNUSABLE), f.consume("ew1:CODE", info))
        assertNull(store.readCredentials())
    }

    @Test
    fun `リトライを尽くした 404 は見つからないとして返す`() = runTest {
        val api = FakeApi().apply { pairResults += PairResult.NotFound }
        val (f, _) = flow(api)

        assertEquals(PairingStep.Failed(PairingFailure.NOT_FOUND), f.consume("ew1:CODE", info))
    }

    @Test
    fun `到達不能は使えない QR と区別して返す`() = runTest {
        // 圏外の端末に「この QR は使えません」と出すと誤誘導になる。
        val api = FakeApi().apply { pairResults += PairResult.Unreachable }
        val (f, _) = flow(api)

        assertEquals(PairingStep.Failed(PairingFailure.UNREACHABLE), f.consume("ew1:CODE", info))
    }

    @Test
    fun `トークン取得が一時的に失敗してもペアリングは成立させる`() = runTest {
        // deviceSecret は保存できている。セッションはサービスが後で取り直せる。
        val api = FakeApi().apply {
            pairResults += PairResult.Success("DEV1", "SECRET1")
            tokenResults += TokenResult.Retryable
        }
        val (f, store) = flow(api)

        assertEquals(PairingStep.Paired, f.consume("ew1:CODE", info))
        assertNotNull(store.readCredentials())
    }

    @Test
    fun `直後のトークン取得が401なら資格情報を消して失敗にする`() = runTest {
        val api = FakeApi().apply {
            pairResults += PairResult.Success("DEV1", "SECRET1")
            tokenResults += TokenResult.Invalid
        }
        val (f, store) = flow(api)

        assertEquals(PairingStep.Failed(PairingFailure.UNUSABLE), f.consume("ew1:CODE", info))
        assertNull(store.readCredentials())
    }
}
```

`InMemoryCredentialStore` を継承できるように `open class` へ変える（`android/domain/src/test/kotlin/com/edgewatcher/domain/InMemoryCredentialStore.kt` の宣言を `open class InMemoryCredentialStore` にし、`writeCredentials` に `open` を付ける）。

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PairingFlowTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: PairingFlow`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/PairingFlow.kt`:

```kotlin
package com.edgewatcher.domain

enum class PairingFailure {
    /** 期限切れ・消費済み。Web で QR を再発行するしかない。 */
    UNUSABLE,

    /** リトライを尽くしても引けなかった。 */
    NOT_FOUND,

    /** サーバに届かない。QR の問題ではない。 */
    UNREACHABLE,

    REJECTED,

    /** 資格情報を保存できなかった。 */
    STORAGE_FAILED,
}

sealed interface PairingStep {
    /** 自分宛ての QR ではない。スキャンを続ける。 */
    data object NotOurQr : PairingStep

    data object Paired : PairingStep

    data class Failed(val reason: PairingFailure) : PairingStep
}

class PairingFlow(
    private val pairing: PairingService,
    private val credentials: CredentialStore,
    private val sessions: SessionManager,
) {
    suspend fun consume(payload: String, deviceInfo: DeviceInfo): PairingStep {
        val code = pairing.codeFrom(payload) ?: return PairingStep.NotOurQr

        val outcome = pairing.pair(code, deviceInfo)
        val paired = when (outcome) {
            is PairingOutcome.Paired -> outcome
            PairingOutcome.Unusable -> return PairingStep.Failed(PairingFailure.UNUSABLE)
            PairingOutcome.NotFound -> return PairingStep.Failed(PairingFailure.NOT_FOUND)
            PairingOutcome.Unreachable -> return PairingStep.Failed(PairingFailure.UNREACHABLE)
            PairingOutcome.Rejected -> return PairingStep.Failed(PairingFailure.REJECTED)
        }

        credentials.writeCredentials(Credentials(paired.deviceId, paired.deviceSecret))
        // 書けたことを確認してから先へ進む。deviceSecret はここでしか手に入らない。
        if (credentials.readCredentials() == null) {
            return PairingStep.Failed(PairingFailure.STORAGE_FAILED)
        }

        return when (sessions.refresh()) {
            is AuthOutcome.Ready -> PairingStep.Paired
            // 一時的な失敗ならペアリングは成立させる。サービスが後で取り直す。
            AuthOutcome.Retryable -> PairingStep.Paired
            AuthOutcome.CredentialsInvalid, AuthOutcome.NotPaired -> {
                credentials.clear()
                PairingStep.Failed(PairingFailure.UNUSABLE)
            }
        }
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.PairingFlowTest" --no-daemon; Pop-Location
```

期待: PASS（8件）。

- [ ] **Step 5: スキャナ画面を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/ui/ScannerScreen.kt`:

```kotlin
package com.edgewatcher.app.ui

import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import com.edgewatcher.domain.PairingFailure
import com.edgewatcher.domain.UnpairReason
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.common.InputImage
import java.util.concurrent.Executors

/**
 * 未ペアリング時はこの画面のみ。設定項目も履歴も持たない。
 *
 * この時点で Foreground Service は動いていないので、カメラはこの画面が持つ。
 * ペアリング成立時は**この画面を閉じてから**サービスを起動すること。
 */
@Composable
fun ScannerScreen(
    reason: UnpairReason?,
    failure: PairingFailure?,
    inProgress: Boolean,
    onPayload: (String) -> Unit,
    onDismissFailure: () -> Unit,
) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current

    Box(Modifier.fillMaxSize()) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { ctx ->
                val previewView = PreviewView(ctx)
                val future = ProcessCameraProvider.getInstance(ctx)
                future.addListener({
                    val provider = future.get()
                    val preview = Preview.Builder().build()
                        .also { it.surfaceProvider = previewView.surfaceProvider }
                    val scanner = BarcodeScanning.getClient()
                    val analysis = ImageAnalysis.Builder()
                        .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                        .build()
                        .also { it.setAnalyzer(Executors.newSingleThreadExecutor()) { proxy ->
                            val image = proxy.image
                            if (image == null) {
                                proxy.close()
                            } else {
                                scanner.process(
                                    InputImage.fromMediaImage(image, proxy.imageInfo.rotationDegrees)
                                )
                                    .addOnSuccessListener { codes ->
                                        codes.firstNotNullOfOrNull { it.rawValue }?.let(onPayload)
                                    }
                                    .addOnCompleteListener { proxy.close() }
                            }
                        } }
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

        Column(Modifier.padding(24.dp)) {
            if (reason != null) {
                // 黙って戻すと、オーナーには端末の故障と区別がつかない。
                Text(
                    when (reason) {
                        UnpairReason.DISCONNECTED_BY_SERVER ->
                            "この端末は接続を解除されました。再度ペアリングしてください。"
                        UnpairReason.LOGGED_OUT ->
                            "ログアウトしました。再度ペアリングしてください。"
                    }
                )
            }
            Text("QR コードを読み取ってください")
            Text("EdgeWatcher の Web 画面で「端末を追加」を押すと QR が表示されます。")
            if (inProgress) Text("接続しています…")
        }
    }

    if (failure != null) {
        AlertDialog(
            onDismissRequest = onDismissFailure,
            confirmButton = { TextButton(onClick = onDismissFailure) { Text("もう一度スキャン") } },
            title = {
                Text(
                    when (failure) {
                        PairingFailure.UNUSABLE -> "この QR コードは使えません"
                        PairingFailure.NOT_FOUND -> "この QR コードを確認できませんでした"
                        PairingFailure.UNREACHABLE -> "サーバーに接続できません"
                        PairingFailure.REJECTED -> "この QR コードは読み取れません"
                        PairingFailure.STORAGE_FAILED -> "端末に保存できませんでした"
                    }
                )
            },
            text = {
                Text(
                    when (failure) {
                        PairingFailure.UNUSABLE ->
                            "有効期限が切れているか、すでに使用済みです。Web 画面で QR を再発行してください。"
                        PairingFailure.NOT_FOUND ->
                            "Web 画面で QR を再発行して、もう一度読み取ってください。"
                        PairingFailure.UNREACHABLE ->
                            "ネットワークの状態を確認して、もう一度お試しください。"
                        PairingFailure.REJECTED ->
                            "EdgeWatcher の QR コードか確認してください。"
                        PairingFailure.STORAGE_FAILED ->
                            "端末の空き容量を確認して、Web 画面で QR を再発行してください。"
                    }
                )
            },
        )
    }
}
```

- [ ] **Step 6: Commit**

```bash
git add android/domain android/app
git commit -m "feat: ペアリングの手順とスキャナ画面を追加"
```

---

### Task 25: 稼働画面とログアウト

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/RunningActions.kt`
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/LogoutFlow.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/ui/RunningScreen.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/RunningActionsTest.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/LogoutFlowTest.kt`

**Interfaces:**
- Consumes: `RunState`（Task 8）、`SessionManager` / `CredentialStore`（Task 5）、`EdgeWatcherApi`（Task 4）
- Produces:
  - `enum class PrimaryAction { RESUME, FRAME_VIEW, EXIT_PREVIEW }`
  - `object RunningActions { fun primary(run: RunState, previewOn: Boolean): PrimaryAction }`
  - `class LogoutFlow(api, sessions, credentials)` — `suspend fun logout()`
  - `@Composable fun RunningScreen(...)`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/RunningActionsTest.kt`:

```kotlin
package com.edgewatcher.domain

import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals

class RunningActionsTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")

    @Test
    fun `観測中は画角合わせを出す`() {
        assertEquals(
            PrimaryAction.FRAME_VIEW,
            RunningActions.primary(RunState.Observing(now), previewOn = false),
        )
    }

    @Test
    fun `プレビュー中は通常表示へ戻す`() {
        assertEquals(
            PrimaryAction.EXIT_PREVIEW,
            RunningActions.primary(RunState.Observing(now), previewOn = true),
        )
    }

    @Test
    fun `オフラインでも画角合わせは使える`() {
        // 送れていない原因が画角ではないとしても、設置作業は続けられる。
        assertEquals(
            PrimaryAction.FRAME_VIEW,
            RunningActions.primary(RunState.Offline(3, now), previewOn = false),
        )
    }

    @Test
    fun `停止中だけ観測の再開を出す`() {
        // Android 14 以上は再起動後に自動復帰できない。
        // この操作がないとアプリから再開する手段がなくなる。
        assertEquals(
            PrimaryAction.RESUME,
            RunningActions.primary(RunState.Stopped, previewOn = false),
        )
    }

    @Test
    fun `停止中はプレビューより再開を優先する`() {
        assertEquals(
            PrimaryAction.RESUME,
            RunningActions.primary(RunState.Stopped, previewOn = true),
        )
    }
}
```

`android/domain/src/test/kotlin/com/edgewatcher/domain/LogoutFlowTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlinx.coroutines.test.runTest
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class LogoutFlowTest {

    private val now = Instant.parse("2026-09-10T05:00:00Z")

    private fun flow(api: FakeApi, store: InMemoryCredentialStore): LogoutFlow {
        val clock = FixedClock(now)
        return LogoutFlow(api, SessionManager(api, store, clock), store)
    }

    @Test
    fun `サーバへ通知してからローカルを消す`() = runTest {
        val store = InMemoryCredentialStore(
            credentials = Credentials("DEV1", "SECRET1"),
            session = Session("TOKEN1", now.plusSeconds(3600)),
        )
        val api = FakeApi().apply { logoutResults += LogoutResult.Success }

        flow(api, store).logout()

        assertEquals(listOf("TOKEN1"), api.logoutCalls)
        assertNull(store.readCredentials())
    }

    @Test
    fun `通信に失敗してもローカルは消す`() = runTest {
        // サーバ側は PAIRED のまま残り、Web では「応答なし」に見える。
        // オーナーが手動で切断すれば揃う。端末側の実害はない。
        val store = InMemoryCredentialStore(
            credentials = Credentials("DEV1", "SECRET1"),
            session = Session("TOKEN1", now.plusSeconds(3600)),
        )
        val api = FakeApi().apply { logoutResults += LogoutResult.Retryable }

        flow(api, store).logout()

        assertNull(store.readCredentials())
    }

    @Test
    fun `セッションが取れなくてもローカルは消す`() = runTest {
        val store = InMemoryCredentialStore(credentials = Credentials("DEV1", "SECRET1"))
        val api = FakeApi().apply { tokenResults += TokenResult.Retryable }

        flow(api, store).logout()

        assertEquals(0, api.logoutCalls.size)
        assertNull(store.readCredentials())
    }

    @Test
    fun `ペアリングしていなくても落ちない`() = runTest {
        val store = InMemoryCredentialStore()
        val api = FakeApi()

        flow(api, store).logout()

        assertEquals(0, api.logoutCalls.size)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.RunningActionsTest" --tests "com.edgewatcher.domain.LogoutFlowTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: RunningActions`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/RunningActions.kt`:

```kotlin
package com.edgewatcher.domain

enum class PrimaryAction { RESUME, FRAME_VIEW, EXIT_PREVIEW }

object RunningActions {

    fun primary(run: RunState, previewOn: Boolean): PrimaryAction = when {
        run is RunState.Stopped -> PrimaryAction.RESUME
        previewOn -> PrimaryAction.EXIT_PREVIEW
        else -> PrimaryAction.FRAME_VIEW
    }
}
```

`android/domain/src/main/kotlin/com/edgewatcher/domain/LogoutFlow.kt`:

```kotlin
package com.edgewatcher.domain

/**
 * サーバ側の結果はセッション切断と同じで、違いは実行主体だけ。
 * **通信に失敗してもローカルは消す。** 端末側は既に deviceSecret を捨てているため、
 * サーバに PAIRED が残っても実害はなく、Web から手動で切断すれば揃う。
 */
class LogoutFlow(
    private val api: EdgeWatcherApi,
    private val sessions: SessionManager,
    private val credentials: CredentialStore,
) {
    suspend fun logout() {
        val auth = sessions.currentToken()
        if (auth is AuthOutcome.Ready) {
            api.logout(auth.sessionToken)
        }
        credentials.clear()
    }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.RunningActionsTest" --tests "com.edgewatcher.domain.LogoutFlowTest" --no-daemon; Pop-Location
```

期待: PASS（9件）。

- [ ] **Step 5: 稼働画面を書く**

`android/app/src/main/kotlin/com/edgewatcher/app/ui/RunningScreen.kt`:

```kotlin
package com.edgewatcher.app.ui

import androidx.camera.core.Preview
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.weight
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.edgewatcher.domain.PrimaryAction
import com.edgewatcher.domain.RunState
import com.edgewatcher.domain.RunningActions
import com.edgewatcher.domain.StatusLine
import java.time.ZoneId

/**
 * 通常時は画像を一切表示しない。観測画像を見る場所は Web に一本化する。
 * ライブプレビューだけが例外で、それは閲覧ではなく設置作業のための道具だから。
 */
@Composable
fun RunningScreen(
    run: RunState,
    intervalMinutes: Int,
    previewOn: Boolean,
    bufferTrimmed: Boolean,
    showLogoutConfirm: Boolean,
    onPrimaryAction: (PrimaryAction) -> Unit,
    onRequestLogout: () -> Unit,
    onConfirmLogout: () -> Unit,
    onCancelLogout: () -> Unit,
    onPreviewSurface: (Preview.SurfaceProvider?) -> Unit,
) {
    val status = StatusLine.of(run, intervalMinutes, ZoneId.systemDefault())
    val action = RunningActions.primary(run, previewOn)

    Column(Modifier.fillMaxSize().padding(24.dp)) {
        if (bufferTrimmed) {
            Text("保存できる上限に達したため、古い画像から削除しています。")
        }

        if (previewOn) {
            AndroidView(
                modifier = Modifier.fillMaxWidth().aspectRatio(3f / 4f),
                factory = { ctx ->
                    PreviewView(ctx).also { onPreviewSurface(it.surfaceProvider) }
                },
            )
        }

        Text(status.title)
        Text(status.detail)

        Button(
            onClick = { onPrimaryAction(action) },
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
        ) {
            Text(
                when (action) {
                    PrimaryAction.RESUME -> "観測を再開"
                    PrimaryAction.FRAME_VIEW -> "画角を合わせる"
                    PrimaryAction.EXIT_PREVIEW -> "通常表示に戻す"
                }
            )
        }

        Spacer(Modifier.weight(1f))

        OutlinedButton(onClick = onRequestLogout, modifier = Modifier.fillMaxWidth()) {
            Text("ログアウト")
        }
    }

    if (showLogoutConfirm) {
        // Web のログアウトは確認を挟まないが、端末側は挟む。
        // 復帰には QR の再発行と端末への物理的な操作が要る。
        AlertDialog(
            onDismissRequest = onCancelLogout,
            title = { Text("ログアウトしますか?") },
            text = {
                Text(
                    "この端末の接続を解除します。観測は停止します。\n" +
                        "再開するには、Web 画面で QR コードを再発行してもう一度読み取らせる必要があります。"
                )
            },
            confirmButton = { TextButton(onClick = onConfirmLogout) { Text("ログアウトする") } },
            dismissButton = { TextButton(onClick = onCancelLogout) { Text("キャンセル") } },
        )
    }
}
```

- [ ] **Step 6: Commit**

```bash
git add android/domain android/app
git commit -m "feat: 稼働画面とログアウトを追加"
```

---

### Task 26: 画面の接続

**Files:**
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/ui/MainActivity.kt`
- Test: なし（画面の組み立てのみ。判断はすべて Task 23〜25 でテスト済み）

**Interfaces:**
- Consumes: `AppContainer`（Task 22）、`PermissionScreen` / `ScannerScreen` / `RunningScreen`（Task 23〜25）
- Produces: 起動から観測開始までが1本につながった状態

- [ ] **Step 1: `MainActivity` を書く**

この Task には新しい判断が無い。`AppState` と `PermissionGate` の分岐に従って画面を選び、ボタンの操作をサービスと `PairingFlow` / `LogoutFlow` に渡すだけである。

```kotlin
package com.edgewatcher.app.ui

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import com.edgewatcher.app.AppContainer
import com.edgewatcher.app.service.ObservationService
import com.edgewatcher.domain.AppState
import com.edgewatcher.domain.LogoutFlow
import com.edgewatcher.domain.PairingFailure
import com.edgewatcher.domain.PairingFlow
import com.edgewatcher.domain.PairingStep
import com.edgewatcher.domain.PermissionGate
import com.edgewatcher.domain.Permissions
import com.edgewatcher.domain.PrimaryAction
import com.edgewatcher.domain.RequiredPermission
import com.edgewatcher.domain.UnpairReason
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private lateinit var container: AppContainer
    private lateinit var pairingFlow: PairingFlow
    private lateinit var logoutFlow: LogoutFlow

    private var gate by mutableStateOf<PermissionGate>(PermissionGate.NeedsRequest(emptySet()))

    private val requestPermissions =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
            refreshGate()
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        container = AppContainer.of(this)
        pairingFlow = PairingFlow(container.pairing, container.credentials, container.sessions)
        logoutFlow = LogoutFlow(container.api, container.sessions, container.credentials)
        refreshGate()

        setContent {
            val state by container.state.collectAsState()
            var failure by remember { mutableStateOf<PairingFailure?>(null) }
            var pairingInProgress by remember { mutableStateOf(false) }
            var previewOn by remember { mutableStateOf(false) }
            var showLogoutConfirm by remember { mutableStateOf(false) }

            if (gate != PermissionGate.Granted) {
                PermissionScreen(
                    gate = gate,
                    onRequest = { requestPermissions.launch(androidPermissions()) },
                    onOpenSettings = { openAppSettings() },
                )
                return@setContent
            }

            when (val current = state) {
                is AppState.Unpaired -> ScannerScreen(
                    reason = current.reason,
                    failure = failure,
                    inProgress = pairingInProgress,
                    onPayload = { payload ->
                        if (pairingInProgress) return@ScannerScreen
                        pairingInProgress = true
                        lifecycleScope.launch {
                            when (val step = pairingFlow.consume(payload, container.deviceInfo)) {
                                PairingStep.NotOurQr -> pairingInProgress = false
                                PairingStep.Paired -> {
                                    // スキャナを閉じてからサービスを起動する。
                                    // カメラは同時に2つ開けない。
                                    pairingInProgress = false
                                    container.publish(com.edgewatcher.domain.RunState.Stopped)
                                    ObservationService.start(this@MainActivity)
                                }
                                is PairingStep.Failed -> {
                                    pairingInProgress = false
                                    failure = step.reason
                                }
                            }
                        }
                    },
                    onDismissFailure = { failure = null },
                )

                is AppState.Paired -> RunningScreen(
                    run = current.run,
                    intervalMinutes = container.settings.intervalMinutes(),
                    previewOn = previewOn,
                    bufferTrimmed = container.bufferTrimmed.collectAsState().value,
                    showLogoutConfirm = showLogoutConfirm,
                    onPrimaryAction = { action ->
                        when (action) {
                            PrimaryAction.RESUME -> ObservationService.start(this)
                            PrimaryAction.FRAME_VIEW -> previewOn = true
                            PrimaryAction.EXIT_PREVIEW -> {
                                previewOn = false
                                // 畳んだらカメラも閉じる。開いたままにすると発熱する。
                                boundService?.setPreview(null)
                            }
                        }
                    },
                    onRequestLogout = { showLogoutConfirm = true },
                    onConfirmLogout = {
                        showLogoutConfirm = false
                        lifecycleScope.launch {
                            logoutFlow.logout()
                            ObservationService.stop(this@MainActivity)
                            container.publishUnpaired(UnpairReason.LOGGED_OUT)
                        }
                    },
                    onCancelLogout = { showLogoutConfirm = false },
                    // カメラの所有権はサービスが持つ。画面は Surface を渡すだけ。
                    onPreviewSurface = { provider -> boundService?.setPreview(provider) },
                )
            }
        }
    }

    override fun onResume() {
        super.onResume()
        refreshGate()
    }

    private fun androidPermissions(): Array<String> = buildList {
        add(Manifest.permission.CAMERA)
        add(Manifest.permission.ACCESS_FINE_LOCATION)
        if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
    }.toTypedArray()

    private fun refreshGate() {
        val granted = buildSet {
            if (isGranted(Manifest.permission.CAMERA)) add(RequiredPermission.CAMERA)
            if (isGranted(Manifest.permission.ACCESS_FINE_LOCATION)) add(RequiredPermission.LOCATION)
            if (Build.VERSION.SDK_INT < 33 || isGranted(Manifest.permission.POST_NOTIFICATIONS)) {
                add(RequiredPermission.NOTIFICATIONS)
            }
        }
        val blocked = buildSet {
            if (!isGranted(Manifest.permission.CAMERA) &&
                !shouldShowRequestPermissionRationale(Manifest.permission.CAMERA)
            ) add(RequiredPermission.CAMERA)
            if (!isGranted(Manifest.permission.ACCESS_FINE_LOCATION) &&
                !shouldShowRequestPermissionRationale(Manifest.permission.ACCESS_FINE_LOCATION)
            ) add(RequiredPermission.LOCATION)
        }
        gate = Permissions.gate(granted, blocked, Build.VERSION.SDK_INT)
    }

    private fun isGranted(permission: String) =
        ContextCompat.checkSelfPermission(this, permission) == PackageManager.PERMISSION_GRANTED

    private fun openAppSettings() {
        startActivity(
            Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", packageName, null))
        )
    }
}
```

**サービスへのバインド。** `boundService` を上のコードで使っているので、`MainActivity` に次を足す。

```kotlin
    private var boundService: ObservationService? = null

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName?, binder: IBinder?) {
            boundService = (binder as? ObservationService.LocalBinder)?.service()
        }

        override fun onServiceDisconnected(name: ComponentName?) {
            boundService = null
        }
    }

    override fun onStart() {
        super.onStart()
        bindService(Intent(this, ObservationService::class.java), connection, 0)
    }

    override fun onStop() {
        // 画面を離れたらプレビューを畳む。カメラを開いたままにすると発熱する。
        boundService?.setPreview(null)
        unbindService(connection)
        boundService = null
        super.onStop()
    }
```

`bindService` の第3引数を `0` にしているのは、**バインドでサービスを起動させないため**。観測の開始はペアリング完了と `[観測を再開]` だけが行う。

`ObservationService` 側に `LocalBinder` を足す。

```kotlin
    inner class LocalBinder : Binder() {
        fun service(): ObservationService = this@ObservationService
    }

    private val binder = LocalBinder()

    override fun onBind(intent: Intent): IBinder {
        super.onBind(intent)
        return binder
    }
```

`LifecycleService.onBind` は `IBinder?` を返すので、`super.onBind(intent)` を呼んだうえで自前の binder を返すこと。

- [ ] **Step 2: ビルドと全テストを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test :app:testMockDebugUnitTest :app:assembleMockDebug --no-daemon; Pop-Location
```

期待: 全テスト PASS、`BUILD SUCCESSFUL`。

- [ ] **Step 3: 実機に入れて起動することを確認する**

```powershell
Push-Location android; .\gradlew.bat :app:installMockDebug --no-daemon; Pop-Location
adb shell am start -n com.edgewatcher.mock/com.edgewatcher.app.ui.MainActivity
```

期待: 権限ゲートが表示される。

- [ ] **Step 4: Commit**

```bash
git add android/app
git commit -m "feat(app): 画面の接続とサービスへのバインドを追加"
```

---

## フェーズ6: 再起動後の復帰と設置時の案内

### Task 27: 再起動後の復帰

**実機は Android 16 なので、13 以下の自動復帰経路は実機では確かめられない。** Robolectric の `@Config(sdk = ...)` で API レベルを切り替えて、両方の分岐をここで押さえる。

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/BootPolicy.kt`
- Create: `android/app/src/main/kotlin/com/edgewatcher/app/service/BootReceiver.kt`
- Modify: `android/app/src/main/AndroidManifest.xml`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/BootPolicyTest.kt`
- Test: `android/app/src/test/kotlin/com/edgewatcher/app/service/BootReceiverTest.kt`

**Interfaces:**
- Consumes: `Notifications`（Task 22）、`ObservationService`（Task 21）
- Produces:
  - `enum class BootAction { START_SERVICE, NOTIFY_RECOVERY, NOTHING }`
  - `object BootPolicy { fun actionFor(sdkInt: Int, paired: Boolean): BootAction }`
  - `class BootReceiver : BroadcastReceiver`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/BootPolicyTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class BootPolicyTest {

    @Test
    fun `Android 13 以下ならサービスを直接開始する`() {
        // 古い端末ほど完全自動で動く、という皮肉な結果になる。
        // 余剰端末の活用というコンセプトとは相性が良い。
        assertEquals(BootAction.START_SERVICE, BootPolicy.actionFor(sdkInt = 33, paired = true))
        assertEquals(BootAction.START_SERVICE, BootPolicy.actionFor(sdkInt = 26, paired = true))
    }

    @Test
    fun `Android 14 以上では通知を出すだけにする`() {
        // バックグラウンドから camera タイプの FGS は開始できず、
        // BOOT_COMPLETED の受信はバックグラウンド起動に当たる。
        assertEquals(BootAction.NOTIFY_RECOVERY, BootPolicy.actionFor(sdkInt = 34, paired = true))
        assertEquals(BootAction.NOTIFY_RECOVERY, BootPolicy.actionFor(sdkInt = 36, paired = true))
    }

    @Test
    fun `ペアリングしていなければ何もしない`() {
        // 未ペアリングの端末を起こしても、QR スキャナが開くだけで意味がない。
        assertEquals(BootAction.NOTHING, BootPolicy.actionFor(sdkInt = 33, paired = false))
        assertEquals(BootAction.NOTHING, BootPolicy.actionFor(sdkInt = 36, paired = false))
    }
}
```

`android/app/src/test/kotlin/com/edgewatcher/app/service/BootReceiverTest.kt`:

```kotlin
package com.edgewatcher.app.service

import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import androidx.test.core.app.ApplicationProvider
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

@RunWith(RobolectricTestRunner::class)
class BootReceiverTest {

    private fun boot(context: Context) {
        BootReceiver().onReceive(context, Intent(Intent.ACTION_BOOT_COMPLETED))
    }

    private fun pair(context: Context) {
        com.edgewatcher.app.AppContainer.of(context).credentials
            .writeCredentials(com.edgewatcher.domain.Credentials("DEV1", "SECRET1"))
    }

    @Test
    @Config(sdk = [33])
    fun `Android 13 では観測サービスを開始する`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        pair(context)

        boot(context)

        val started = shadowOf(context as android.app.Application).nextStartedService
        assertNotNull(started)
        assertEquals(ACTION_START, started.action)
    }

    @Test
    @Config(sdk = [36])
    fun `Android 16 では通知だけを出す`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        pair(context)

        boot(context)

        assertNull(shadowOf(context as android.app.Application).nextStartedService)
        val manager = context.getSystemService(NotificationManager::class.java)
        assertEquals(1, shadowOf(manager).allNotifications.size)
    }

    @Test
    @Config(sdk = [36])
    fun `ペアリングしていなければ何もしない`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        com.edgewatcher.app.AppContainer.of(context).credentials.clear()

        boot(context)

        assertNull(shadowOf(context as android.app.Application).nextStartedService)
        val manager = context.getSystemService(NotificationManager::class.java)
        assertEquals(0, shadowOf(manager).allNotifications.size)
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.BootPolicyTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: BootPolicy`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/BootPolicy.kt`:

```kotlin
package com.edgewatcher.domain

enum class BootAction { START_SERVICE, NOTIFY_RECOVERY, NOTHING }

object BootPolicy {

    /** Android 14（API 34）以上では `camera` タイプの FGS をバックグラウンドから開始できない。 */
    fun actionFor(sdkInt: Int, paired: Boolean): BootAction = when {
        !paired -> BootAction.NOTHING
        sdkInt >= 34 -> BootAction.NOTIFY_RECOVERY
        else -> BootAction.START_SERVICE
    }
}
```

`android/app/src/main/kotlin/com/edgewatcher/app/service/BootReceiver.kt`:

```kotlin
package com.edgewatcher.app.service

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import com.edgewatcher.app.AppContainer
import com.edgewatcher.domain.BootAction
import com.edgewatcher.domain.BootPolicy

class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED) return

        val container = AppContainer.of(context)
        val action = BootPolicy.actionFor(
            sdkInt = Build.VERSION.SDK_INT,
            paired = container.credentials.readCredentials() != null,
        )

        when (action) {
            BootAction.START_SERVICE -> ObservationService.start(context)
            BootAction.NOTIFY_RECOVERY -> {
                // 通知をタップするとアプリが前面に出て、そこからサービスを開始できる。
                context.getSystemService(NotificationManager::class.java)
                    .notify(Notifications.RECOVERY_ID, container.notifications.recoveryRequired())
            }
            BootAction.NOTHING -> Unit
        }
    }
}
```

`AndroidManifest.xml` の `<application>` 内に追記:

```xml
        <receiver
            android:name=".service.BootReceiver"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.BOOT_COMPLETED" />
            </intent-filter>
        </receiver>
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.BootPolicyTest" --no-daemon; Pop-Location
Push-Location android; .\gradlew.bat :app:testMockDebugUnitTest --tests "com.edgewatcher.app.service.BootReceiverTest" --no-daemon; Pop-Location
```

期待: PASS（3件 / 3件）。**Android 13 の自動復帰経路は、ここでしか検証できない。**

- [ ] **Step 5: `START_STICKY` の再生成が失敗したときの受け皿を足す**

`ObservationService.startObserving()` を、Foreground の開始に失敗した場合に停止状態へ落とすように包む。

```kotlin
    private fun startObserving() {
        try {
            startForeground(
                Notifications.ONGOING_ID,
                container.notifications.ongoing(RunState.Observing(container.settings.lastUploadAt())),
            )
        } catch (e: Exception) {
            // Android 14 以上では START_STICKY による再生成も
            // バックグラウンド起動に当たり、camera 型の開始が拒否されうる。
            // 黙って消えると「なぜか止まる」になるので、通知で気づかせる。
            container.publish(RunState.Stopped)
            getSystemService(NotificationManager::class.java)
                .notify(Notifications.RECOVERY_ID, container.notifications.recoveryRequired())
            stopSelf()
            return
        }
        acquireWakeLock()
        runCycle()
    }
```

`import android.app.NotificationManager` を追加する。

- [ ] **Step 6: Commit**

```bash
git add android/domain android/app
git commit -m "feat: 再起動後の復帰と再生成失敗時の受け皿を追加"
```

---

### Task 28: バッテリー最適化と正確なアラームの案内

**Files:**
- Create: `android/domain/src/main/kotlin/com/edgewatcher/domain/SetupHints.kt`
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/ui/RunningScreen.kt`
- Modify: `android/app/src/main/kotlin/com/edgewatcher/app/ui/MainActivity.kt`
- Test: `android/domain/src/test/kotlin/com/edgewatcher/domain/SetupHintsTest.kt`

**Interfaces:**
- Consumes: なし
- Produces:
  - `enum class SetupHint { BATTERY_OPTIMIZATION, EXACT_ALARM }`
  - `object SetupHints { fun needed(batteryOptimized: Boolean, exactAlarmAllowed: Boolean, sdkInt: Int): Set<SetupHint> }`

- [ ] **Step 1: 失敗するテストを書く**

`android/domain/src/test/kotlin/com/edgewatcher/domain/SetupHintsTest.kt`:

```kotlin
package com.edgewatcher.domain

import kotlin.test.Test
import kotlin.test.assertEquals

class SetupHintsTest {

    @Test
    fun `どちらも整っていれば案内しない`() {
        assertEquals(
            emptySet(),
            SetupHints.needed(batteryOptimized = false, exactAlarmAllowed = true, sdkInt = 36),
        )
    }

    @Test
    fun `バッテリー最適化の対象なら案内する`() {
        // メーカー独自の省電力機構が Foreground Service を停止させることが多く、
        // 案内しないと「なぜか止まる」の主要因になる。
        assertEquals(
            setOf(SetupHint.BATTERY_OPTIMIZATION),
            SetupHints.needed(batteryOptimized = true, exactAlarmAllowed = true, sdkInt = 36),
        )
    }

    @Test
    fun `正確なアラームが許可されていなければ案内する`() {
        assertEquals(
            setOf(SetupHint.EXACT_ALARM),
            SetupHints.needed(batteryOptimized = false, exactAlarmAllowed = false, sdkInt = 36),
        )
    }

    @Test
    fun `両方必要ならどちらも案内する`() {
        assertEquals(
            setOf(SetupHint.BATTERY_OPTIMIZATION, SetupHint.EXACT_ALARM),
            SetupHints.needed(batteryOptimized = true, exactAlarmAllowed = false, sdkInt = 36),
        )
    }

    @Test
    fun `Android 11 以下では正確なアラームの許可は存在しない`() {
        // SCHEDULE_EXACT_ALARM の許可が要るのは Android 12（API 31）以降。
        assertEquals(
            emptySet(),
            SetupHints.needed(batteryOptimized = false, exactAlarmAllowed = false, sdkInt = 30),
        )
    }
}
```

- [ ] **Step 2: テストを実行して失敗を確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.SetupHintsTest" --no-daemon; Pop-Location
```

期待: コンパイルエラー（`Unresolved reference: SetupHints`）で FAIL。

- [ ] **Step 3: 最小の実装を書く**

`android/domain/src/main/kotlin/com/edgewatcher/domain/SetupHints.kt`:

```kotlin
package com.edgewatcher.domain

enum class SetupHint { BATTERY_OPTIMIZATION, EXACT_ALARM }

/**
 * 権限とは別枠の案内。**強制はしない。**
 * ペアリング直後に一度出し、以後は稼働画面から再度開けるようにする。
 */
object SetupHints {

    fun needed(batteryOptimized: Boolean, exactAlarmAllowed: Boolean, sdkInt: Int): Set<SetupHint> =
        buildSet {
            if (batteryOptimized) add(SetupHint.BATTERY_OPTIMIZATION)
            // SCHEDULE_EXACT_ALARM の許可が要るのは Android 12 以降。
            if (sdkInt >= 31 && !exactAlarmAllowed) add(SetupHint.EXACT_ALARM)
        }
}
```

- [ ] **Step 4: テストが通ることを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test --tests "com.edgewatcher.domain.SetupHintsTest" --no-daemon; Pop-Location
```

期待: PASS（5件）。

- [ ] **Step 5: 画面に案内を足す**

`RunningScreen` に `hints: Set<SetupHint>` と `onOpenHint: (SetupHint) -> Unit` を引数として追加し、状態行の下に案内を出す。

```kotlin
        hints.forEach { hint ->
            when (hint) {
                SetupHint.BATTERY_OPTIMIZATION -> {
                    Text("電池の最適化を解除してください")
                    Text("端末の省電力機能によって観測が止まることがあります。")
                }
                SetupHint.EXACT_ALARM -> {
                    Text("アラームの許可が必要です")
                    Text("許可がないと、送信の間隔が正確でなくなることがあります。")
                }
            }
            OutlinedButton(onClick = { onOpenHint(hint) }, modifier = Modifier.fillMaxWidth()) {
                Text("設定を開く")
            }
        }
```

`MainActivity` に、状態の取得と設定画面への導線を足す。

```kotlin
    private fun currentHints(): Set<SetupHint> {
        val power = getSystemService(PowerManager::class.java)
        val alarms = getSystemService(AlarmManager::class.java)
        return SetupHints.needed(
            batteryOptimized = !power.isIgnoringBatteryOptimizations(packageName),
            exactAlarmAllowed = Build.VERSION.SDK_INT < 31 || alarms.canScheduleExactAlarms(),
            sdkInt = Build.VERSION.SDK_INT,
        )
    }

    private fun openHint(hint: SetupHint) {
        val intent = when (hint) {
            SetupHint.BATTERY_OPTIMIZATION ->
                Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
            SetupHint.EXACT_ALARM ->
                Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM,
                    Uri.fromParts("package", packageName, null))
        }
        startActivity(intent)
    }
```

`RunningScreen` の呼び出しに `hints = currentHints()` と `onOpenHint = ::openHint` を渡す。`onResume` で再評価されるよう、`gate` と同じく `mutableStateOf` に持たせる。

- [ ] **Step 6: 全体のビルドとテストを確認する**

```powershell
Push-Location android; .\gradlew.bat :domain:test :app:testMockDebugUnitTest :app:assembleMockDebug --no-daemon; Pop-Location
```

期待: 全テスト PASS、`BUILD SUCCESSFUL`。

- [ ] **Step 7: Commit**

```bash
git add android/domain android/app
git commit -m "feat: バッテリー最適化と正確なアラームの案内を追加"
```

---

## フェーズ7: 実機確認と docs の更新

### Task 29: 実機スモーク

**Files:** なし（動作確認のみ）

**Interfaces:**
- Consumes: 完成したアプリ
- Produces: 各手順の結果報告

- [ ] **Step 1: mock フレーバーを実機に入れる**

```powershell
adb devices
Push-Location android; .\gradlew.bat :app:installMockDebug --no-daemon; Pop-Location
```

- [ ] **Step 2: Web をモックモードで起動して QR を出す**

```powershell
Push-Location web; npm install; npm run dev; Pop-Location
```

`VITE_API_BASE_URL` を設定しなければモックモードになる（`web/src/config/env.ts`）。ブラウザで「端末を追加」を押し、QR を表示する。

- [ ] **Step 3: 手順 1〜5、8、10 を実行して結果を記録する**

| # | 手順 | 期待 |
| --- | --- | --- |
| 1 | QR を読む | 「接続しました」になり、稼働画面へ移る |
| 2 | 権限を拒否して起動し直す | 「設定を開く」だけが出る。縮退モードに入らない |
| 3 | `[画角を合わせる]` を2回押す | プレビューが出て、もう一度で戻る |
| 4 | プレビュー中に5分待つ | 通知の「最終送信」が更新される |
| 5 | 画面を消して15分放置 | 通知の「最終送信」が3回更新されている |
| 8 | 端末を再起動 | 「EdgeWatcher が停止しています」の通知が出る。タップで再開する |
| 10 | ログアウト | 確認ダイアログの後、理由付きで QR スキャナへ戻る |

**結果は正直に報告する。** 落ちた手順は落ちたと、観測した挙動とともに報告する。

- [ ] **Step 4: live バックエンドが用意できる場合のみ、手順 6・7・9 を実行する**

```powershell
$env:EW_API_BASE_URL = "<AWS dev の API Gateway URL>"
Push-Location android; .\gradlew.bat :app:installLiveDebug --no-daemon; Pop-Location
```

| # | 手順 | 期待 |
| --- | --- | --- |
| 6 | 機内モードにして10分待つ | 「オフライン / N件待機中」に変わる |
| 7 | 機内モードを解除 | Web のダッシュボードに**最新の画像が先に**現れる |
| 9 | Web から端末を削除 | 次の送信で「この端末は接続を解除されました」と出て QR スキャナへ戻る |

**接続先が用意できていない場合は、この3手順を保留として明示的に報告する。** 「やっていない」を「問題なかった」と混ぜない。

- [ ] **Step 5: 手順 8 の Android 13 以下の経路について報告する**

実機が Android 16 のため、自動復帰の経路は実機で確認できない。Task 27 の `BootReceiverTest`（`@Config(sdk = [33])`）が緑であることをもって代替とし、**実機未確認であることを明記して報告する。**

---

### Task 30: docs の更新

**Files:**
- Modify: `docs/01-openquestion.md`
- Modify: `docs/04-native.md`
- Modify: `docs/mocks/native-mocks.html`

**Interfaces:**
- Consumes: 実装で確定した値
- Produces: docs と実装の一致

- [ ] **Step 1: `01-openquestion.md` を更新する**

| 項目 | 変更 |
| --- | --- |
| APP-01 | 一覧表の `open` → `decided`。本文に「**決定(2026-09-10)**: 初期値は5分。バッファ上限の288件が『5分間隔で24時間相当』として決まっているため、初期値を5分に置くと最も負荷の高い条件が既定になる」を追記 |
| APP-03 | `pending` → `decided`。「**決定(2026-09-10)**: 本画像は長辺1600px・品質80、サムネイルは長辺160px・品質70、いずれも JPEG。合計が 4,500,000 バイトを超える場合は品質を 80 → 65 → 50 と落とし、それでも超える場合は長辺を1280pxにする」を追記 |
| APP-04 | `pending` → `decided`。「**決定(2026-09-10)**: CameraX / `FusedLocationProviderClient.lastLocation` / 計装テストは書かず JVM 単体テストと Robolectric で担保する。詳細は `docs/superpowers/specs/2026-09-10-android-app-design.md`」を追記 |

- [ ] **Step 2: `04-native.md` を更新する**

- §1.9 に追記: 「**実際にネットワーク上を流れるのは 403 である。** オーソライザが `isAuthorized:false` を返すと API Gateway が 403 に変換するため。端末は 401 と 403 の両方を同じ入口として扱う（`engineering/api.yml` 冒頭）」
- §1.5 の「送信間隔は 5 / 10 / 15 分」に「**初期値は5分**」を追記
- §5 の未確定事項の表から APP-01 / APP-03 / APP-04 の行を削除し、代わりに「実装レベルの設計は `docs/superpowers/specs/2026-09-10-android-app-design.md` に確定済み」と記す
- §2.3 に「上限は 288件 または 524,288,000 バイト（500MiB）」と単位を明記

- [ ] **Step 3: `native-mocks.html` から端末名を削る**

`POST /device/pair` の応答に端末名は含まれないため、モックが実装と食い違っている。

| 箇所 | 変更前 | 変更後 |
| --- | --- | --- |
| `paired` シナリオの見出し | `「玄関」として接続しました` | `接続しました` |
| `notif-normal` の通知 | `最終送信 14:32 ・ 5分間隔 ・ 玄関` | `最終送信 14:32 ・ 5分間隔` |
| `notif-offline` の通知 | `12件待機中 ・ 最終送信 11:48 ・ 玄関` | `12件待機中 ・ 最終送信 11:48` |

あわせて `paired` シナリオの `note` に「`POST /device/pair` の応答は `{deviceId, deviceSecret}` のみで、端末名は端末に渡らない。名前の表示は Web に一本化する」を追記する。

- [ ] **Step 4: Commit**

```bash
git add docs
git commit -m "docs: Android アプリの実装で確定した値を反映"
```

---

## 完了条件

1. `./gradlew :domain:test :app:testMockDebugUnitTest` が全件緑
2. `./gradlew :app:assembleMockDebug` と、`EW_API_BASE_URL` を与えた `:app:assembleLiveDebug` が成功
3. Task 29 の実機スモークの結果が、保留分も含めて報告されている
4. Task 30 の docs 更新がコミットされている
