# EdgeWatcher バックエンド設計

最終更新: 2026-08-24
ステータス: 方針確定(具体値の一部が未確定 → §5)

関連: `02-infra.md`(AWS リソース)、`06-auth.md`(認証)、`engineering/dynamodb.md`(キー設計)、
`03-web.md` / `04-native.md`(API の利用側)

本書はバックエンドを「Lambda によるアプリケーション実装」と「それを取り巻く AWS の設定値」の
両面から定義する。リソースの作り方(Terraform のスタック構成・モジュール粒度)は `02-infra.md`、
テーブルのキー設計は `engineering/dynamodb.md` にあり、本書では重複させない。

---

## 1. 機能要件

### 1.1 Lambda 関数の構成

**認可の境界で4つに分ける。** 分割線を認可の境界と一致させることで、IAM ポリシーが
関数ごとに自然に最小化される。

| 関数 | 経路 | オーソライザ | 主な責務 |
| --- | --- | --- | --- |
| `web-api` | `/app-config`, `/devices/*`, `/observations/*` | JWT(Cognito) | 端末の管理、観測の閲覧、署名付き URL の発行 |
| `device-auth` | `/device/pair`, `/device/token` | なし(認証前) | ペアリングの成立、セッショントークンの発行 |
| `device-api` | `/device/uploads`, `/device/logout` | Lambda オーソライザ | 画像の受信、ログアウト |
| `authorizer` | — | — | 端末セッションの検証 |

同じ関数の中では、HTTP メソッドとパスによるルーティングを Go 側で行う。
API Gateway のルートは経路ごとに定義し、統合先だけが同じ関数を指す。

#### この分け方を選んだ理由

**IAM が自然に最小化される。** 画像バケットへの `PutObject` を持つのは `device-api` だけでよく、
`web-api` は `GetObject` の署名生成しか要らない。1関数にまとめると、Web からの読み取り経路にも
書き込み権限が付いて回る。

**リソース設定を用途別に振れる。** `device-api` は最大 4.5MB のペイロードを扱うためメモリを厚く取り、
`web-api` は軽いので小さく保てる。`authorizer` は毎リクエスト走るのでコールドスタートを最小化する
設定にする(§2.2)。

**認証前の経路が独立する。** `device-auth` はオーソライザを通らない唯一の関数であり、
入力を一切信用できない。これを他の経路と同じバイナリに同居させないことで、
「ここだけは自前で検証する」という性質がコードの配置に現れる。

### 1.2 API 一覧

```text
--- web-api(JWT オーソライザ) ---
GET    /app-config                            環境定数(retentionDays / deviceLimit / intervalOptions)
GET    /devices                               端末一覧(+ ペアリング状態)
POST   /devices                               端末を追加 → Device(PENDING) + PairingSession
POST   /devices/{id}/pairing-sessions         再ペアリング / QR 再発行
GET    /devices/{id}/pairing-sessions/latest  ポーリング用
POST   /devices/{id}/disconnect               セッション切断
PATCH  /devices/{id}                          端末名 / 送信間隔の変更
DELETE /devices/{id}                          削除(status = ARCHIVED)
GET    /devices/{id}/observations?date=       指定日の観測(サムネイルの署名付き URL 付き)
GET    /observations/{id}/image               本画像の署名付き URL

--- device-auth(認証前) ---
POST   /device/pair                           { pairingCode, deviceInfo } → { deviceId, deviceSecret }
POST   /device/token                          { deviceId, deviceSecret } → { sessionToken, expiresAt }

--- device-api(Lambda オーソライザ) ---
POST   /device/uploads                        本画像 + サムネイル + メタデータ → { nextConfig }
POST   /device/logout                         204
```

### 1.3 アップロードの受け口

**端末は本画像・サムネイル・メタデータを1リクエストで送る**(`03-web.md` §3.7)。
`multipart/form-data` で受け、Lambda が両方を S3 に置く。

```text
POST /device/uploads
  Authorization: <sessionToken>
  Content-Type: multipart/form-data
    image      : JPEG(本画像)
    thumbnail  : JPEG(サムネイル)
    metadata   : JSON { observationId, capturedAt, lat, lng }
  ↓
  1. Authorizer が付けた deviceId / ownerId を認可コンテキストから取る
  2. S3 に本画像とサムネイルを Put
  3. DynamoDB に Observation を Put(attribute_not_exists 条件。§2.6)
  4. Device の latest* / lastReceivedAt を更新(条件付き。§3.3)
  5. { nextConfig: { intervalMinutes } } を返す
```

#### ペイロード上限

| 制約 | 値 |
| --- | --- |
| API Gateway (HTTP API) のペイロード | 10MB |
| **Lambda の同期呼び出しペイロード** | **6MB** ← これが効く |
| API Gateway → Lambda のエンコード | base64(約1.33倍に膨らむ) |

実質、**本画像 + サムネイルの合計で 4.5MB 程度**が上限になる。
これは APP-03(画像の解像度・圧縮率)の天井を決める制約であり、そちらで値を決める際の入力になる。

上限を超えたリクエストは API Gateway が `413` を返す。端末側はこれをリトライ不能な失敗として扱い、
バッファから捨てる。リトライしても永久に通らないため。

#### 書き込みの順序

**S3 を先、DynamoDB を後にする。** 逆にすると「レコードはあるが画像がない」状態が生まれ、
Web が署名付き URL を発行しても 404 になる。

S3 を先にすると「画像はあるがレコードがない」孤児オブジェクトが生まれうるが、
こちらは**ライフサイクルルールが保持期間後に回収する**ため放置してよい。
どちらかが必ず起きるなら、自動的に消える側に倒す。

### 1.4 S3 のキー設計

画像バケットのキーは、**端末ごと・日付ごとに階層化する。**

```text
observations/<deviceId>/<YYYY-MM-DD>/<observationId>.jpg
observations/<deviceId>/<YYYY-MM-DD>/<observationId>_thumb.jpg
```

- `deviceId` を先頭に置くのは、端末削除時に**プレフィックス指定でまとめて列挙・削除できる**ようにするため
  (DATA-02 の掃除処理。MVP スコープ外だが、キー設計だけは先に効かせておく)
- 日付を挟むのは、コンソールから覗くときに人間が辿れるようにするため
- 本画像とサムネイルを**同じプレフィックスに置き、接尾辞だけで区別する**。別ツリーに分けると、
  削除時に2箇所を消す必要が生まれる

`observationId` は DynamoDB の SK と同じ ULID を使う。**S3 のキーから DynamoDB のアイテムを
一意に引ける**ため、孤児オブジェクトの照合ができる。

#### ライフサイクル

`retention_days`(dev 1 / prod 7)後に自動削除する(`02-infra.md` §7)。
DynamoDB の TTL と同じ値を使い、「画像はないのにレコードだけ残る」状態を避ける。

### 1.5 署名付き URL

Web に画像を渡す経路は署名付き URL のみ。バケットは非公開のままにする。

| 用途 | 発行元 | 有効期限 |
| --- | --- | --- |
| サムネイル(一覧・履歴のコマ列) | `GET /devices/{id}/observations` のレスポンスに同梱 | 15分 |
| 本画像(履歴の大表示) | `GET /observations/{id}/image` で都度発行 | 15分 |

署名の生成は S3 への通信を伴わない純粋な計算であり、件数が増えてもレイテンシに影響しない。
そのためサムネイルはまとめて返してよい(`03-web.md` §3.6)。

### 1.6 エラーレスポンス

`03-web.md` §1.10 のエラー階層が機能するよう、**ステータスコードの意味を固定する。**

| ステータス | 意味 | クライアントの扱い |
| --- | --- | --- |
| `400` | リクエストの形式が不正 | リトライ不能 |
| `401` | 認証されていない / セッション失効 | Web は `/login` へ、端末はトークン再取得 |
| `403` | 認証済みだが対象の所有者でない | 404 と同じ表示にする |
| `404` | 対象が存在しない | 一覧へ戻る |
| `409` | 状態の競合(QR が消費済み・期限切れ) | リトライ不能。再発行を促す |
| `413` | ペイロード超過 | リトライ不能。端末はバッファから捨てる |
| `429` | 上限超過(端末数・レート制限) | リトライ不能。理由を表示 |
| `5xx` | サーバ側の障害 | 指数バックオフでリトライ |

本文の形式は全経路で統一する。

```json
{ "error": { "code": "DEVICE_LIMIT_EXCEEDED", "message": "端末の上限に達しています" } }
```

`code` は機械可読な識別子で、クライアントの分岐はこちらを見る。`message` は表示用。
**HTTP ステータスだけでは足りない**場面があるため両方持つ。例えば `409` は
「QR が期限切れ」と「QR が消費済み」の両方で返るが、`04-native.md` §1.4 はこれらを
区別して扱う必要がある。

#### 403 と 404 を同じ表示にする理由

`06-auth.md` §7 のとおり、Lambda は JWT の `sub` と対象リソースの `ownerId` の一致を毎回確認する。
不一致なら `403` を返すが、**クライアントは 404 と同じ画面を出す**
(`03-web.md` §1.10.2)。他人の `deviceId` の存在有無を、エラーの出し分けから推測されないようにするため。

サーバ側で `404` に寄せて返す案もあるが、**ログには区別を残したい**(不正アクセスの兆候として)。
そのため API は正しく `403` を返し、表示側で吸収する。

---

## 2. 非機能要件

### 2.1 ランタイム

| 項目 | 値 | 根拠 |
| --- | --- | --- |
| 言語 | Go | `00-overview.md` §7 |
| ランタイム | `provided.al2023` | Go は独自ランタイムで動かす。起動が速い |
| アーキテクチャ | `arm64` | x86 比で約2割安く、Go はクロスコンパイルが容易 |

### 2.2 関数ごとのリソース設定

| 関数 | メモリ | タイムアウト | 備考 |
| --- | --- | --- | --- |
| `authorizer` | 128MB | 3秒 | 毎リクエスト走る。DynamoDB `GetItem` 1回のみ |
| `web-api` | 256MB | 10秒 | 署名生成と DynamoDB の読み書きのみ |
| `device-auth` | 256MB | 10秒 | `TransactWriteItems` とハッシュ計算 |
| `device-api` | 1024MB | 30秒 | 最大 4.5MB のペイロードを保持し S3 へ2回 Put する |

**メモリは CPU 割り当ての代理変数である。** Lambda は割り当てメモリに比例して CPU が増えるため、
`device-api` にメモリを厚く取るのは、容量よりも**S3 への転送を速く終わらせる**ためである。
実行時間が短くなり、結果として課金が下がることも多い。値は OPS-05 の試算後に見直す。

### 2.3 同時実行

**予約同時実行数は設定しない。**端末10台・5分間隔では同時実行が1を超えることすら稀であり、
制限を置く意味がない。

ただし**アカウント全体の同時実行上限(既定1000)は共有**されるため、暴走時にアカウント全体を
巻き込むリスクは残る。これは請求アラート(`02-infra.md` §2)で検知する方針とし、
関数側では絞らない。

**プロビジョンド同時実行も使わない。**コールドスタートを消す価値に対して定額課金が見合わない。
Go + `arm64` + 小さいバイナリなら、コールドスタートは実用上問題にならない。

### 2.4 IAM(最小権限)

関数ごとに実行ロールを分ける(`02-infra.md` §4 の `lambda` モジュールが1関数ぶんずつ作る)。

| 関数 | DynamoDB | S3 | その他 |
| --- | --- | --- | --- |
| `authorizer` | `GetItem`(テーブルのみ) | — | — |
| `web-api` | `GetItem` / `Query` / `PutItem` / `UpdateItem` / `DeleteItem` / `TransactWriteItems`(テーブル + GSI1) | `GetObject`(署名生成のため) | — |
| `device-auth` | `Query`(GSI2)/ `TransactWriteItems` / `UpdateItem` | — | — |
| `device-api` | `PutItem` / `UpdateItem` | **`PutObject`** | — |

- リソース指定は**テーブル ARN とインデックス ARN を明示**する。ワイルドカードを使わない
- `web-api` に `PutObject` は与えない。Web から画像が書き込まれる経路は存在しない
- `device-api` に `Query` は与えない。端末が他の観測を読む経路は存在しない

**`s3:GetObject` を持たないと署名付き URL は発行できても使えない。** 署名付き URL の権限は
署名者の権限を継承するため、`web-api` のロールに `GetObject` が必要になる。
署名生成そのものは API 呼び出しを伴わないが、権限は要る。

### 2.5 ログと可観測性

- ロググループは Terraform で明示的に作る。Lambda が暗黙に作るものは保持期間が無期限になる
  (`02-infra.md` §4)
- 保持期間は dev / prod とも **30日**
- **構造化ログ(JSON)で出力**する。`requestId` / `deviceId` / `ownerId` / エラーコードを含める
- **資格情報・`pairingCode`・署名付き URL をログに出さない**。署名付き URL はそれ自体が
  一定時間有効な認可であり、ログに残すと保持期間ぶん漏洩したのと同じになる

監視・アラートの具体は OPS-04 で決める。本書ではログの出し方までを定める。

### 2.6 冪等性

**アップロードは冪等にする。** 端末は送信成功の応答を受け取れずにリトライすることがあり、
そのたびに観測が二重に記録されると履歴が汚れる。

**`observationId`(ULID)は端末側で採番し、`metadata` に含めて送る。**サーバは
`attribute_not_exists(SK)` を条件に Put し、条件で落ちた場合も**成功として扱う**。

これにより「端末が同じ画像を2回送っても、記録は1つ」が構造的に担保される。
サーバ側で採番すると、リトライのたびに新しい ID が振られて重複を防げない。

ULID はタイムスタンプを含むため、端末側で採番しても SK の時系列順は保たれる。
端末の時計がずれていれば順序も狂うが、定点観測の用途では実害が小さく、
`capturedAt` も端末由来である以上、時計のずれは別の形で既に受け入れている。

S3 のキーにも同じ `observationId` を使うため、Put のやり直しは同じキーへの上書きになり、
孤児オブジェクトも増えない。

---

## 3. 設計判断

### 3.1 Web 向けの認証を Lambda で行わない

`/devices/*` などは API Gateway の JWT オーソライザ(Cognito)を使い、Lambda を挟まない
(`02-infra.md` §6)。Lambda の呼び出しもコールドスタートも発生しないため、自前で検証する理由がない。

Lambda オーソライザを使うのは端末側の経路だけである。こちらは DynamoDB を引く必要があり、
JWT の検証だけでは足りない。

### 3.2 オーソライザのキャッシュを切る

**レスポンスキャッシュは TTL = 0。** 「Web から端末を削除した瞬間に、有効期限が残っている
トークンでも拒否される」という即時失効が、毎リクエストの `GetItem` に支えられている
(`06-auth.md` §4)。

なお削除は論理削除(`status = ARCHIVED`)なのでレコードは残る。オーソライザは
「取得できたか」ではなく **`status = PAIRED` を確認する**。

認可コンテキストに `deviceId` と `ownerId` を載せ、後段の `device-api` が
再度 DynamoDB を引かずに済むようにする。

### 3.3 遅れて届いた観測で最新画像を巻き戻さない

端末はオフラインから復旧すると、最新の1枚を先に送り、残りを古い順に送る(`04-native.md` §3.5)。
このとき `Device.latestThumbnailKey` を素直に上書きすると、後から届く古い画像で
ダッシュボードのカードが巻き戻る。

そのため `latest*` の更新には `latestCapturedAt < :capturedAt` の条件を付ける。
一方 `lastReceivedAt` は条件なしで更新する。古い画像であっても「たった今受信した」事実は
変わらないためである。詳細と2段階の呼び出し方は `engineering/dynamodb.md` §5.1。

### 3.4 設定は Pull で返す

端末への設定配信は、アップロードのレスポンスに `nextConfig` を同梱する Pull 型で行う
(`00-overview.md` §6)。新規の AWS リソース(IoT Core 等)を追加せず、既存の Lambda 構成に
そのまま乗る。

**サーバは命令を push しない。** 端末が自発的に取得して適用する形を保つことで、
端末の状態遷移をサーバが直接動かす経路が存在しなくなる。切断も削除も、
端末が次に来たときに `401` として現れる(`06-auth.md` §5)。

### 3.5 Terraform は関数の器だけを持つ

`02-infra.md` §5 のとおり、Terraform は関数・IAM ロール・ロググループ・環境変数を管理し、
**コード本体は `lifecycle.ignore_changes` で管理対象外**とする。
コードの更新は CI が `aws lambda update-function-code` で直接行う。

これにより、コードをデプロイするたびに Terraform の plan にコード差分が出続けて
「plan をレビューしてから prod に適用する」運用がノイズに埋もれる、という事態を避けられる。

### 3.6 環境変数で渡すもの

| 変数 | 用途 | 出どころ |
| --- | --- | --- |
| `TABLE_NAME` | DynamoDB のテーブル名 | `data` モジュールの出力 |
| `IMAGES_BUCKET` | 画像バケット名 | `storage-images` の出力 |
| `RETENTION_DAYS` | 観測の `expiresAt` 計算、`/app-config` の応答 | env の変数(`02-infra.md` §4) |
| `DEVICE_LIMIT` | 端末数上限、`/app-config` の応答 | env の変数 |
| `SIGNED_URL_TTL` | 署名付き URL の有効期限(秒) | 既定 900 |

シークレットは渡さない。DynamoDB / S3 へのアクセスは実行ロールで行い、
Cognito の検証は API Gateway 側で完結する。**Secrets Manager も Parameter Store も使わない。**

---

## 4. デプロイ

`00-overview.md` §8 のとおり、インフラは手動 apply、Lambda コードは CI で自動デプロイ。

| 対象 | 契機 |
| --- | --- |
| Lambda(4関数) | dev は develop へのマージで自動、prod は手動実行またはタグ |

ビルドは `GOOS=linux GOARCH=arm64` でクロスコンパイルし、`bootstrap` という名前の
単一バイナリを zip に固める(`provided.al2023` の規約)。
具体的なワークフローは OPS-03 で定める。

**4関数を1リポジトリ・1モジュールで持ち、共有パッケージを跨いで使う。**
関数ごとにリポジトリを分けると、DynamoDB のアイテム定義のような共有型を
複製することになり、片方だけ直す事故が起きる。

---

## 5. 未確定事項

| 項目 | 参照 |
| --- | --- |
| 画像の解像度・圧縮率(上限 4.5MB の範囲で決める) | `01-openquestion.md` APP-03 |
| メモリ設定の実測に基づく見直し | OPS-05 |
| 監視・アラートの具体 | OPS-04 |
| GitHub Actions のワークフロー定義と Go のビルド方式 | OPS-03 |
| ペアリング開始のレート制限の実装方式(API Gateway の使用量プランか、DynamoDB のカウンタか) | AUTH-05 |
