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
| `google_idp_enabled` | Google IdP を作らない。User Pool と Hosted UI だけが立つ(この間は誰もログインできない) | AUTH-07 |

**確定したら値を埋めて apply するだけでよい。**コードの書き換えは不要。

Google の client secret だけは値の置き場所が違う。tfvars にも TF_VAR の直書きにもせず、
Secrets Manager に置いて apply 時に注入する(下の「Google IdP を有効にする」)。

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

## Google IdP を有効にする (AUTH-07)

Google の client secret は **Terraform が値を管理しない**。`envs/dev/secrets.tf` が
作るのは Secrets Manager の**入れ物だけ**で、`aws_secretsmanager_secret_version` は
あえて置いていない。値を Terraform に持たせると、tfvars か TF_VAR か state の
いずれかに平文で現れることになり、ASM に置いた意味がなくなるためである。

そのため有効化は **2段階の apply** になる。1回目の apply はシークレットが
まだ存在しない状態で走るので、`google_idp_enabled` を明示のフラグにして
「値が揃っているか」と「有効化したいか」を分けてある。

### 1. 入れ物を作る

`terraform.tfvars` は `google_idp_enabled = false`(既定)のまま apply する。
`edgewatcher-dev-google-oauth-client-secret` が空のまま作られる。

### 2. 値を投入する(人間が一度だけ)

Google Cloud Console で OAuth クライアントを作り、シークレットを流し込む。
**この操作だけは AWS の資格情報を持つ人間が行う。**

```sh
aws secretsmanager put-secret-value \
  --secret-id edgewatcher-dev-google-oauth-client-secret \
  --secret-string '<Google が発行した client secret>'
```

ローテーションも同じコマンドで、以後 apply し直すだけで反映される。

### 3. 有効化する

`terraform.tfvars` に client ID(こちらは秘密ではない)を書き、フラグを立てる。

```hcl
google_client_id   = "xxxxx.apps.googleusercontent.com"
google_idp_enabled = true
```

apply の方法は2つある。

**GitHub Actions から**(手元に AWS の資格情報がなくてもよい)

`main` ブランチで `infra-apply` ワークフローを `workflow_dispatch` する。
ワークフローが Secrets Manager から値を読み、`TF_VAR_google_client_secret` として
Terraform に渡す。`plan` で差分を確認してから `apply` を選ぶ。
リポジトリ変数 `AWS_ACCOUNT_ID` が必要(ARN の組み立てに使うだけで、秘密ではない)。

`edgewatcher-ci-apply-<env>` ロールの信頼ポリシーは sub を `refs/heads/main` に
限っているため、他のブランチから dispatch しても AssumeRole の段階で拒否される。

**手元から**

```sh
export TF_VAR_google_client_secret=$(aws secretsmanager get-secret-value \
  --secret-id edgewatcher-dev-google-oauth-client-secret \
  --query SecretString --output text)
terraform -chdir=infra/envs/dev apply
```

値を入れ忘れたまま `google_idp_enabled = true` にした場合は、`cognito-web` の
precondition が apply を止める。空のシークレットでも IdP の作成自体は AWS 側で
成功してしまい、「ログインボタンは出るが必ず失敗する」という、apply のログにも
CloudWatch にも現れない壊れ方になるため。

### state に残ることについて

Cognito の IdP リソースが client secret を属性として持つため、**値は tfstate に入る**。
これは Terraform で Cognito を管理する以上避けられない。tfstate バケットは
バージョニング + SSE + パブリックアクセス全面ブロックで、読めるのは
`edgewatcher-ci-*` ロールと開発者の IAM ユーザーだけである(Phase 1)。
ASM に置く意味は「値がリポジトリと開発者の手元に存在しないこと」にある。

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
  Secrets Manager に置き、apply 時に `TF_VAR_google_client_secret` として注入する
  (上の「Google IdP を有効にする」。`.gitignore` で `*.auto.tfvars` も除外済み)

### モジュール単体の検証について

`modules/domain` は `configuration_aliases`(us-east-1 の provider)を要求するため、
**単体での `terraform validate` はエラーになる**。これは仕様であり、
呼び出し側から provider を渡した状態でのみ検証できる。
`root_domain` が確定して `envs/dev` から呼び出されるようになった時点で検証される。
