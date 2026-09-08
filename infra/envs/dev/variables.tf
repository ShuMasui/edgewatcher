variable "env" {
  description = "環境名。リソース名のプレフィックスに入る(02-infra.md §9)。"
  type        = string
  default     = "dev"
}

variable "region" {
  description = "リージョン。"
  type        = string
  default     = "ap-northeast-1"
}

variable "root_domain" {
  description = <<-EOT
    ルートドメイン名。未確定の間は null(01-openquestion.md OPS-01)。
    null の場合、CloudFront と API Gateway の既定ドメインで動作する。
  EOT
  type        = string
  default     = null
}

variable "retention_days" {
  description = <<-EOT
    画像・観測レコードの保持期間(日)。dev = 1 / prod = 7。
    S3 のライフサイクルと DynamoDB の TTL の両方に同じ値が渡る(02-infra.md §4)。
  EOT
  type        = number
  default     = 1
}

variable "device_limit" {
  description = "1オーナーが登録できる端末数の上限。GET /app-config で Web に返す(03-web.md §1.9)。"
  type        = number
  default     = 10
}

variable "signed_url_ttl" {
  description = "署名付き URL の有効期限(秒)。既定15分(03-web.md §2.1)。"
  type        = number
  default     = 900
}

variable "point_in_time_recovery" {
  description = "DynamoDB の PITR を有効化するか(01-openquestion.md DATA-04)。"
  type        = bool
  default     = false
}

variable "google_client_id" {
  description = "Google OAuth クライアント ID。未取得の間は null(AUTH-07)。"
  type        = string
  default     = null
}

variable "google_client_secret" {
  description = <<-EOT
    Google OAuth クライアントシークレット。

    **tfvars には書かないこと。** 値は Secrets Manager の
    `edgewatcher-<env>-google-oauth-client-secret` に置き、apply 時に
    TF_VAR_google_client_secret として注入する(secrets.tf、infra/README.md)。
  EOT
  type        = string
  default     = null
  sensitive   = true
}

variable "google_idp_enabled" {
  description = <<-EOT
    Cognito に Google IdP を作るか(AUTH-07)。

    **2段階 apply のためのスイッチ。** 1回目は false のまま apply して
    Secrets Manager の入れ物を作り、値を投入してから true にして 2回目を
    apply する。シークレットが存在しない状態でも 1回目の apply が必ず成功
    するように、依存を暗黙(client_id が null かどうか)ではなく明示にしている。

    true にするには client_id と client_secret の両方が必要で、
    片方だけだと apply が precondition で止まる — 値の注入を忘れたまま
    有効化すると、空のシークレットを持つ IdP が出来上がり、
    「ログインボタンはあるが必ず失敗する」という気づきにくい壊れ方をするため。
  EOT
  type        = bool
  default     = false
}

variable "upload_placeholder" {
  description = "疎通確認用の index.html を配置するか。"
  type        = bool
  default     = true
}
