# EdgeWatcher Android アプリ 設計

最終更新: 2026-09-10
ステータス: 設計確定（実装前）

関連: `docs/04-native.md`（機能要件）/ `docs/06-auth.md`（認証）/
`docs/engineering/api.yml`（API 契約）/ `docs/mocks/native-mocks.html`（画面モック）/
`docs/00-overview.md` §5・§6・§7

## 0. この文書の位置づけ

`04-native.md` が「何を作るか」を定めているのに対し、本書は「どう作るか」を定める。
`api.yml` は拘束力のある契約であり、本書がそれと食い違った場合は本書が誤りである。

本書で新たに決めたのは、`01-openquestion.md` に `open` / `pending` として残っていた
APP-01 / APP-03 / APP-04 の3件（§2）と、実装レベルの構造（§3 以降）である。

## 1. スコープ

`04-native.md` の機能要件をすべて満たす Android アプリを完成させる。
バックエンド・Web・インフラには手を入れない。API 契約も変更しない。

スコープ外:

- CI/CD ワークフローの追加（Android の配信は Play Console への手動アップロード。`00-overview.md` §8）
- 計装テスト（§14 に理由）
- リリース署名鍵の管理と Play Console への提出

## 2. 確定した未決事項

### APP-01: 送信間隔の初期値 = 5分

サーバから `nextConfig` が届くまでの既定値。5分を選ぶのは、バッファ上限の288件が
「5分間隔で24時間相当」として決まっており（APP-02）、初期値を5分に置くと
最も負荷の高い条件が既定になるため。設置直後の動作確認でも5分待てば送信の成否が分かる。

### APP-03: 画像仕様

| 項目 | 値 |
| --- | --- |
| フォーマット | JPEG（`api.yml` の `encoding` が `image/jpeg` を指定） |
| 本画像 | 長辺 1600px / 品質 80 |
| サムネイル | 長辺 160px / 品質 70（長辺160px は `04-native.md` §1.5 で確定済み） |
| 合計サイズ上限 | 4,500,000 バイト（`api.yml`。超過は 400 でリトライ不能） |

1枚あたり本画像 250〜450KB を見込む。288件で1端末1日あたり 100MB 前後、
prod の保持7日で 700MB 程度。上限 4.5MB に対して10倍以上の余裕があり、
端末差で膨らんでも 400 を踏まない。

**サイズガード**: 符号化後の合計バイト数を実測し、4,500,000 を超えていたら
品質 80 → 65 → 50 の順に再符号化する。それでも超える場合は長辺を 1280px に落として
品質 50 で符号化する。送っても必ず 400 になるフレームをバッファに積まないため。

### APP-04: 実装レベルの選定

| 領域 | 選定 | 理由 |
| --- | --- | --- |
| カメラ | CameraX | 対応端末の幅（API 26 〜 Android 16）に対し、端末差を自前で踏み抜くコストが Camera2 の制御自由度を上回る。要件に低レベル制御を必要とする箇所が無い |
| QR 読み取り | ML Kit `barcode-scanning`（bundled）を CameraX `ImageAnalysis` 経由 | bundled 版は Play 開発者サービスに依存せず動く。格安端末を前提にする以上ここは外せない |
| 位置情報 | `FusedLocationProviderClient.lastLocation` | `getCurrentLocation` は測位を待つため `04-native.md` §1.5 の「測位を待たない」に反する |
| HTTP | OkHttp + kotlinx.serialization（Retrofit は使わない） | 端末が叩くのは4本のみ。multipart のパート名と Content-Type は契約が縛る箇所であり、直接書く方が `api.yml` と1対1に対応する。テストは MockWebServer |
| 永続化 | Room（メタデータ）+ アプリ専用ストレージのファイル（画像） | `04-native.md` §3.3 の決定 |
| 資格情報 | `EncryptedSharedPreferences` | `04-native.md` §2.5 の決定 |
| UI | Jetpack Compose | 画面が2つ。状態から表示への写像をテストしやすい |
| DI | 手書きの `AppContainer`（Hilt は使わない） | 全てコンストラクタ注入にすればテストに必要なものは揃う。Service への注入のためだけに DI フレームワークと KSP のビルド時間を足す理由がない |
| 非同期 | Coroutines + Flow | |
| テスト | JVM 単体テスト + Robolectric。計装テストは書かない | §14 |

## 3. プロジェクト構成

リポジトリ直下に `android/` を新設する（`backend/` `web/` `infra/` と同じ並び）。

```
android/
  settings.gradle.kts
  build.gradle.kts
  gradle/libs.versions.toml     バージョンカタログ
  domain/                       kotlin("jvm")。Android SDK はクラスパスに無い
  app/                          com.android.application
```

`:domain` を Android ライブラリではなく純粋な JVM モジュールにするのは、
Android SDK がクラスパスに存在しないことで、「うっかり `Context` を握って
テストが重くなる」という劣化がコンパイルエラーとして即座に止まるため。
構造で守れるものを規律で守らない。

### ビルド設定

| 項目 | 値 |
| --- | --- |
| `applicationId` | `com.edgewatcher` |
| `minSdk` | 26 |
| `targetSdk` / `compileSdk` | 36（Android 16）。安定版の最新であり、動作確認に使う実機と一致する。ローカル SDK に `android-36` が無ければ `sdkmanager` で追加する。プレビュー SDK は使わない |
| 画面の向き | 縦固定 |
| `versionName` / `versionCode` | `1.0.0` / `1` |

### フレーバー

次元 `backend` に `mock` と `live` の2フレーバーを置く。

| フレーバー | `EdgeWatcherApi` の実装 | 備考 |
| --- | --- | --- |
| `mock` | アプリ内の偽バックエンド。ネットワークに一切出ない | `applicationIdSuffix = ".mock"` を付け、live 版と同じ端末に共存できるようにする |
| `live` | OkHttp 実装 | `EW_API_BASE_URL` をビルド時に受け取る |

`EW_API_BASE_URL` は環境変数、または Gradle プロパティ `ewApiBaseUrl` から読む。
**`live` フレーバーでこれが未設定ならビルドを失敗させる。** 空の URL を焼いた APK が
作れてしまうと、何も送っていないアプリを実物と誤認する事故が起きるため。

切り替えの軸を「モックか実物か = フレーバー（コンパイル時に固定）」と
「どの AWS か = 環境変数（ビルド時に注入）」に分けている。AWS 環境は変わりうるが
API 契約は変わらない、という前提に対応する。モックの実装が live ビルドに
コンパイルされないことが、ソースセットの分離によって構造的に保証される。

`release` ビルドでは平文通信を一切許可しない（`networkSecurityConfig`）。
`mock` はネットワークに出ないため平文の設定自体が不要である。

### mock フレーバーの偽バックエンド

`api.yml` の端末群4本を模す。ペアリングは Web アプリをモックモードで動かして
QR を出し、それを実機で読む（`web/src/services/mock-server.ts` が `ew1:PAIR_XXXXXXXXXX`
形式のコードを発行する）。

偽バックエンドは**常に成功を返す**。確率的な失敗は入れない。再現しない失敗は
デバッグの役に立たないためである。オフライン時の挙動（バッファ滞留、指数バックオフ、
上限退避）は `live` フレーバーで実機を機内モードにして確認する。

## 4. 境界（ポート一覧）

`:domain` はインターフェースを定義し、`:app` が実物を実装する。
この一覧がそのまま、テストで差し替える対象の一覧でもある。

| ポート | 責務 | `:app` 側の実装 |
| --- | --- | --- |
| `EdgeWatcherApi` | 端末群4本の呼び出し。結果は封じた型で返し、HTTP ステータスを呼び出し側に漏らさない | OkHttp（live）/ 偽バックエンド（mock） |
| `CredentialStore` | `deviceId` / `deviceSecret` / `sessionToken` / `sessionExpiresAt` の読み書きと全消去 | `EncryptedSharedPreferences` |
| `ObservationStore` | 観測メタデータと画像ファイルの登録・列挙・削除・総バイト数 | Room + アプリ専用ストレージ |
| `CaptureSource` | 1枚撮る。プレビュー用 Surface の付け外し | CameraX |
| `JpegEncoder` | 指定された長辺と品質で JPEG に符号化する | `Bitmap` / `BitmapFactory` |
| `LocationSource` | 最後の既知位置（測位は待たない。取得できなければ null） | `FusedLocationProviderClient` |
| `AlarmScheduler` | 次回撮影時刻に起こす / 取り消す | `AlarmManager` |
| `Clock` | 現在時刻 | システム時刻 |
| `IdGenerator` | 指定時刻をタイムスタンプ部に持つ ULID の採番 | 実装は `:domain` 内で完結してよい |

`JpegEncoder` をポートとして切ったのは、画素の操作と「どう符号化するか」の判断を
分けるため。長辺・品質・サイズガードの段階は方針であって画像処理ではないので
`:domain` の `ImagePolicy` が持ち、`:app` は言われた寸法で符号化するだけにする。

`:domain` が持つロジック: `PairingService` / `SessionManager` / `CaptureCoordinator` /
`UploadQueue` / `BufferPolicy` / `ObservationEngine` / `ImagePolicy` / `StatusLine`。

## 5. 状態モデル

画面・常駐通知・サービスが同じ1つの型を見る。`04-native.md` §1.7 が
「画面と通知の両方に同じ情報を1行で出す」と要求している以上、表示の一致を
実装の規律ではなく型で担保する。以下、本書における `§n` は本書の節を指し、
要件側を参照する場合は必ず `04-native.md §n` と書く。

```kotlin
sealed interface AppState {
    data class Unpaired(val reason: UnpairReason?) : AppState
    data class Paired(val run: RunState) : AppState
}

sealed interface RunState {
    data class Observing(val lastUploadAt: Instant?) : RunState
    data class Offline(val pending: Int, val lastUploadAt: Instant?) : RunState
    data object Stopped : RunState
}

enum class UnpairReason { DISCONNECTED_BY_SERVER, LOGGED_OUT }
```

モックの3状態（`ok` / `warn` / `off`）と1対1に対応する。
`Stopped` のときだけ `[観測を再開]` を出すという `04-native.md` §1.7 の分岐は、
この型の分岐そのものになる。

`Offline` の定義: **未送信が1件以上あり、かつ直近の送信試行が失敗している**。

## 6. ペアリング

QR ペイロード `ew1:<pairingCode>` から `ew1:` を剥がした残りを、そのまま
`pairingCode` として送る。**端末はコードの形式を検証しない。** 形式の妥当性は
サーバが決めることであり、端末が「52文字 base32」を強制すると
モックモードの Web が発行するコードと噛み合わなくなる。

`deviceInfo` には `Build.MODEL` / `Build.VERSION.RELEASE` / `BuildConfig.VERSION_NAME` を入れる。

### 失敗の読み分け

| 結果 | 挙動 |
| --- | --- |
| 404 `PAIRING_NOT_FOUND` | 1秒空けて再送。**初回1回 + リトライ3回 = 最大4リクエスト** |
| 409 `PAIRING_CODE_CONSUMED` / `PAIRING_CODE_EXPIRED` | 即座に「この QR コードは使えません」 |
| 400 `VALIDATION_ERROR` | リトライしない |
| ネットワーク到達不能 | **404 と混ぜない。**「サーバーに接続できません」として再スキャンを促す |

「最大3回リトライ」（`04-native.md` §1.4 / `api.yml`）を、初回を含めた最大4リクエストと
解釈する。曖昧なまま実装すると挙動が2通りに割れるため、ここで数字として固定する。

到達不能を 404 と同じ経路に入れないのは、圏外の端末が「この QR は使えません」と
表示してしまい、オーナーが QR を再発行しても直らないという誤誘導になるため。

### 資格情報の保存

`deviceId` と `deviceSecret` を `EncryptedSharedPreferences` に書き込み、
**書けたことを確認してから** `POST /device/token` に進む。`deviceSecret` が平文で
流れるのはこの1度だけであり、保存に失敗したまま先へ進むと、その端末は
Web から QR を再発行するまで永久に復帰できない。

### カメラの持ち主の切り替え

未ペアリング時はサービスが動いていないため、スキャナ画面がカメラを持つ。

- ペアリング成立 → **スキャナを閉じてから** Foreground Service を起動する
- 切断・ログアウト → **サービスを止めてから** スキャナを開く

Android のカメラは同時に2つのクライアントが開けないため、この順序は守る必要がある。

## 7. セッションと自己修復

送信前に `sessionExpiresAt` を確認し、過ぎていれば先に `POST /device/token` を呼ぶ。
ただしこれは往復を減らす最適化にすぎず、正しさの根拠にはしない。端末の時計はずれるため、
期限判定が外れた場合は下の 401/403 経路が必ず拾う。

`api.yml` 冒頭が明記するとおり、**端末が実際に受け取るのは 403 である。**
オーソライザが `isAuthorized:false` を返すと API Gateway が 403 に変換する。
`04-native.md` §1.9 が「401」と書いているのはネットワーク上の事実と食い違う。
401（ヘッダ欠落）と 403（拒否）の両方を同じ入口にする。

```
/device/uploads または /device/logout が 401 または 403
  └→ POST /device/token
       200                  → 新しい sessionToken で再送。以後継続
       401                  → 資格情報を全消去 → サービス停止
                              → Unpaired(DISCONNECTED_BY_SERVER) へ
       5xx / ネットワーク断 → 修復ではない。バックオフして待つ。フレームは捨てない
```

最後の行が本節でもっとも重要である。`POST /device/token` の 5xx や圏外を
「切断された」と解釈すると、電波が悪いだけの端末が `deviceSecret` を捨ててしまい、
屋外の設置場所まで行って QR を読み直す以外に復旧手段がなくなる。
**資格情報を消すのは 401 が返ってきたときだけとする。**

未ペアリング画面に戻る際は理由を残す（`AppState.Unpaired(reason)`）。
「この端末は接続を解除されました。再度ペアリングしてください。」
黙って QR スキャナに戻すと、オーナーには端末の故障と区別がつかない。

## 8. 観測パイプライン

### 8.1 タイマー

サービス稼働中は `PARTIAL_WAKE_LOCK` を保持し続け、それとは別に
`AlarmManager.setExactAndAllowWhileIdle` で**次の1回だけ**を予約する。

周期アラームにしないのは、`nextConfig` で間隔が変わりうるため。撮影のたびに
次回を決め直す形にすると、「設定変更は最大1送信サイクル遅れて反映される」という
`04-native.md` §1.5 の定義が実装上そのまま自明になる。

次回時刻は**撮影周期の完了時点から interval 後**に置く。定点観測に厳密な等間隔の
意味はなく、ドリフトを補正しようとすると撮影が詰まったときに連射になる。

`canScheduleExactAlarms()` が false の場合は設定画面へ誘導する（§11）。
**未許可のままでも `setAndAllowWhileIdle` にフォールバックして動き続ける。**
不正確になるが、止まるよりはるかにましである。

### 8.2 カメラの所有権

サービスが `LifecycleRegistry` を持ち、状態によってバインドを切り替える。

| 状況 | バインド | 撮影 |
| --- | --- | --- |
| 通常（プレビュー OFF） | 何もバインドしない = カメラは閉じている | 発火のたびに `ImageCapture` をバインド → 撮影 → `unbindAll()` |
| プレビュー ON | `Preview` + `ImageCapture` を維持 = カメラは開いたまま | バインドし直さず、**同じセッションから撮る** |

これが `04-native.md` §2.2（発熱を避けるため常時は開かない）と §3.2（プレビュー中も
同じセッションから撮り、送信に穴を開けない）を同時に満たす形である。
CameraX は `bindToLifecycle` した時点でカメラを開くため、`ImageCapture` を
常時バインドしたままにすると §2.2 に反する。

画面はサービスにバインドして `PreviewView` の `SurfaceProvider` を渡すだけで、
カメラ本体には触らない。

### 8.3 撮影1周期

```
アラーム発火（wakelock は保持済み）
  → 撮影（§8.2 の規則）
  → 本画像 長辺1600px/品質80、サムネイル 長辺160px/品質70 を生成
  → 合計バイト数を検査し、4,500,000 超なら本書 §2 のサイズガードを適用
  → observationId = ULID（タイムスタンプ部は capturedAt に一致させる）
  → 最後の既知位置を付与（測位は待たない。取れなければ lat/lng を省く）
  → 画像をアプリ専用ストレージへ、メタデータを Room へ（未送信として）
  → バッファ上限を判定し、超過分を古い順に削除（§9）
  → 送信キューを起こす（§10）
```

ULID のタイムスタンプを `capturedAt` に合わせるのは装飾ではない。`api.yml` が
「サーバは日別クエリの範囲を ULID のタイムスタンプ部から導く」と書いており、
ずれると観測が Web 上で別の日に並ぶ。

`capturedAt` は ISO 8601（RFC 3339）で送る。パースできなければ 400 になる。

## 9. バッファ

Room の1行が持つのは `observationId`（主キー）/ `capturedAt` / `imagePath` /
`thumbPath` / `lat` / `lng` / `sizeBytes` / 送信状態 / 試行回数 / 次回試行時刻 のみ。
画像本体はアプリ専用ストレージのファイルに置く（`04-native.md` §3.3）。

退避は**新しい1件を登録した後**に判定し、次のいずれかが成立する間、
最古から削除する（Room の行とファイルを同時に消す）。

- 件数が 288 を超える
- 合計バイト数が 524,288,000 バイト（500MiB）を超える

削除が発生した回は、モックの `buffer-full` に対応するバナーを画面に出す。

## 10. 送信キュー

送信対象は毎回この規則で選ぶ。

1. 未送信の中で最も新しい行が**まだ一度も送信を試みていない**なら、それを選ぶ
2. そうでなければ、未送信の中で最も古い行を選ぶ

結果として「最新の1枚を先に送り、そのあと残りを古い順に送る」になる。
新しい撮影が入るたびに規則1が再び成立するため、長時間オフラインから復帰した
ときだけを特別扱いする分岐は要らない。通常時は未送信が1件しかないので同じ動きになる。

これは `04-native.md` §3.5 の要求（復旧時に最新の1枚を先に送る）を満たす。
遅れて届いた古い画像が `Device.latestThumbnailKey` を巻き戻さないための条件付き更新は
サーバ側が行う（`engineering/dynamodb.md` §5.1）。

| 応答 | 扱い |
| --- | --- |
| 200 | ファイルと行を削除。`nextConfig.intervalMinutes` を保存し、次回アラームから適用 |
| 400 | **フレームを破棄。**リトライしても永久に通らない |
| 413 | **フレームを破棄。**同上 |
| 401 / 403 | §7 のセッション修復へ。**フレームは残す** |
| 5xx / ネットワーク断 | 指数バックオフ。**フレームは残す** |

バックオフは、その行の**連続失敗回数を n** として、次回試行までの待ち時間を
`min(5 * 2^(n-1), 300)` 秒とする（1回目の失敗後は5秒、以降 10, 20, 40, 80, 160, 300…）。
送信に成功した行は削除されるため、n が持ち越されることはない。
上限を300秒に置くのは、それ以上待っても次の撮影が来るだけだから。
**リトライ中も定期撮影は止めない**（`04-native.md` §2.4）。

同じ `observationId` で再送しても記録は1件に保たれるため、200 を受け取れなかった
場合の再送は安全である（`api.yml` の冪等性の節）。

リクエストは multipart/form-data で、パート名は `image` / `thumbnail` / `metadata`、
Content-Type はそれぞれ `image/jpeg` / `image/jpeg` / `application/json`。
`Authorization` ヘッダは **`Bearer` を付けない素のトークン**。
`metadata` に `deviceId` は含めない（サーバは読まない）。

## 11. UI・権限・通知

### 画面

初回起動時、未ペアリング画面に入る前に権限ゲートを通す。

| 権限 | 備考 |
| --- | --- |
| `CAMERA` | 必須 |
| `ACCESS_FINE_LOCATION` | 必須 |
| `POST_NOTIFICATIONS` | Android 13+ |
| `FOREGROUND_SERVICE_CAMERA` / `_LOCATION` | マニフェスト宣言のみ。実行時要求は不要 |

`ACCESS_BACKGROUND_LOCATION` は要求しない。恒久的に拒否された場合は
「設定を開く」導線だけを出し、**権限なしで動作する縮退モードは持たない**。

以降の画面は `AppState` の分岐そのままである。

- `Unpaired` → 全画面 QR スキャナ。`reason` があれば理由バナーを重ねる
- `Paired` → 状態1行 + `[画角を合わせる]`（`Stopped` のときだけ `[観測を再開]`）+ `[ログアウト]`

ボトムシートは5種（ペアリング中 / 無効な QR / 接続しました / バッテリー最適化の案内 /
ログアウト確認）で、`native-mocks.html` の14シナリオを覆う。

**端末名は表示しない。** `POST /device/pair` の応答は `{deviceId, deviceSecret}` だけで、
端末名は端末に届かないため。完了画面は「接続しました」、通知は
「観測中 ・ 最終送信 14:32 ・ 5分間隔」とする。`api.yml` を変更してまで表示する
価値は無い。オーナーは目の前の端末をセットアップしている最中であり、
どの端末かは分かっている。

### ペアリング直後の案内

ペアリング完了直後に一度だけ、**バッテリー最適化の除外**と、
必要なら**正確なアラームの許可**（`canScheduleExactAlarms()` が false のとき）を案内する。
強制はせず、稼働画面から再度開けるようにする。メーカー独自の省電力機構が
Foreground Service を停止させることが実際に多く、案内しないと
「なぜか止まる」の主要因になる（`04-native.md` §1.3）。

### 通知

常駐通知の本文は、画面の状態行と**同じ `:domain` の関数（`StatusLine`）から生成する**。
`04-native.md` §1.7 が「画面と通知に同じ情報を出す」と要求している以上、一致を
実装の注意深さに頼らず、1か所からの派生にする。

チャンネルは2つ。

| チャンネル | 重要度 | 用途 |
| --- | --- | --- |
| 常駐 | `LOW`（無音） | Foreground Service の常駐通知 |
| 復帰要求 | `HIGH` | 再起動後に観測が止まっていることの通知（§12） |

## 12. 再起動と復帰

`BOOT_COMPLETED` を受けて `Build.VERSION.SDK_INT` で分岐する。

| OS | 挙動 |
| --- | --- |
| Android 13 以下 | Foreground Service を直接開始する。オーナーの操作は不要 |
| Android 14 以上 | **高優先度通知のみを出す。** タップで Activity を前面に出し、そこから Foreground Service を開始する |

Android 14 以降はバックグラウンドから `camera` タイプの Foreground Service を
開始できず、`BOOT_COMPLETED` の受信はバックグラウンド起動に当たるため、
自動復帰は原理的に不可能である（`04-native.md` §3.4）。

サービスは `START_STICKY` で再生成させる。ただし**再生成もバックグラウンド起動に
当たるため、Android 14 以上では失敗しうる。** 失敗した場合は `04-native.md` §1.10 と
同じ「`Stopped` + 復帰要求の通知」に落とす。`START_STICKY` が当てになるのは 13 以下である。

一方、**アラーム発火による撮影はこの制限に掛からない。** サービスは既に前面で
動いており、新たに Foreground Service を開始しているわけではないため。
制限が効くのは起動の瞬間（boot と再生成）だけである。

## 13. エラー処理の総括

端末が取りうる行動は4つしかない。すべてのエラーはこのいずれかに写像される。

| 行動 | 対象 |
| --- | --- |
| リトライする | `/device/pair` の 404（最大4リクエスト）、アップロードの 5xx・ネットワーク断（指数バックオフ） |
| フレームを捨てる | アップロードの 400 と 413 |
| セッションを取り直す | アップロード・ログアウトの 401 と 403 |
| 資格情報を捨てて未ペアリングへ戻る | `/device/token` の 401 のみ |

最後の行を狭く保つことが、この設計でもっとも重要な制約である。
ここが広がると、通信状態が悪いだけの端末が現地に行かないと復旧できなくなる。

## 14. テスト戦略

グランドルール（TDD）に従い、すべての機能は失敗するテストから書く。
配分は「実際に Red を観測できる場所」を基準に決める。

| 層 | 実行環境 | 対象 |
| --- | --- | --- |
| `:domain` 単体テスト | JVM | ペアリングのリトライ判定、セッション自己修復、バッファ退避、送信順序、指数バックオフ、エラー分類、画像方針とサイズガード、ULID 採番、`nextConfig` 適用、状態行の文言 |
| `:app` JVM テスト | JVM + MockWebServer | `api.yml` の応答を再現し、multipart のパート名・Content-Type・`Authorization` が素のトークンであることを検証 |
| Robolectric | JVM | Room の DAO、通知の文言、権限分岐、Compose 画面の状態別表示、`BOOT_COMPLETED` の SDK 分岐 |
| 計装テスト | — | 書かない |

Robolectric の `@Config(sdk = ...)` で API レベルを切り替えられるため、
**実機（Android 16）では確かめようのない「Android 13 以下は自動復帰する」経路を
ここで検証する。** 実機で踏めない分岐をテストで押さえる、という役割分担である。

計装テストを書かないのは、カメラの実撮影・Foreground Service の実起動・実アラームは
自動化しても不安定で、緑になっても得られる確信が低いため。代わりに実機での
手動スモークを行う。

### 実機スモーク手順（Android 16 実機）

1. `mock` フレーバーを入れ、Web をモックモードで動かして QR を読み、ペアリングが成立する
2. 権限ゲートの3権限を許可 / 拒否したときの表示を確認する
3. `[画角を合わせる]` でライブプレビューが出て、もう一度押すと戻る
4. プレビュー中も5分間隔の撮影が止まらない（通知の「最終送信」が更新される）
5. 画面を消して15分放置し、撮影と送信が続いていることを確認する
6. `live` フレーバーで機内モードにし、`Offline / N件待機中` に変わることを確認する
7. 機内モードを解除し、最新の1枚が先に送られることを Web 側で確認する
8. 端末を再起動し、復帰要求の通知が出てタップで観測が再開することを確認する
9. Web から端末を削除し、次の送信で未ペアリング画面へ理由付きで戻ることを確認する
10. ログアウトの確認ダイアログを経て未ペアリング画面へ戻ることを確認する

手順 6・7・9 は稼働中の `live` バックエンド（AWS の dev 環境）を必要とする。
接続先が用意できていない段階では、この3手順を保留として明示的に報告する。

Android 13 以下の実機は手元に無いため、手順 8 の自動復帰経路は
Robolectric のテストで担保し、実機確認の対象外とする。

## 15. docs の更新

本設計の確定に伴い、次を更新する。

| 対象 | 内容 |
| --- | --- |
| `01-openquestion.md` APP-01 | `open` → `decided`。初期値5分 |
| `01-openquestion.md` APP-03 | `pending` → `decided`。長辺1600px / 品質80 / サムネイル長辺160px・品質70 |
| `01-openquestion.md` APP-04 | `pending` → `decided`。CameraX / `lastLocation` / 計装テストは書かない |
| `04-native.md` §5 | 未確定事項の表から解決済みの項目を削り、決定値を本文へ反映 |
| `04-native.md` §1.9 | 実際に届くのは 403 である旨を追記（`api.yml` との食い違いの解消） |
| `docs/mocks/native-mocks.html` | 端末名を出している2箇所（`paired` シナリオの見出し、常駐通知2種）から端末名を削る |

## 16. 実装フェーズ

| # | 内容 | 完了条件 |
| --- | --- | --- |
| 0 | Gradle の骨組み、2モジュール、フレーバー、バージョンカタログ | `./gradlew test` が緑 |
| 1 | `:domain` の純ロジックを TDD で実装 | 要件の判断がすべて JVM テストで覆われている |
| 2 | `EdgeWatcherApi` の OkHttp 実装、`mock` フレーバーの偽バックエンド | MockWebServer のテストが緑 |
| 3 | 永続化（Room + `EncryptedSharedPreferences`） | Robolectric の DAO テストが緑 |
| 4 | カメラ・位置・アラーム・Foreground Service | 実機で撮影と送信が回る |
| 5 | UI と通知 | モックの14シナリオが再現できる |
| 6 | 再起動復帰、バッテリー最適化・正確なアラームの誘導 | 実機スモーク 8 が通る |
| 7 | 実機スモーク一式と docs 更新（§15） | 手順1〜10 の結果を報告 |

フェーズ1が全体の中心である。ここが終わった時点で、要件に書かれた判断の大半は
Android を一切起動せずに検証済みになる。
