# EdgeWatcher インフラ設計

最終更新: 2026-08-22
ステータス: 方針確定(ルートドメイン名のみ未確定 → `01-openquestion.md` OPS-01)

関連: `00-overview.md` §8(環境構成)、`06-auth.md`(認証設計)

## 1. 前提

- リージョンは **us-east-1**(東京)。観測デバイス・利用者ともに国内を想定しており、
  アップロードのレイテンシと転送コストの両面で最短になる
- AWS アカウントは **単一**。dev / prod はリソース名のプレフィックスで分離する
- 全リソースを Terraform で管理する。コンソールでの手動変更は行わない

### 単一アカウントを選んだ理由

Organizations によるアカウント分離のほうが事故の爆発半径は小さいが、Route53 ホストゾーンを
1つに集約する方針(§3)と組み合わせると、ACM の DNS 検証レコードを別アカウントのゾーンに書くために
クロスアカウントのロール引き受けが必要になる。個人開発の規模に対して構築と運用の複雑さが見合わない。

代わりに、次の2点で環境の取り違えを防ぐ。

- リソース名を必ず `edgewatcher-<env>-<name>` で統一し、名前だけで環境が判別できるようにする
- tfstate を完全に分離し(§2)、apply 時に参照する state が物理的に別ファイルである状態を保つ

## 2. Terraform のスタック構成

state を3層に分ける。

| スタック    | 持つもの                                                             | apply 頻度     | state の置き場                |
| ----------- | -------------------------------------------------------------------- | -------------- | ----------------------------- |
| `bootstrap` | tfstate 用 S3 バケット                                               | ほぼ一度きり   | 自分自身が作ったバケット      |
| `shared`    | Route53 ホストゾーン、GitHub Actions 用 OIDC プロバイダと IAM ロール | 年数回         | `shared/terraform.tfstate`    |
| `envs/dev`  | dev のアプリケーションリソース一式                                   | 機能追加のたび | `envs/dev/terraform.tfstate`  |
| `envs/prod` | prod のアプリケーションリソース一式                                  | 機能追加のたび | `envs/prod/terraform.tfstate` |

```text
infra/
  bootstrap/
  shared/
  envs/
    dev/     ( main.tf / iam.tf / variables.tf / outputs.tf / terraform.tfvars )
    prod/
  modules/
    domain/
    cognito-web/
    data/
    storage-images/
    web-hosting/
    api/
    lambda/
  placeholder/    ( 疎通確認用の index.html )
```

### ロックに DynamoDB を使わない

当初はロック用の DynamoDB テーブルも `bootstrap` に含める設計だったが、
**Terraform 1.10 以降は S3 のネイティブロック(`use_lockfile = true`)が使える**ため不要。
`bootstrap` は S3 バケット1つだけになる。管理するリソースが減り、
「ロックテーブルだけ消えて apply が通らない」という事故の種も消える。

### 未確定事項を optional 変数で切り離す

`root_domain`(OPS-01)と `google_client_id`(AUTH-07)が未確定でも構築を止めないため、
**これらを `null` 許容の変数にし、`count` で関連リソースの作成を切り替える**。

| 変数               | `null` のとき                                                                                |
| ------------------ | -------------------------------------------------------------------------------------------- |
| `root_domain`      | Route53・ACM・カスタムドメインを作らない。CloudFront と API Gateway の既定ドメインで動作する |
| `google_client_id` | Google IdP を作らない。User Pool と Hosted UI だけが立つ                                     |

確定したら値を埋めて apply するだけでよく、**コードの書き換えは発生しない**。
同じ仕組みを2つの異なる外部依存に適用することで、扱い方が1つに揃う。

### `bootstrap` の鶏卵問題

tfstate の置き場そのものを Terraform で作るため、初回だけローカル state で apply し、
生成された S3 バケットへ `terraform init -migrate-state` で state を移す。この手順は一度きりであり、
以後 `bootstrap` を触ることはほとんどない。

### 手動で行うアカウントのブートストラップ手順

Terraform を動かすための資格情報そのものは Terraform では作れない。以下はコンソール/CLI で
**手動で一度だけ**実施し、Terraform の管理対象には入れない。

1. ルートユーザーに MFA を設定し、以後ルートユーザーは使わない
2. 請求アラート(Budgets)を設定する
3. 開発者用の IAM ユーザーを1つ作成する
   - MFA を必須にする
   - 権限は直接アタッチせず、`AdministratorAccess` を持つ IAM グループ経由で付与する
4. そのユーザーからアクセスキーを発行し、ローカルの `~/.aws/credentials` にプロファイルとして置く
5. `bootstrap` を apply し、state を S3 に移行する

続いて `shared` / `envs` を apply する。

### 開発者 IAM ユーザーを Terraform 管理下に置かない理由

「**Terraform の入力になる ID は手動、Terraform の出力になる ID は Terraform**」で線を引く。

| ID                                                   | 作り方                         |
| ---------------------------------------------------- | ------------------------------ |
| 開発者本人の IAM ユーザー(+ MFA、アクセスキー)       | 手動。Terraform の管理対象外   |
| GitHub Actions の OIDC ロール(plan / apply / deploy) | Terraform(`shared`)            |
| Lambda の実行ロール                                  | Terraform(`lambda` モジュール) |

理由は3点。

- **鶏卵問題**: `bootstrap` の apply 自体に AWS 資格情報が要るため、それを作る ID を
  Terraform で用意することはできない
- **アクセスキーが state に平文で残る**: `aws_iam_access_key` を使うとシークレットが tfstate に
  そのまま書き込まれる。その tfstate を置くバケットは `bootstrap` が作るものであり、
  鍵を守る入れ物を鍵入りの state で管理する循環になる
- **自分の権限を自分で書き換えられる構成は危険**: apply の実行主体を apply が変更できると、
  ポリシーの記述ミス1回でアカウントから締め出されうる。復旧にルートユーザーが必要になる

### 一時認証情報での運用(ブートストラップ後の締め直し)

上記の手順3では、`bootstrap` を apply できる必要があるため開発者ユーザーに `AdministratorAccess` を
直接与えている。ブートストラップが終わったら、日常的に長期アクセスキーで管理者権限を使わない形に締め直す。

- 作業用のロール(`edgewatcher-developer`)を Terraform で作り、信頼ポリシーで
  「MFA 認証済みの開発者ユーザーからのみ引き受け可能」とする
- `~/.aws/config` のプロファイルに `role_arn` と `mfa_serial` を書く。
  AWS CLI も Terraform も自動でロールを引き受け、以後の操作は一時認証情報で行われる
- 手動作成した IAM ユーザーの `AdministratorAccess` グループは**外さずに残す**。
  これがロールのポリシーを壊したときの復旧経路(ブレークグラス)になる

つまり、日常の操作は一時認証情報に寄せつつ、締め出しからの復旧手段だけは
Terraform の外側に手動で残しておく、という二重化にする。

### `shared` を分けた理由

Route53 ホストゾーンを1つだけ作って dev / prod で共有する(§3)以上、そのホストゾーンは
dev の state にも prod の state にも属せない。同じ性質(環境を跨ぐ・低頻度・消えると復旧が重い)を持つ
tfstate バケットと OIDC プロバイダも同じ層にまとめる。

### env から shared への参照方法

env スタックはホストゾーンを **`terraform_remote_state` ではなく `data "aws_route53_zone"`** で
ドメイン名から引く。

```hcl
data "aws_route53_zone" "root" {
  name = var.root_domain
}
```

`terraform_remote_state` を使うと env が shared の state ファイルの内部構造(output 名)に依存し、
shared 側のリファクタリングが env を壊す。ドメイン名という外部から不変な識別子で引けば、
両者の結合はその文字列1本だけで済む。

## 3. ドメインと証明書

ルートドメインを1つ取得し、ホストゾーンも1つ。環境ごとにサブドメインで分ける。

| 用途             | prod                | dev                     |
| ---------------- | ------------------- | ----------------------- |
| Web(CloudFront)  | `app.<root-domain>` | `app.dev.<root-domain>` |
| API(API Gateway) | `api.<root-domain>` | `api.dev.<root-domain>` |

ホストゾーンは `shared` が持ち、A/AAAA レコードは各 env スタックが自分の分だけを作る。
レコード名に環境が含まれるため、env 同士がレコードを奪い合うことはない。

### ACM 証明書は環境ごとに2枚必要

- **CloudFront 用**: `us-east-1` に存在しなければならない(CloudFront の仕様)
- **API Gateway カスタムドメイン用**: API と同じ `us-east-1` に必要

`modules/domain` がこの2枚をまとめて作り、呼び出し側から2つの provider エイリアスを受け取る。
リージョン制約をモジュールの内側に閉じ込め、env 側のコードに `us-east-1` が漏れないようにする。

```hcl
module "domain" {
  source    = "../../modules/domain"
  providers = {
    aws           = aws            # us-east-1
    aws.us_east_1 = aws.us_east_1
  }
  zone_id     = data.aws_route53_zone.root.zone_id
  web_domain  = var.web_domain
  api_domain  = var.api_domain
}
```

検証方式は DNS 検証。検証レコードもこのモジュールが作るため、apply 1回で証明書発行まで完了する。

## 4. モジュールの粒度

切る基準は2つ。**一緒に置換されるか**(ライフサイクルが同じか)と、
**外に見せるインターフェースがあるか**(出力が他モジュールの入力になるか)。

| モジュール       | 責務                                                                      | 主な出力                            |
| ---------------- | ------------------------------------------------------------------------- | ----------------------------------- |
| `domain`         | ACM 証明書2枚 + DNS 検証レコード                                          | 証明書 ARN ×2                       |
| `cognito-web`    | Web User Pool、App Client、Google IdP、Hosted UI ドメイン                 | User Pool ID、Client ID、issuer URL |
| `data`           | DynamoDB シングルテーブル(TTL 属性・GSI を含む)                           | テーブル名、テーブル ARN            |
| `storage-images` | 画像バケット、ライフサイクルルール、パブリックアクセス全遮断              | バケット名、バケット ARN            |
| `web-hosting`    | 配信バケット、CloudFront ディストリビューション、OAC、SPA フォールバック  | ディストリビューション ドメイン名   |
| `api`            | API Gateway HTTP API、ルート定義、オーソライザ2種、カスタムドメイン紐付け | API エンドポイント、実行 ARN        |
| `lambda`         | Lambda 関数1つ分(関数・IAM ロール・ロググループ・権限)                    | 関数 ARN、Invoke ARN                |

### `retention_days` は3箇所に配る

保持期間(dev = 1、prod = 7)は env の変数として1箇所で定義し、そこから3つの宛先に配る。

| 宛先                                | 用途                                           |
| ----------------------------------- | ---------------------------------------------- |
| `storage-images`                    | S3 ライフサイクルルールの日数                  |
| `lambda` の環境変数                 | 観測レコードの `expiresAt`(DynamoDB TTL)の計算 |
| API のレスポンス(`GET /app-config`) | Web が履歴を遡れる範囲(`retentionDays`)の決定  |

同じ値が S3 と DynamoDB の両方に効くため、変数を1つにしておかないと
「画像はないのにレコードだけ残る」ずれが生じる。

### Cognito モジュールが1つで済む理由

当初は Web User Pool と Device User Pool の2つを想定していたが、**Device User Pool は廃止した**
(`06-auth.md` §1)。端末の認証は Lambda Authorizer と DynamoDB で完結するため、
Cognito は Web 利用者の認証だけを担う。結果として「2つの Pool を1モジュールにまとめるか分けるか」
という論点自体が消えた。

### `lambda` を汎用モジュールにする理由

Go の Lambda 関数は4つある(`web-api` / `device-auth` / `device-api` / `authorizer`。
分割の根拠は `05-backend.md` §1.1)。
関数ごとに IAM ロール・ロググループ・保持期間・環境変数の定型を書き直すのは変更漏れの温床になるため、
「関数1つ = モジュール1呼び出し」の形にする。ロググループを Terraform で明示的に作るのは、
Lambda が暗黙に作るロググループには保持期間が設定されず(無期限保存)、コストが際限なく積み上がるため。

## 5. コードとインフラの境界

`00-overview.md` §8 では、インフラは手動 apply、Lambda コードは CI で自動デプロイ、と決めている。
Lambda 関数はこの境界を跨ぐ唯一のリソースであり、扱いを明示しておく必要がある。

**Terraform は関数の「器」だけを管理し、コード本体は管理対象外とする。**

- 初回作成時のみ、ダミーの zip(空のハンドラ)で関数を作る
- `filename` と `source_code_hash` を `lifecycle.ignore_changes` に入れる
- 以後のコード更新は CI が `aws lambda update-function-code` で直接行う

```hcl
resource "aws_lambda_function" "this" {
  # ...
  lifecycle {
    ignore_changes = [filename, source_code_hash]
  }
}
```

これをやらないと、コードをデプロイするたびに Terraform の plan にコード差分が出続け、
「plan をレビューしてから prod に適用する」という運用(`00-overview.md` §8 の手動 apply の根拠)が
ノイズに埋もれて機能しなくなる。逆にメモリ・タイムアウト・環境変数といった構成値は Terraform 側に残るため、
インフラ変更としてレビュー対象に載り続ける。

## 6. API Gateway とオーソライザ

HTTP API を1つ作り、経路によってオーソライザを使い分ける。

| 経路                                | オーソライザ        | 実体                                             |
| ----------------------------------- | ------------------- | ------------------------------------------------ |
| `/devices/*`(Web からの端末管理)    | JWT オーソライザ    | Cognito Web Pool の issuer を指定。Lambda 不要   |
| `/observations/*`(Web からの閲覧)   | JWT オーソライザ    | 同上                                             |
| `/app-config`(Web の環境定数)       | JWT オーソライザ    | 同上。非機密だが例外を作らない(`03-web.md` §3.8) |
| `/device/token`, `/device/pair`     | なし(認証前)        | Lambda 内でコード/シークレットを検証             |
| `/device/uploads`, `/device/logout` | Lambda オーソライザ | DynamoDB を引いて端末セッションを検証            |

**Lambda オーソライザのレスポンスキャッシュは無効(TTL = 0)にする。** Web から端末を削除した瞬間に、
有効期限が残っているトークンでも拒否される、という即時失効の性質は毎リクエストの GetItem が支えている。
キャッシュを効かせるとその性質が壊れる。詳細は `06-auth.md` §4。

Web 側に Cognito 組み込みの JWT オーソライザを使うのは、Lambda の呼び出しもコールドスタートも
発生しないため。自前で検証する理由がない。

## 7. ストレージ

### 画像バケット(`storage-images`)

- パブリックアクセスを全ブロック。Web からの参照は署名付き URL のみ
- ライフサイクルルールで `var.retention_days` 日後に自動削除(dev = 1、prod = 7)。
  DynamoDB 側の TTL と同じ値を使い、「画像はないのにレコードだけ残る」状態を避ける
- **両環境とも自動削除を有効にする。** dev だけ無効にすると、開発中に溜まった画像が
  無期限に残ってコストが積み上がる
- バージョニングは無効。上書きが発生しない書き込み専用の用途であり、コストに見合わない

### Web 配信バケット(`web-hosting`)

- S3 は静的ウェブサイトホスティングを **使わず**、CloudFront から OAC(Origin Access Control)で参照する。
  バケットを一切公開せずに済み、HTTPS を CloudFront 側で終端できる
- SPA のため、403 / 404 を `/index.html` に 200 で書き換えるカスタムエラーレスポンスを設定する

## 8. CI/CD と AWS 認証(OPS-03)

長期のアクセスキーは発行しない。`shared` に GitHub OIDC プロバイダを1つ作り、
**plan 用と apply 用でロールを分離**する。

| ロール                        | 権限                                                             | 使うワークフロー                  |
| ----------------------------- | ---------------------------------------------------------------- | --------------------------------- |
| `edgewatcher-ci-plan`         | 読み取り専用(`ReadOnlyAccess` 相当)+ tfstate の読み取り          | PR 時の `terraform plan`          |
| `edgewatcher-ci-apply-dev`    | dev リソースへの書き込み                                         | `workflow_dispatch` の dev apply  |
| `edgewatcher-ci-apply-prod`   | prod リソースへの書き込み                                        | `workflow_dispatch` の prod apply |
| `edgewatcher-ci-deploy-<env>` | Lambda の `UpdateFunctionCode`、S3 同期、CloudFront invalidation | アプリコードのデプロイ            |

PR で自動実行される plan には読み取りロールしか渡らないため、PR 経由で本番リソースが変更される経路が
構造的に閉じる。apply ロールは信頼ポリシーで `workflow_dispatch` を起点とするワークフロー
(かつ特定ブランチ)からのみ引き受け可能にする。

`00-overview.md` §8 の「必ず dev → prod の順で apply する」という運用ルールは、
prod の apply ワークフローの前段で「直近の dev apply が成功しているか」を確認するステップを置いて補助する
(強制はせず、警告に留める)。

## 9. 命名とタグ

- リソース名: `edgewatcher-<env>-<name>`(例: `edgewatcher-dev-images`, `edgewatcher-prod-api`)
- 共通タグ(`default_tags` で provider に設定): `Project = edgewatcher`, `Environment = <env>`,
  `ManagedBy = terraform`

グローバルに一意な名前が要る S3 バケットには、衝突回避のためアカウント ID のサフィックスを付ける。

## 10. 未確定事項

- ルートドメイン名(`01-openquestion.md` OPS-01)
- 監視・アラートの具体(OPS-04)。CloudWatch アラームは `observability` モジュールとして
  後から追加する想定で、今回のモジュール一覧には含めていない
- コスト試算(OPS-05)
