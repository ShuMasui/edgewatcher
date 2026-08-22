# EdgeWatcher 認証・認可設計

最終更新: 2026-08-22
ステータス: 方針確定(具体値の一部が未確定 → `01-openquestion.md` §1)

関連: `00-overview.md` §5、`02-infra.md` §6、`05-backend.md`

## 1. 全体方針

利用者は2種類あり、認証の仕組みを完全に分離する。

| 主体 | 認証基盤 | 資格情報 |
| --- | --- | --- |
| Web ユーザー(オーナー) | Cognito Web User Pool | Google SSO のみ |
| 観測デバイス(Android) | **自前**(DynamoDB + Lambda Authorizer) | デバイスシークレット + セッショントークン |

### Device User Pool を廃止した理由

当初は端末用に Cognito User Pool をもう1つ用意し、`CUSTOM_AUTH` フローでパスワードレス認証を行う設計だった。
Device Pool を置く目的は Cognito のセッションライフサイクル(トークン発行・更新・失効)を借りることにあったが、
**Lambda Authorizer が毎リクエスト DynamoDB を引いて端末の有効性を確認する**構成を採る以上、
その役割は実質的に DynamoDB 側へ移っている。

Cognito に残るのは JWT の署名だけであり、その対価として次を抱えることになる。

- User Pool 1つ分の Terraform 管理と、属性変更による置換(登録済みデータの消失)リスク
- `DefineAuthChallenge` / `CreateAuthChallenge` / `VerifyAuthChallenge` の3トリガー
- ペアリング開始時に Cognito ユーザーを先に作る必要があり、ペアリング未完了のまま
  孤児ユーザーが残る問題(旧 AUTH-03、旧 AUTH-05)

得られるものに対して負債が大きいため、Device Pool は作らない。Cognito は Web 利用者の認証に限定する。

## 2. ペアリングフロー

QR コードの向きは「Web が表示 → 端末がスキャン」。OTP による二段階確認は行わない。

```text
[Web] オーナー(Cognito 認証済み)が「端末を追加」(端末名を入力)
   │ POST /devices  { name }
   ▼
[Lambda] TransactWriteItems(1コール)
   - Put : Device レコード(deviceId, ownerId, name, status=PENDING)
   - Put : PairingSession
           - pairingCode : 128bit ランダム(URL-safe base64)
           - deviceId, ownerId
           - status    : PENDING
           - expiresAt : now + 5分(TTL 属性も兼ねる)
   │ { deviceId, pairingCode, expiresAt }
   ▼
[Web] QR を描画(ペイロードは "ew1:<pairingCode>" のみ)
      一覧には「ペアリング待ち」の行が即座に出る
      GET /devices/{deviceId}/pairing-sessions/latest を2秒間隔でポーリング
   ▼
[端末] QR をスキャン → POST /device/pair(認証不要)
      { pairingCode, deviceInfo: { model, osVersion, appVersion } }
   ▼
[Lambda] TransactWriteItems(1コール)
   - Update : PairingSession を PENDING → CONSUMED
              条件 = status が PENDING かつ expiresAt > now
   - Update : Device を status=ACTIVE にし、deviceSecretHash と deviceInfo を設定
              条件 = 当該 deviceId のレコードが存在する
   │ { deviceId, deviceSecret }  ← deviceSecret を返すのはこの1回だけ
   ▼
[端末] EncryptedSharedPreferences に保存
      → POST /device/token でセッション取得 → Foreground Service 開始
[Web] ポーリングで CONSUMED を検知 → 「接続しました」に切り替わる
```

### Device レコードを先に作る

Device レコードはペアリング開始時点で `status = PENDING` として作り、`deviceId` は以後不変とする。
これにより、端末を初期化しても別の Android 端末に置き換えても、
**再ペアリングすれば同じ観測地点として過去の画像履歴が繋がり続ける**(`03-web.md` §5.3)。
Web の端末一覧にも「ペアリング待ち」の行が即座に現れるため、
オーナーが端末のところへ移動している間もペアリングが進行中であることが見える。

代償として、QR を発行したまま一度もスキャンされなかった端末が `PENDING` の行として残りうる。
これは自動削除せず、オーナーが明示的に削除する(`03-web.md` §5.4)。

### 使い捨ての担保

「読んで、PENDING か確かめて、書く」という手順に分けると、同一 QR の同時スキャンで二重に発行されうる。
そのため状態遷移は **`ConditionExpression` 付きの単一 Update** に畳み込み、
`TransactWriteItems` で Device の更新と 1 コールにまとめる。条件が満たされなければ両方とも書かれないため、
「QR は消費済みなのに端末が ACTIVE になっていない」という半端な状態が構造的に発生しない。

### 失効判定に TTL を使わない

DynamoDB の TTL による削除は最大48時間遅れることがあり、期限切れレコードがしばらく残る。
失効の判定は必ず条件式の `expiresAt > now` で行い、TTL 属性はレコードの掃除役に限定する。

### QR 漏洩に対する考え方

OTP を廃止したことで、QR のスクリーンショットが第三者に渡れば、その第三者の端末が
オーナーのアカウントに紐づく余地が生まれる。これに対しては **TTL 5分 + 一度きりの消費**のみで対応する。

判断の根拠は、この QR が「オーナーが目の前の端末を今まさにセットアップしている5分間」にしか
存在しないこと、そして先にスキャンされていればオーナー自身のペアリングが失敗して異常に気づけることによる。
承認ボタンによる追加確認は、得られる安全性に対して手順が増えすぎるため採らない。

## 3. デバイス資格情報とセッション

長期資格情報と短期セッションの2層構造を持つ。

| | 実体 | 端末側の保存 | サーバ側の保存 | 寿命 |
| --- | --- | --- | --- | --- |
| `deviceSecret` | 256bit ランダム | EncryptedSharedPreferences | SHA-256 ハッシュのみ | 無期限(再ペアリングまで) |
| `sessionToken` | `<deviceId>.<128bit ランダム>` | EncryptedSharedPreferences | SHA-256 ハッシュ + 失効時刻 | 12時間 |

```text
POST /device/pair    { pairingCode, deviceInfo }   → { deviceId, deviceSecret }
POST /device/token   { deviceId, deviceSecret }    → { sessionToken, expiresAt }
POST /device/uploads Authorization: <sessionToken> → { nextConfig }
POST /device/logout  Authorization: <sessionToken> → 204
```

`deviceSecret` はセッションの再取得にしか使わない。日常のアップロードには毎回 `sessionToken` を使うため、
長期資格情報がネットワークやログに露出する頻度を1日2回程度に抑えられる。

### ハッシュは SHA-256 で足りる

bcrypt や Argon2 のような遅いハッシュが必要なのは、人間が選んだ低エントロピーのパスワードを
オフライン総当たりから守るため。256bit の乱数に対しては総当たりが成立しないため、
Lambda のコールドスタートに KDF のコストを乗せる意味がない。

### セッションは端末につき1本、交換のたびにローテーション

`sessionToken` は Device レコード上に1つだけ保持し、`POST /device/token` のたびに新しい値で上書きする。
これにより古いトークンは即座に無効化され、同時に複数のセッションが生きる状態が発生しない。

トークンの先頭に `deviceId` を置いているのは、Authorizer が **`DEVICE#<deviceId>` への GetItem 1回**で
次の4点をまとめて判定できるようにするため。

1. 端末レコードが存在するか(= Web から削除されていないか)
2. `status` が `ACTIVE` か
3. `sessionTokenHash` が一致するか
4. `sessionExpiresAt` を過ぎていないか

これは `01-openquestion.md` DATA-01 が挙げていた「Lambda Authorizer が端末の存在と有効性を
1回の GetItem で確認する」というアクセスパターンをそのまま満たす。

## 4. Lambda Authorizer

- API Gateway HTTP API の **simple response**(`{ isAuthorized, context }`)形式を使う
- 認可コンテキストに `deviceId` と `ownerId` を載せ、後段の Lambda が再度 DynamoDB を引かずに済むようにする
- **レスポンスキャッシュは無効(TTL = 0)**

キャッシュを切るのは、`00-overview.md` §5 が謳う「削除された瞬間、有効期限が残っている
トークンでも即座に拒否される」という即時失効が、毎リクエストの GetItem に依存しているため。
キャッシュを有効にすると、その TTL の間だけ削除済みの端末がアップロードを続けられてしまう。

GetItem 1回のコストと引き換えに即時失効を取る、という明示的なトレードオフとして選択している。

## 5. 401 を起点とした端末の自己修復

端末はサーバから 401 を受けたときの挙動だけで状態を回復する。プッシュ通知や別チャネルは持たない。

```text
アップロードが 401
   └→ POST /device/token でセッション再取得を試みる
        ├─ 成功 → 新しい sessionToken で再送。以後継続
        └─ 401  → deviceSecret が無効
                  (= Web 側で「セッション切断」されたか、端末が削除された)
                  ローカルの資格情報を全消去
                  Foreground Service を停止
                  未ペアリング画面(QR スキャナ)に戻る
```

Web からの操作が、端末側の後始末まで自動的に波及する。端末に対して切断や削除を能動的に通知する
仕組みを持たずに済むのは、端末が必ず定期的にサーバへ来る性質を利用できるため。

## 6. 端末の状態と、切断・ログアウト・削除

### 状態モデル

`Device.status` は3値を取る。表示上の「応答なし」は `ACTIVE` からの派生であり、永続化しない。

| `status` | 意味 | Web での表示 |
| --- | --- | --- |
| `PENDING` | 作成済みだがまだ一度もスキャンされていない | ペアリング待ち |
| `ACTIVE` | 資格情報が有効 | 接続中 / 応答なし(最終受信時刻で派生) |
| `REVOKED` | Web から資格情報を失効させた | 切断済み |

「Web から切断した端末」と「単に圏外・電源断の端末」を `status` で区別できるため、
最終受信時刻の閾値だけで両者を推測する必要がない(`01-openquestion.md` AUTH-09)。

### セッション切断が `sessionToken` だけでは成立しない理由

Web からの「セッション切断」で `sessionTokenHash` のみを消すと、端末は 401 を受けたあと
手元の `deviceSecret` を使って数秒で新しいセッションを取得してしまい、切断にならない。

したがって切断は **`sessionTokenHash` と `deviceSecretHash` の両方を無効化し、
`status` を `REVOKED` にする**操作と定義する。復帰には Web からの再ペアリングが必要になる。

### 3つの操作の違い

| 操作 | 実行主体 | Device レコード | 画像履歴 | 復帰方法 |
| --- | --- | --- | --- | --- |
| セッション切断 | Web | 残る(`REVOKED`) | 残る | Web から再ペアリング |
| ログアウト | 端末 | 残る(`REVOKED`) | 残る | Web から再ペアリング |
| 削除 | Web のみ | 消える | 消える | なし(新規登録) |

**ログアウトとセッション切断は、サーバ側の結果としては同じ**(資格情報を失効させ `REVOKED` にする)。
違いは実行主体だけである。端末側でログアウトするとローカルの `deviceSecret` は失われるため、
サーバ側で `deviceSecret` を生かしておいても端末には復帰する手段がない。
したがって `POST /device/logout` は `sessionTokenHash` と `deviceSecretHash` の両方を消し、
Web の一覧に「切断済み」として現れて [再ペアリング] ボタンが出る状態に揃える。

端末からの削除は許可しない。屋外に常設された端末が物理的に触られただけで、
オーナーの管理下からレコードごと消えてしまう事態を避けるため。

オフライン中にログアウトされた場合、この呼び出しは失敗しサーバ側は `ACTIVE` のまま残る。
その端末は Web 上では「応答なし」として見え、オーナーが手動で [セッション切断] を押せば
`REVOKED` に揃う。端末側は既にローカル資格情報を捨てているため、実害はない。

## 7. Web ユーザーの認証

- Cognito Web User Pool。**Google を唯一の IdP とし、メール/パスワード認証は提供しない**。
  ログイン画面は「Google で続行」1つだけになり、サインアップ・確認メール・パスワード再設定の
  3画面とメール送信基盤が丸ごと不要になる。アバター画像も Google のプロフィールから取得する
- API Gateway の **JWT オーソライザ**(組み込み)で検証する。Lambda を経由しないため
  コールドスタートも追加コストも発生しない
- Web 向けの各 API は、JWT の `sub` を `ownerId` として扱い、
  自分が所有する端末・観測データ以外にアクセスできないことを毎回確認する

## 8. 未確定事項

`01-openquestion.md` §1 を参照。本設計の変更により、以下は解消した。

- AUTH-01(OTP の桁数)— OTP 自体を廃止
- AUTH-03(Device レコード作成トリガー)— Cognito トリガーを使わず、1つの Lambda 内で完結
- AUTH-06(OTP 再送)— OTP 自体を廃止

AUTH-02(メール送信)は、ペアリングのクリティカルパスから外れて
「Cognito のサインアップ確認メールを標準送信のままにするか」という低優先度の論点に縮小した。
