# EdgeWatcher Android アプリ 設計

作成: 2026-09-10（2026-09-10 に全面改訂）
ステータス: 確定 / 実装未着手
上位文書: `docs/04-native.md`（機能要件）、`docs/engineering/api.yml`（API 契約）
モック: `docs/mocks/native-mocks.html`

`04-native.md` が「何を作るか」を定めるのに対し、本書は「どう作るか」を定める。
両者が食い違う箇所は本書が優先し、`04-native.md` 側を後追いで修正する（§12）。

> **改訂の記録**: 本書は同名の初版（2026-09-10 04:48 コミット）を差し替えたものである。
> 初版から変わったのは DI・HTTP クライアント・モジュール構成・サムネイルの寸法・
> テスト方針の5点で、いずれも「とにかくアプリを完成させる」という優先順位の変更に基づく。
> **初版を前提に書かれた `docs/superpowers/plans/2026-09-10-android-app.md`（7,971行）は
> 破棄し、本書から作り直す。**

---

## 0. 進め方（この設計に対する制約）

**本プロジェクトはサブエージェント駆動で開発する。**

- 実装計画は独立して着手できるタスクに分割し、**各タスクを1つのサブエージェントに担当させる**
- **実装は Haiku または Sonnet に委譲する。Opus は設計・計画・レビュー・統合判断にのみ使う。**
  これはトークン予算の制約であり、遵守すること
- タスクの粒度は「担当するサブエージェントが本書の該当セクションと対象ファイルだけを読めば
  完結する」ところまで落とす。**前後のタスクの文脈を引き継がないと書けないタスクは、
  分割が足りていない**
- 各タスクの完了条件は、そのタスク単体で機械的に判定できる形（コンパイルが通る /
  指定したテストが緑になる）で書く。「動くこと」のような人間の判断を要する条件は
  §11 の受け入れ手順にまとめ、個々のタスクの完了条件にしない

この制約は品質の妥協ではなく、**本書の具体性に対する要求**である。
委譲できるだけの具体性が本書に無ければ、その部分は設計が終わっていない。

### 0.1 完成を最優先する

機能は `04-native.md` に書かれたものだけを作る。テストは §10 の4本だけを書く。
`04-native.md` に無いものを足さない。迷ったら足さない。

---

## 1. 技術選定

| 項目 | 選定 |
| --- | --- |
| 言語 / UI | Kotlin + Jetpack Compose |
| アーキテクチャ | 緩い Clean Architecture（`domain` / `usecase` / `infrastructure` / `presentation`） |
| DI | **Hilt（KSP）。Module は最外装の `di/` にのみ置く** |
| カメラ | **CameraX** + `LifecycleService` |
| QR | **ML Kit `barcode-scanning`（bundled モデル）** を CameraX `ImageAnalysis` 経由で |
| HTTP | **Retrofit + OkHttp + kotlinx.serialization** |
| ローカル DB | Room（メタデータのみ）+ アプリ専用ストレージのファイル（画像） |
| 資格情報 | `EncryptedSharedPreferences` |
| 位置情報 | `FusedLocationProviderClient.lastLocation` |
| 非同期 | Coroutines + Flow |
| ULID | **自前実装**（採番時刻を引数で受け取れる必要があるため。§5.2） |
| `minSdk` / `compileSdk` / `targetSdk` | 26 / 36 / 36（プレビュー SDK は使わない） |
| `applicationId` | `com.edgewatcher` |
| 画面の向き | 縦固定 |

`APP-04` の未確定項目（カメラ API・位置情報の取得方式・テスト方針）は
§1.1・上表・§10 で確定する。

### 1.1 CameraX を選ぶ理由

`04-native.md` §3.2 は「カメラの所有権は Foreground Service が持ち、画面はプレビュー用の
Surface を渡す」と定める。CameraX の `Preview` use case が受け取る `SurfaceProvider` は
まさにこの接点そのものであり、設計がそのまま写像できる。

- `ImageCapture` を常時 bind したまま `Preview` だけを bind / unbind すればプレビューの
  トグルになる。定期撮影は同じ `ProcessCameraProvider` から行われるため、
  「プレビューを開いている間だけ観測に穴が開く」という挙動にならない
- **`Preview` + `ImageCapture` は、CameraX が LEGACY ハードウェアレベルの端末でも
  サポートを保証している組み合わせである。** 余剰端末・格安端末を活用するコンセプト上、
  これは決定的に重要

Camera2 を直接使うと、プレビューのトグルのたびに `CaptureSession` の再構成が必要になり、
API 26 から Android 16 までの端末差分を自分で背負うことになる。要件に低レベル制御を
必要とする箇所は無い。

### 1.2 `lastLocation` を使う理由

`getCurrentLocation` は測位を待つ。`04-native.md` §1.5 は「測位を待たない。最後の既知位置で
十分であり、定点観測デバイスは動かない」と定めているため、`lastLocation` を使う。
取得できなければ `null` のまま送る。位置は必須項目ではない。

---

## 2. レイヤ構成

```
android/
  settings.gradle.kts
  gradle/libs.versions.toml
  app/
    build.gradle.kts
    src/main/AndroidManifest.xml
    src/main/java/com/edgewatcher/
      EdgeWatcherApp.kt              @HiltAndroidApp
      di/                            Hilt Module はここだけに置く
      domain/                        純 Kotlin。android.* を import しない
        model/    DeviceCredentials  Session  PendingObservation
                  ObservationState   IntervalMinutes  StatusLine
        error/    ApiFailure
        port/     CredentialStore  ObservationBuffer  ObservationApi
                  CameraGateway    LocationGateway    JpegEncoder
                  AlarmScheduler   Clock             IdGenerator
      usecase/                       純 Kotlin。domain/port にだけ依存
        PairDeviceUseCase          RefreshSessionUseCase
        CaptureObservationUseCase  UploadNextObservationUseCase
        LogoutDeviceUseCase
      infrastructure/
        api/      EdgeWatcherService（Retrofit）  dto/  AuthInterceptor  ErrorMapper
        store/    EncryptedCredentialStore
        db/       ObservationEntity  ObservationDao  RoomObservationBuffer  ImageFileStore
        camera/   CameraXGateway     AndroidJpegEncoder
        location/ FusedLocationGateway
        service/  ObservationService（LifecycleService）
                  AndroidAlarmScheduler  BootReceiver  Notifications
      presentation/
        MainActivity
        permission/ PermissionScreen
        pairing/    PairingScreen     PairingViewModel
        running/    RunningScreen     RunningViewModel
    src/test/java/com/edgewatcher/    JVM ユニットテスト（§10）
```

### 2.1 唯一の構造上の規約

**`domain` と `usecase` は `android.*` および Android ライブラリを import しない。**

これが守られている限り §10 の4本のテストは JVM 上で即座に回り、エミュレータを必要としない。
逆に、この規約を破らないと書けないロジックが現れたら、それは `infrastructure` に属する。

`ApiFailure` の分類、eviction、送信順序、ULID の採番、画像方針とサイズガード、
状態行の文言はすべて純粋な計算であり、この規約の下に置ける。

`JpegEncoder` をポートとして切ったのは、**「どう符号化するか」の判断と画素の操作を
分けるため**である。長辺・品質・サイズガードの段階（§5.3）は方針であって画像処理ではないので
`domain` が持ち、`infrastructure` は言われた寸法で符号化するだけにする。

### 2.2 画面 ↔ Service の接続: Bound Service

`RunningScreen` は `bindService` で `ObservationService` に接続し、`Binder` 経由で

- `StateFlow<ObservationState>` を購読する（状態表示）
- プレビュー ON のときだけ `Preview.SurfaceProvider` を渡し、OFF で外す

unbind によって参照が自動的に切れるため、Compose が破棄した Surface を Service が
掴み続ける事態が構造的に起こらない。アプリスコープのシングルトンに Surface を預ける方式では、
解放の書き忘れ一つで「プレビューを閉じたのにカメラが掴まれたまま発熱し続ける」という、
屋外設置で最も発見しにくい壊れ方をする。カメラは同時に2つのクライアントが開けない資源なので、
寿命が対で管理される仕組みを当てる。

Service の生存自体は `startForegroundService` が担う。bind はあくまで画面が繋がっている
間だけの付加であり、**画面を閉じても観測は止まらない。**

### 2.3 アップロードキューは Service 内のコルーチンが駆動する

`WorkManager` の `OneTimeWorkRequest` を使えば指数バックオフ・ネットワーク制約・
プロセス死をまたぐ永続化が無料で手に入る（`04-native.md` §3.1 が WorkManager を退けたのは
`PeriodicWork` の15分下限が理由であり、単発の送信には当てはまらない）。

それでも採らない。FGS はどのみちウェイクロックを保持して常駐しており、プロセス死からの
復帰は `AlarmManager` と `START_STICKY` で既に確保されているため、WorkManager の利点は
二重投資になる。一方で `04-native.md` §3.5 が要求する「最新の1枚を先に、以後古い順」という
優先順位を WorkManager のスケジューラに守らせるのは面倒であり、
バッファの真実の源が Room と WorkManager のキューに二重化する。

---

## 3. 確定した未決事項

### 3.1 APP-01: 送信間隔の初期値 = **5分**

送信間隔は Web が決め、端末はアップロード応答の `nextConfig` から受け取る。
しかし**ペアリング直後から最初のアップロード成功までの間、端末は間隔を知る手段を持たない**
（端末向けの設定取得エンドポイントは存在しない）。したがって端末側にブートストラップ値が要る。

**この値は backend の `createDevice` が使う既定値と一致していなければならない。**
ずれると最初の1サイクルだけ端末の実際の動作と Web の表示が食い違う。
現在の backend の既定値は `internal/webapi/webapi.go` の
`defaultInterval = api.IntervalOptions[0]`、すなわち **5分**である。

実装時、この定数の隣に「backend の `defaultInterval` と同一であること」というコメントを
残すこと。片方だけが変更されても機械的には検出できない。

### 3.2 APP-03: 画像仕様

| 項目 | 値 |
| --- | --- |
| フォーマット | JPEG（`api.yml` の `encoding` が `image/jpeg` を指定） |
| 本画像 | **長辺 1600px / 品質 80** |
| サムネイル | **長辺 480px / 品質 75** |
| 合計サイズ上限 | 4,500,000 バイト（超過は 400。リトライ不能） |
| EXIF | 落とす |

本画像は実効 250〜450KB を見込む。上限 4.5MB に対して一桁の余裕があり、
**上限が routine な制約ではなく安全網として機能する**状態を保てる。
288件のバッファはおよそ 100MB 前後になり、500MiB より件数の上限が先に効く。
これは「5分間隔で24時間分を保持する」という `APP-02` の意図どおりの動作である。

**サムネイルは `04-native.md` の暫定値 160px から 480px に引き上げる。**
Web のダッシュボードは端末カードの画像に `latestThumbnailUrl`（端末が送るこのサムネイル）を
使っており（`03-web.md` §1.5）、カード幅は通常 280〜360px あるため 160px では明確にぼやける。
480px なら履歴のコマ列（表示は 80〜120px）にもダッシュボードのカードにも耐える。
1枚 30〜50KB で、288件でも1端末1日あたり十数 MB にしかならない。

### 3.3 APP-04

§1 の表で確定。カメラは CameraX、位置情報は `lastLocation`、テスト方針は §10。

---

## 4. ペアリング

### 4.1 フロー

1. CameraX `Preview` + `ImageAnalysis` に ML Kit を挿し、QR を読む
2. **ペイロードが `ew1:` で始まらなければ黙って無視し、読み取りを続ける。**
   他のアプリの QR に反応しないため。エラー表示も出さない
3. `ew1:` を除いた残りを `pairingCode` として `POST /device/pair` に送る
4. 応答の `deviceSecret` を `EncryptedSharedPreferences` に保存する
5. `POST /device/token` で `sessionToken` と `expiresAt` を取得し、同じく保存する
6. `startForegroundService` で `ObservationService` を開始する
7. バッテリー最適化の除外と `SCHEDULE_EXACT_ALARM` を1度だけ案内する（§4.4）

`deviceInfo` には `model` / `osVersion` / `appVersion` を入れる。表示専用であり、
何の認可判断にも使われない。

### 4.2 失敗の読み分け

`api.yml` の定めるとおりに実装する。**丸めてはならない。**

| status | code | 挙動 |
| --- | --- | --- |
| 404 | `PAIRING_NOT_FOUND` | **1秒間隔でリトライ。初回1回 + リトライ3回 = 最大4リクエスト** |
| 409 | `PAIRING_CODE_CONSUMED` | 即座に「この QR は使えません」 |
| 409 | `PAIRING_CODE_EXPIRED` | 即座に「この QR の有効期限が切れています」 |
| 400 | `VALIDATION_ERROR` | 即座に失敗。リトライしない |

404 だけがリトライ対象なのは、`pairingCode` の逆引きが結果整合な GSI2 を通るためである。
409 を 404 に丸めると絶対に成功しない QR を回し続け、404 を 409 に丸めると
成功するはずのペアリングを諦める。

### 4.3 端末名は画面にも通知にも出さない

**`POST /device/pair` の応答は `deviceId` と `deviceSecret` だけであり、端末名を含まない。**
他に端末名が届く経路も無いため、**端末はそもそも自分の名前を知らない。**

`docs/mocks/native-mocks.html` は `paired` シナリオの見出しと常駐通知2種で端末名を
表示しているが、これは実装不能である。モック側を修正する（§12）。

### 4.4 権限とシステム設定の案内

権限は QR スキャナに入る前に通す。`CAMERA` と `ACCESS_FINE_LOCATION` のどちらかが
恒久的に拒否された場合は、設定を開く導線だけを出す。**縮退モードは持たない。**
カメラか位置情報のどちらかが欠けた時点でこのアプリの目的が成立しないため。

**`ACCESS_BACKGROUND_LOCATION` は要求しない**（`04-native.md` §1.3）。
FGS の `location` タイプが「使用中」を成立させるため不要であり、要求すると Play の審査で
背景位置情報の正当化が必要になる。

権限とは別枠で、ペアリング完了直後に次の2つを1度だけ案内する。どちらも強制はせず、
稼働画面から再度開けるようにする。

- **バッテリー最適化の除外** — メーカー独自の省電力機構が FGS を止める主要因
- **`SCHEDULE_EXACT_ALARM`**（Android 12+）— 無いと `setExactAndAllowWhileIdle` が使えず、
  §5.1 のタイマーが成立しない

---

## 5. 観測パイプライン

### 5.1 タイマー

`04-native.md` §3.0 のとおり、ウェイクロックと `AlarmManager` を二重化する。

| 仕組み | 役割 |
| --- | --- |
| `PARTIAL_WAKE_LOCK` | Service 稼働中は保持し続け、CPU のサスペンドを止める |
| `AlarmManager.setExactAndAllowWhileIdle` | 次回撮影時刻に必ず起こす。プロセス再生成後の復帰点にもなる |

**`Handler.postDelayed` やコルーチンの `delay` を撮影周期の駆動に使ってはならない。**
画面 OFF でサスペンドに入ると発火しないか大幅に遅延する。FGS は「殺されにくい」ことを
保証するだけで「動き続ける」ことを保証しない。

アラームは**撮影のたびに次回分を再設定する**（繰り返しアラームを使わない）。
`nextConfig` による間隔変更が次のサイクルから自然に反映されるため。

Doze は考慮しない。Doze の条件は「画面 OFF + 静止 + **バッテリー駆動**」であり、
常時給電の本プロジェクトでは成立しない。

### 5.2 撮影1周期

```
① capturedAt = Clock.now()
② observationId = IdGenerator.ulid(capturedAt)   ← 採番時刻を引数で受ける
③ CameraGateway.capture()
④ ImagePolicy に従って符号化（§5.3）
⑤ LocationGateway.lastKnown()   ← 測位を待たない。null 可
⑥ 画像2枚をファイルへ、メタデータ1行を Room へ（state = PENDING）
⑦ eviction（§6）
⑧ 次回アラームを再設定
```

**`observationId` の採番時刻は `capturedAt` と厳密に一致させる。**
サーバは日別クエリの範囲を ULID のタイムスタンプ部から導くため、ここがずれると
観測が別の日に並ぶ。`IdGenerator.ulid()` を引数なしで呼べる形にしてはならない。
既存の ULID ライブラリの多くは採番時刻を外から渡せないため、自前で実装する。

**`metadata` に `deviceId` を入れない。** 入れてもサーバは読まない。
端末の同一性はオーソライザだけが決める。

### 5.3 符号化とサイズガード

本画像を長辺1600px・品質80、サムネイルを長辺480px・品質75で符号化する。
**符号化後の合計バイト数を実測し、4,500,000 を超えていたら段階的に落とす。**

```
品質 80 → 65 → 50 の順に本画像を再符号化する
それでも超える場合は、本画像を長辺 1280px・品質 50 で符号化する
```

送っても必ず 400 になるフレームをバッファに積まないため。この判断は `domain` の
`ImagePolicy` が持ち、`infrastructure` の `JpegEncoder` は言われた寸法で符号化するだけにする。

---

## 6. バッファ

**288件 または 524,288,000 バイト（500MiB）のいずれか先に達したら、古いものから削除する**
（`APP-02`）。

判定は撮影のたびに行う。件数と総バイト数の両方を評価し、どちらかを超えていれば
`capturedAt` の古い順に、両方の条件を満たすまで削除する。
削除は Room の行と画像ファイル2枚を**対で**行い、片方だけが残らないようにする。

画像本体をファイルに置き Room にメタデータだけを持つのは、BLOB を SQLite に入れると
DB ファイルが肥大し、削除しても領域が戻りにくいためである（`04-native.md` §3.3）。
バッファは常に書いては消すことを繰り返すため、この性質が効いてくる。

バッファ上限を保持期間より長くしても意味がない。観測レコードの TTL は `capturedAt` を
基準に決まるため、長時間オフラインだった端末が古い画像を送っても、
サーバ到着時点で期限切れになりうる。

---

## 7. 送信キュー

### 7.1 取り出し順

`04-native.md` §3.5 に従う。**最新の未送信1枚を先に送り、そのあと古い順。**

長時間オフラインから復帰したとき単純な FIFO で送ると、Web のダッシュボードに
何時間も前の画像が出続ける。オーナーが最初に知りたいのは「今どうなっているか」である。

### 7.2 リクエスト

`POST /device/uploads`、multipart。パート名は **`image` / `thumbnail` / `metadata`**。
`metadata` は `application/json` で `observationId` と `capturedAt`（RFC 3339）が必須、
`lat` / `lng` は取得できていれば入れる。

`Authorization` ヘッダは **`Bearer` を付けない素のトークン**を送る。

送信前に `Session.expiresAt` を見て、残り5分未満なら先に `POST /device/token` で更新する。
12時間ごとに1往復が無駄になるのを避けるためであり、必須ではないが安い。

### 7.3 応答の扱い

| 応答 | 扱い |
| --- | --- |
| 200 | Room の行と画像ファイルを削除。`nextConfig.intervalMinutes` を保存し次回アラームへ反映 |
| 400 | **捨てる。** 本文に起因する失敗であり、再送しても永久に通らない |
| 413 | **捨てる。** API Gateway のペイロード上限超過。本文は `Error` スキーマではない |
| 401 / 403 | セッション再取得へ（§8） |
| 5xx / 通信断 | 指数バックオフで同じ行を再試行。連続失敗回数 n に対し `min(5 * 2^(n-1), 300)` 秒 |

**リトライ中も定期撮影は止めない。** 撮った画像はバッファに積まれ、上限に達するまで失われない。

`nextConfig` は**必ず適用する**。サーバは設定を push しないため、送信間隔の変更が
オーナーから端末へ届く経路はこれ1本しかない。結果として設定変更は最大1送信サイクル遅れる。

---

## 8. セッションと自己修復 — **401 と 403 の両方**を入口にする

```
アップロードが 401 または 403
  └→ POST /device/token
       ├ 200 → 新しい sessionToken で再送し、以後継続
       ├ 401 → deviceSecret が無効（Web から切断されたか、端末が削除された）
       │        資格情報とバッファを全消去 → Foreground Service を停止
       │        未ペアリング画面へ戻り、理由を画面上に残す
       └ 5xx / 通信断 → 何も消さない。バックオフして後で再試行する
```

### 8.1 `04-native.md` §1.9 の記述は、実装すると壊れる

`04-native.md` §1.9 は「401 を起点とした自己修復」と題し、401 を唯一の入口としている。
**これをそのまま実装すると、セッション失効から永久に回復できない。**

`api.yml` の `deviceSession` の定義により、端末ルートでは:

- **セッションが失効した / status が PAIRED でない / トークンのハッシュが不一致** →
  Lambda オーソライザが拒否し、API Gateway が **403**（`{"message":"Forbidden"}`）を返す
- **`Authorization` ヘッダそのものが無い** → オーソライザは呼ばれず **401**
  （`{"message":"Unauthorized"}`）

日常運用で起きるのは前者、すなわち **403** である。したがって端末は
**401 と 403 の両方**を `POST /device/token` へ流す。

なお `POST /device/token` 自体の失敗はすべて 401 である（404 も 403 も返らない）。
これは `06-auth.md` §5 が 401 を端末の自己修復の唯一の入口と定めているためで、
この一点においては `04-native.md` §1.9 の記述と一致する。

### 8.2 資格情報を消してよい条件

**`POST /device/token` が 401 を返したときだけ、資格情報を消してよい。**

5xx・タイムアウト・ネットワーク断で消してはならない。**通信障害と切断を取り違えると、
一時的に圏外になっただけの端末が資格情報を捨て、復帰には Web からの QR 再発行と
現地への物理的な訪問が必要になる。** 屋外設置の端末でこれが起きると回復コストが跳ね上がる。

### 8.3 未ペアリング画面へ戻すときは理由を残す

> 「この端末は接続を解除されました。再度ペアリングしてください。」

黙って QR スキャナに戻すと、オーナーには端末の故障と区別がつかない。

### 8.4 エラー本文の形が2種類ある

アプリケーション層のエラーは `{"error":{"code","message"}}` だが、
**オーソライザと API Gateway が返す 401 / 403 / 413 は `{"message":"..."}` である。**

パーサはこの2つを両方受け付け、**パースに失敗しても status だけで分類を決められること。**
分類の判断を本文に依存させてはならない。

### 8.5 ログアウト

確認ダイアログを挟んでから `POST /device/logout`（本文なし、204）を呼び、
ローカルの資格情報とバッファを破棄して未ペアリング画面へ戻る。

Web と違って確認を挟むのは、端末は `deviceSecret` を失うため、復帰に Web からの
QR 再発行と端末への物理的な操作が要るからである。取り返しのつきやすさが違う。

**端末からの「削除」は提供しない。** 屋外に常設された端末が物理的に触られただけで
オーナーの管理下からレコードごと消える事態を避ける。

---

## 9. UI・通知・再起動

### 9.1 画面

実質2画面。タブもナビゲーションドロワーも持たない。

| 画面 | 表示条件 | 内容 |
| --- | --- | --- |
| 未ペアリング | 資格情報を持たない | QR スキャナ（全画面） |
| ペアリング済み | 資格情報を持つ | 状態1行、`[画角を合わせる]`、`[ログアウト]` |

通常時は**画像を一切表示しない。** `[画角を合わせる]` を押したときだけライブプレビューに
切り替わり、もう一度押すと戻る（トグル）。**直近に送信した画像のサムネイルも表示しない。**
観測画像を見る場所は Web に一本化する。ライブプレビューだけが例外なのは、
それが閲覧ではなく設置作業のための道具だからである。

**ライブプレビュー中も定期送信は止まらない。** 同じカメラセッションから撮影する（§2.2）。

### 9.2 状態行

画面と常駐通知に**同一の文字列**を1行で出す。文言の生成は `domain` の `StatusLine` が持つ。

| 状態 | 条件 | 表示 |
| --- | --- | --- |
| 観測中 | 直近の送信に成功 | 「観測中 / 最終送信 14:32」 |
| 送信待ち | 送信に失敗しバッファに滞留 | 「オフライン / 12件待機中」 |
| 観測停止中 | サービスが動いていない | 「停止中」+ `[観測を再開]` |

「オフライン」を明示するのは、画面を見ただけでは通信の成否が分からないためである。
屋外設置後にオーナーが端末を見に行く動機のほとんどが「本当に送れているのか」の確認であり、
その答えを最初に出す。

`[観測を再開]` は**停止中のときだけ**出す。Android 14 以降は再起動後に自動復帰できず、
この操作がないとアプリから観測を再開する手段がなくなる。

端末名は表示しない（§4.3）。

### 9.3 再起動

| OS | 挙動 |
| --- | --- |
| Android 13 以下（SDK ≤ 33） | `BOOT_COMPLETED` を受けて FGS を自動的に開始する |
| Android 14 以上（SDK ≥ 34） | 自動復帰は**できない**。高優先度通知を出し、タップで前面に出してから開始する |

Android 14 以降、バックグラウンドから `camera` タイプの FGS を開始することは禁止されており、
`BOOT_COMPLETED` の受信はバックグラウンド起動に当たる。型を `dataSync` に変えても解決しない。
カメラは「使用中のみ」の権限であり、`camera` 型の FGS がその「使用中」を成立させているため。

通知文言:
> 「EdgeWatcher が停止しています。タップして観測を再開してください。」

**この分岐は SDK ≤33 側を検証できない**（§11.2）。したがって分岐は
`if (Build.VERSION.SDK_INT >= 34)` の1箇所に閉じ込め、両側の処理を最小に保つ。

---

## 10. テスト戦略 — 書くのは4本だけ

**方針: 実機を見れば分かる不具合にテストを書かない。**

書くのは、**壊れても実機では静かに間違うもの**、あるいは**実機で再現するのが非現実的なもの**
だけに限る。すべて JVM ユニットテストであり、エミュレータを必要としない（§2.1 の規約による）。

| # | 対象 | 書く理由 |
| --- | --- | --- |
| 1 | `IdGenerator.ulid(capturedAt)` の採番時刻が `capturedAt` と一致する | ずれるとサーバ側で観測が別の日に並ぶが、**端末画面には何も現れない** |
| 2 | HTTP status → `ApiFailure` の分類 | 1行の取り違えで「二度と繋がらない」か「無限再送」になる。§8 の 401/403 の扱いと §8.2 の「5xx で消さない」をここで固定する |
| 3 | バッファの eviction（288件 と 500MiB の先着） | 実機で 500MiB まで貯めて確かめるのは非現実的 |
| 4 | 送信順序（最新1枚 → 以後古い順） | 復帰時にしか現れず、実機での再現に長時間のオフラインを要する |

### 10.1 書かないもの

UI テスト、計装テスト、Robolectric、Room マイグレーションテスト、MockWebServer による
ネットワーク層のテスト。

Room のスキーマは **v1 のみ**とし、変更時は `fallbackToDestructiveMigration` を使う。
バッファを失っても観測が数枚欠けるだけであり、マイグレーションを書いて維持する費用に見合わない。

---

## 11. 受け入れ

個々の実装タスクの完了条件ではなく、統合後にまとめて確認する（§0）。
実機を常時給電で設置して行う。

### 11.1 手順

1. QR を読んでペアリングが成立する。404 のリトライが効いていることを含む
2. 権限を許可 / 拒否したときの表示が正しい
3. `[画角を合わせる]` でライブプレビューが出て、もう一度押すと戻る
4. **プレビューの ON / OFF を跨いでも送信が途切れない**（通知の「最終送信」が更新される）
5. **画面を消して3時間以上放置し、欠測がない**
6. 機内モード30分 → `オフライン / N件待機中` に変わる
7. 機内モード解除 → **最新の1枚が先に** Web のダッシュボードへ現れ、そのあと古い分が埋まる
8. 再起動 → 復帰要求の通知が出て、タップで観測が再開する（Android 14 以上の経路）
9. Web から端末を切断 → 次の送信で未ペアリング画面へ理由付きで戻る
10. ログアウトの確認ダイアログを経て未ペアリング画面へ戻り、Web 側が `DISCONNECTED` になる

手順 6・7・9・10 は稼働中の dev バックエンドを必要とする。

### 11.2 検証できない経路を明示する

**Android 13 以下の実機は手元に無いため、§9.3 の自動復帰経路は検証できない。**
Robolectric を書かない方針（§10.1）と合わせ、この経路は**未検証のまま出荷する**。

これを許容するのは、当該経路が `BOOT_COMPLETED` を受けて `startForegroundService` を
呼ぶだけであり、分岐が1箇所に閉じているためである。壊れていた場合の影響は
「古い端末で再起動後に自動復帰しない」に留まり、通知経由の手動再開は
どのバージョンでも動く。**この判断は本書に記録し、後から実機が手に入った時点で確認する。**

---

## 12. 既存ドキュメントの修正

実装と並行して直す。放置すると、次に読む人が誤った実装を書く。

| 対象 | 修正内容 |
| --- | --- |
| `04-native.md` §1.9 | 「401 を起点」→「401 と 403 を起点」。§8.1 の理由を添える |
| `04-native.md` §1.4 | 401 と書かれている箇所を揃える |
| `04-native.md` §1.5 | サムネイル「長辺160px」→「長辺480px」 |
| `04-native.md` §5 | 未確定事項の表から解決済みの項目を削り、決定値を本文へ反映 |
| `api.yml` `/device/uploads` | `thumbnail` の説明「長辺160px」→「長辺480px」 |
| `01-openquestion.md` APP-01 | `open` → `decided`（5分。backend の `defaultInterval` と同一） |
| `01-openquestion.md` APP-03 | `pending` → `decided`（本画像 1600px/q80、サムネイル 480px/q75） |
| `01-openquestion.md` APP-04 | `pending` → `decided`（CameraX / `lastLocation` / §10 のテスト方針） |
| `docs/mocks/native-mocks.html` | 端末名を出している3箇所（`paired` シナリオの見出し、常駐通知2種）から端末名を削る（§4.3） |

---

## 13. リポジトリ・CI・配布

- Gradle プロジェクトを **`android/`** に置く（`backend/` `web/` `infra/` と並べる）
- API のベース URL は `buildConfigField` で dev / prod を切り替える。
  API Gateway の URL は秘密ではないため `gradle.properties` に置いてよい
- GitHub Actions は **検証のみ**。PR と `develop` への push で
  `ktlint` + JVM ユニットテスト + `assembleDebug` を回す
- **配布は実機への直接インストール。** 署名鍵の管理、Play への公開、難読化は範囲外

---

## 14. やらないこと

Play 配布・リリース署名鍵・難読化 / `mock` フレーバーと偽バックエンド /
端末側の設定画面 / 証明書ピンニング / 画像のローカル暗号化 /
`ACCESS_BACKGROUND_LOCATION` / Room マイグレーション / 計装テスト・UI テスト・Robolectric /
複数端末プロファイル / 端末からの端末削除 / 直近に送った画像のサムネイル表示

これらを外す判断は `04-native.md` §3.6（端末側 UI を最小に保つ理由）と、
本書 §0.1（完成を最優先する）に基づく。
