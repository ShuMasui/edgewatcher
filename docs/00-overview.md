# EdgeWatcher 概要設計

最終更新: 2026-08-20
ステータス: 設計中(基本方針確定、詳細設計は各コンポーネントごとに別途詰める)

## 1. プロジェクトの目的

- ポートフォリオとして開発するアプリケーション
- コンセプト: 余っているAndroid端末や格安で入手できるAndroid端末を、無数のセンサーを組み合わせた
  「最強のエッジデバイス」と位置づけ、対候性を持つ**エッジ定点観測デバイス**として活用するプロジェクト
- 型番を揃えることが難しい前提(余剰端末・格安端末の寄せ集め)のため、**端末を自由に追加できる**構造とする

## 2. システム全体像

EdgeWatcherは3つのコンポーネントで構成される。

- **EdgeWatcher Android App**: 定点観測デバイスにインストールし、画像・位置情報を定期配信する
- **EdgeWatcher Backend**: AWS上のサーバーレス構成。配信された画像を受け取り、保存・配信する
- **EdgeWatcher Web App**: 観測画像の閲覧、および端末自体の登録・管理を行う

```
[EdgeWatcher Android App (Kotlin)]
   │ 画像+位置情報を定期送信 (Foreground Service)
   ▼
[API Gateway (HTTP API)] ── [Lambda Authorizer] (Cognito JWT検証 + DynamoDBで端末生存確認)
   │
   ▼
[Lambda (Go)] ─┬─> [S3: 画像バケット] (30日でライフサイクル削除)
               └─> [DynamoDB: シングルテーブル]

[EdgeWatcher Web App (React/Vite SPA)]
   │ S3(静的ホスティング) + CloudFront + ACM + Route53
   ▼
[API Gateway] → [Lambda (Go)] → DynamoDB / S3(署名付きURLで画像取得)

[Cognito]
  - Web User Pool (Webユーザーのサインアップ/ログイン)
  - Device User Pool (端末のIoT的な認証、パスワードレス)
```

### 使用AWSリソース

Cognito(×2 User Pool)、API Gateway(HTTP API)、Lambda(Go)、Lambda Authorizer、DynamoDB(シングルテーブル)、
S3(×2: フロントエンド配信用 / 画像保存用)、CloudFront、ACM、Route53。全てTerraformでIaC管理。

## 3. スコープ(MVP)

- **センサー**: カメラ + 位置情報(GPS)のみ。それ以外のセンサー(温湿度・気圧等)は将来拡張とし、今回の実装対象外
- **AI活用**: 「最大限AIを活用」は開発プロセス側(Claude Code等による高速開発・継続的な保守)を指す。
  アプリ自体に画像解析等のAI推論機能は今回のスコープに含めない(バックエンドもRekognition/Bedrock等は前提としない)

## 4. Web UX方針

- ダッシュボード: 所有する全端末の**最新スナップショットを一覧表示**(定点カメラの「今の状態」を確認する用途)
- 端末をクリックすると、その端末の**タイムライン/履歴**(定点観測の時系列変化)を閲覧できる
- 地図表示は今回のスコープ外

## 5. 端末ライフサイクル・認証設計

### Cognito構成

Web User PoolとDevice User Poolを分離する。Web利用者のログイン(メール/パスワード)と端末のIoT的な認証
(QRペアリング経由でのみ払い出し、パスワードレス)は性質が異なるため、Poolを分けることで権限スコープの
取り違えを構造的に防ぐ。

### ペアリングフロー(QR + OTPの二段階認証)

QRコードの漏洩(スクリーンショット等)だけでは端末をペアリングできないよう、QRの使い捨てコードに加えて
端末画面に表示される5桁OTPをWeb側で追加入力させる方式とする。

1. Web: オーナーが「端末を追加」を実行 → Lambdaが `PairingSession` をDynamoDBに作成
   (使い捨てpairingCode、TTL 5分、status=PENDING) → QRコードとしてWebに表示
2. 端末: EdgeWatcherアプリでQRをスキャン → `pairingCode` をバックエンドに送信
3. バックエンド: コードを検証 → status=CLAIMEDに更新 → 5桁OTPを生成しDynamoDBに保存 → 端末にOTPを返す
4. 端末: 画面にOTPを表示(「オーナーにこの番号を伝えてください」)
5. Web: ステータスをポーリングしCLAIMEDを検知 → OTP入力欄を表示 → オーナーが端末の画面を見てOTPを入力 → 送信
6. バックエンド: OTP一致を検証 → 一致すればDevice User Pool上に端末用Cognitoユーザーを作成しトークン発行、
   DynamoDBに `Device` レコードを作成、status=CONFIRMED
7. 端末: ポーリングでCONFIRMEDを検知 → トークン(IdToken/AccessToken/RefreshToken)を受信し暗号化保存
   (EncryptedSharedPreferences) → 以後、定期アップロードを開始
8. OTP不一致・タイムアウト時: status=FAILEDとし、トークンは発行しない(最初からやり直し)

### 端末側UI(常時観測デバイス前提で最小限)

- 未ペアリング時: QRスキャナ + 発行された5桁OTPの表示のみ
- ペアリング済み時: 「接続中のアカウント: `<メールアドレス等>`」の表示 + 「ログアウト」ボタンのみ
- 常時給電(モバイルバッテリー等)を前提とするため、省電力のための機能制限は設けない

### 削除とログアウトの違い

- **ログアウト(端末側から可能)**: ローカル保存トークンを破棄しForeground Serviceを停止、未ペアリング状態に
  戻るクライアント側のみの操作。バックエンド側のDeviceレコード・Cognitoユーザーはそのまま残り、
  Webの一覧には「未接続」として表示され続ける
- **削除(Web側からのみ可能)**: 端末自体からは削除できない。Lambdaが
  (a) Cognito Device Poolの当該ユーザーを `AdminDeleteUser` で完全削除(以後のリフレッシュ不可)、
  (b) DynamoDBのDeviceレコードを削除、の2つを行う。さらに **Lambda Authorizerが毎リクエストDynamoDBの
  Device存在確認を行う**ため、削除された瞬間、有効期限が残っているAccessTokenでも即座に拒否される
  (JWT自体の有効期限だけに頼らない実質的な即時失効)

## 6. 配信・送信ポリシー

- **送信間隔**: 5分・10分・15分から選択可能(初期値は別途決定)
- **設定配信方式**: Pull型。端末の定期アップロードのレスポンスに次回までの設定値(送信間隔など)を同梱する。
  新規AWSリソース(AWS IoT Core等)を追加せず、既存のLambda構成にそのまま乗せられるため。
  設定変更は「最大1送信サイクル遅延」で反映される(次回送信タイミングで反映)
- **オフラインバッファリング**: 端末側で一定件数/容量上限付きのローカル保持を行い、復旧時に順次送信する。
  上限を超えた古いデータは削除(最新優先)
- **画像保持期間**: S3ライフサイクルルールにより一定期間後(例: 30日)に自動削除
- **端末所有モデル**: 1人のWebユーザーが複数の端末を所有・管理できる

## 7. 技術スタック

| 領域 | 選定 |
|---|---|
| Androidアプリ | Kotlin。バックグラウンド処理はForeground Service(常駐通知)+内部タイマーで実現 |
| Webフロントエンド | React(Vite)のSPA。S3 + CloudFrontで静的配信 |
| バックエンドAPI | API Gateway(HTTP API) + Lambda + Lambda Authorizer |
| Lambda実行言語 | Go |
| データストア | DynamoDB(シングルテーブル設計) |
| 画像ストレージ | S3(署名付きURLでWebから参照、直接公開しない) |
| IaC | Terraform |
| モバイル配信 | Google Play Storeの限定配信(審査軽量化と継続配信の両立) |

### 技術的注意点

Android の `WorkManager` の `PeriodicWorkRequest` はOS側の制約で最短間隔が15分であり、5分・10分間隔の
定期実行を実現できない。そのため、常時観測デバイスという用途に即し、**Foreground Service + 内部タイマー**
方式を採用する。専用ハードウェアとして常時稼働させる前提のため、常駐通知が表示されることは許容する。

## 8. 環境構成

`prod` / `dev` / `stg` の3環境を分離する。

| 環境 | 用途 |
|---|---|
| `prod` | 本番環境。Google Play Storeの限定配信でリリースされたアプリが接続する |
| `dev` | 開発環境。開発中の動作確認用 |
| `stg` | CI上で回すための検証環境。自動テスト・デプロイ検証などに使用 |

各環境ごとにAWSリソース(Cognito Pool×2、DynamoDB、S3、API Gateway、CloudFrontなど)を独立して持つ。

Terraformの構成方針:

- **モジュール分離**: リソース定義は `modules/` 配下に共通モジュールとして切り出す(Cognito、API、Storage等の単位)
- **envディレクトリ分離**: `envs/dev`, `envs/stg`, `envs/prod` のように環境ごとにディレクトリを分け、
  各ディレクトリから共通モジュールを呼び出す
- **変数分離**: 環境ごとの値(ドメイン名、リソース名のsuffix等)は各envディレクトリ配下の変数ファイル
  (`terraform.tfvars`等)で分離して管理する
- **state分離**: tfstateも環境ごとに完全に分離する。誤って本番環境に適用してしまうリスクを避ける

## 9. 未確定・次回検討事項

以下は本ドキュメントの時点ではまだ詳細を詰めていない。各コンポーネントの実装計画を立てる際に別途設計する。

- DynamoDBの詳細なテーブル/インデックス設計(シングルテーブル設計の方針は決定済み、PK/SK設計は未確定)
- Androidアプリの詳細設計(カメラ/位置情報取得の実装方式、ローカルバッファの実装方式など)
- Webアプリの詳細設計(画面遷移、状態管理など)
- CI/CD・Terraformのモジュール構成などの運用設計
- 非機能要件(監視・アラート、コスト試算など)
