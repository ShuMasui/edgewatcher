# EdgeWatcher インフラ

Terraform で管理する AWS リソース一式。設計の根拠は `docs/02-infra.md`。

```
infra/
  bootstrap/        tfstate 用 S3 バケット(ほぼ一度きり)
  shared/           GitHub OIDC、CI ロール、Route53 ホストゾーン
  envs/dev/         dev のアプリケーションリソース一式
  modules/          共通モジュール7つ
  placeholder/      疎通確認用の index.html
```

## 前提: 未確定事項の扱い

**2つの未決事項がブロッカーになりうるため、optional 変数で切り離してある。**

| 変数 | `null` のとき | 参照 |
| --- | --- | --- |
| `root_domain` | Route53・ACM・カスタムドメインを作らない。`*.cloudfront.net` と `execute-api` の既定 URL で動作する | OPS-01 |
| `google_client_id` | Google IdP を作らない。User Pool と Hosted UI だけが立つ(この間は誰もログインできない) | AUTH-07 |

**確定したら値を埋めて apply するだけでよい。**コードの書き換えは不要。

---

## Phase 0 — 人間が手動で行う(Terraform 管理外)

Terraform を動かすための資格情報そのものは Terraform では作れない。
「**Terraform の入力になる ID は手動、Terraform の出力になる ID は Terraform**」で線を引く
(`docs/02-infra.md` §2)。

以下はコンソール / CLI で**一度だけ**実施する。

1. **ルートユーザーに MFA を設定**し、以後ルートユーザーは使わない
2. **請求アラート(Budgets)を設定**する
3. **開発者用の IAM ユーザーを1つ作成**する
   - MFA を必須にする
   - 権限は直接アタッチせず、`AdministratorAccess` を持つ IAM グループ経由で付与する
4. そのユーザーから**アクセスキーを発行**し、`~/.aws/credentials` にプロファイルとして置く

```ini
[edgewatcher]
aws_access_key_id     = ...
aws_secret_access_key = ...
region                = ap-northeast-1
```

```sh
export AWS_PROFILE=edgewatcher
aws sts get-caller-identity   # 通ることを確認
```

> **なぜ IAM ユーザーを Terraform で作らないか**
>
> - **鶏卵問題**: `bootstrap` の apply 自体に資格情報が要る
> - **アクセスキーが state に平文で残る**: 鍵を守る入れ物を、鍵入りの state で管理する循環になる
> - **自分の権限を自分で書き換えられる構成は危険**: ポリシーの記述ミス1回でアカウントから締め出されうる

### ブートストラップ後の締め直し

日常的に長期アクセスキーで管理者権限を使わない形にする。作業用ロール
`edgewatcher-developer` を用意し、`~/.aws/config` に `role_arn` と `mfa_serial` を書く。
手動作成した IAM ユーザーの `AdministratorAccess` は**外さずに残す**
(ロールのポリシーを壊したときのブレークグラス)。

---

## Phase 1 — bootstrap

tfstate の置き場そのものを Terraform で作るため、初回だけローカル state で apply する。

```sh
cd infra/bootstrap
terraform init
terraform plan     # 内容を確認してから
terraform apply
```

出力されたバケット名を控える。

```sh
terraform output -raw tfstate_bucket
```

続いて `bootstrap/main.tf` の `backend "s3"` ブロックのコメントを外し、state を移行する。

```sh
terraform init -migrate-state \
  -backend-config="bucket=$(terraform output -raw tfstate_bucket)"
```

この手順は一度きりで、以後 `bootstrap` を触ることはほとんどない。

> `docs/02-infra.md` は当初ロック用の DynamoDB テーブルも作る設計だったが、
> **Terraform 1.10 以降は S3 のネイティブロック(`use_lockfile = true`)が使えるため不要**。
> `bootstrap` は S3 バケット1つだけになっている。

---

## Phase 2 — shared

GitHub Actions 用の OIDC プロバイダと CI ロールを作る。長期のアクセスキーは発行しない。

```sh
cd infra/shared
export TFSTATE_BUCKET=edgewatcher-tfstate-<アカウントID>

terraform init -backend-config="bucket=${TFSTATE_BUCKET}"
terraform plan -var="tfstate_bucket=${TFSTATE_BUCKET}"
terraform apply -var="tfstate_bucket=${TFSTATE_BUCKET}"
```

`github_owner` / `github_repo` は `terraform.tfvars` に置く。

作られるロール:

| ロール | 権限 | 使うワークフロー |
| --- | --- | --- |
| `edgewatcher-ci-plan` | 読み取り専用 + tfstate の読み取り | PR 時の `terraform plan` |
| `edgewatcher-ci-apply-dev` / `-prod` | 書き込み | `workflow_dispatch` の apply |
| `edgewatcher-ci-deploy-dev` / `-prod` | Lambda のコード更新、S3 同期、CloudFront invalidation | アプリコードのデプロイ |

PR で自動実行される plan には読み取りロールしか渡らないため、
**PR 経由で本番リソースが変更される経路が構造的に閉じる**。

---

## Phase 3 — envs/dev

```sh
cd infra/envs/dev
terraform init -backend-config="bucket=${TFSTATE_BUCKET}"
terraform plan
terraform apply
```

**必ず dev → prod の順で apply する**(`docs/00-overview.md` §8)。
`stg` を置かない代わりの運用ルール。

### plan で注意して見るところ

Cognito User Pool のようなステートフルなリソースが**置換(destroy/create)される差分が出ていないか**。
置換されると登録済みユーザーが消失する。インフラを手動 apply にしている最大の理由がこれ。
`cognito-web` モジュールには `prevent_destroy` を入れてあるが、plan の目視は省略しない。

---

## Phase 4 — 疎通確認

```sh
WEB=$(terraform output -raw web_url)
API=$(terraform output -raw api_url)
```

| # | 確認 | 期待 |
| --- | --- | --- |
| 1 | `open $WEB` | 疎通確認用ページが表示される |
| 2 | `curl -s -o /dev/null -w '%{http_code}\n' $WEB/no-such-path` | `200`(SPA フォールバック) |
| 3 | `curl -s -o /dev/null -w '%{http_code}\n' $API/devices` | `401`(JWT オーソライザが未認証を拒否) |
| 4 | `curl -s -o /dev/null -w '%{http_code}\n' $API/device/uploads -X POST` | `401`(Lambda オーソライザが拒否) |

**3 と 4 が通れば、オーソライザとルーティングの結線は正しい。**
これらは Lambda を呼ぶ前に判定されるため、ハンドラのコードがなくても検証できる。

> **Lambda 本体は 502 を返す。** 現時点で配置されているのはダミーの zip であり、
> Go のバイナリはまだない(`docs/02-infra.md` §5)。**502 は想定どおりの結果**で、
> API Gateway → Lambda の統合と invoke 権限が正しく張れていることの裏返しでもある。
> 実際の応答は `05-backend.md` に基づく Go の実装をデプロイしてから確認する。

---

## 日常の操作

```sh
terraform fmt -recursive        # 整形
terraform validate              # 静的検証
terraform plan                  # 差分確認(apply の前に必ず)
```

### 規約

- `required_version = "~> 1.15"`、AWS provider はメジャー固定。**`.terraform.lock.hcl` をコミットする**
- リソース名は `edgewatcher-<env>-<name>`。S3 はアカウント ID をサフィックスに付ける
- 共通タグは provider の `default_tags` で付ける
- **`terraform.tfvars` にシークレットを置かない。** Google の client secret は
  `TF_VAR_google_client_secret` 環境変数で渡す(`.gitignore` で `*.auto.tfvars` も除外済み)

### モジュール単体の検証について

`modules/domain` は `configuration_aliases`(us-east-1 の provider)を要求するため、
**単体での `terraform validate` はエラーになる**。これは仕様であり、
呼び出し側から provider を渡した状態でのみ検証できる。
`root_domain` が確定して `envs/dev` から呼び出されるようになった時点で検証される。
