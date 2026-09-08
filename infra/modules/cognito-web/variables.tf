variable "name_prefix" {
  description = "リソース名のプレフィックス。"
  type        = string
}

variable "account_id" {
  description = "アカウント ID。Hosted UI ドメインのグローバル一意名の衝突回避に使う。"
  type        = string
}

variable "google_idp_enabled" {
  description = <<-EOT
    Google IdP を作るか(01-openquestion.md AUTH-07)。

    false の場合 User Pool と Hosted UI だけが立ち、誰もログインできない。
    呼び出し側の 2段階 apply のスイッチであり、client_id が null かどうかで
    暗黙に決めるのではなく明示のフラグにしてある — シークレットは
    Secrets Manager から apply 時に注入されるため、「値が揃っているか」と
    「有効化したいか」は別の事実だからである。
  EOT
  type        = bool
  default     = false
}

variable "google_client_id" {
  description = <<-EOT
    Google OAuth クライアント ID。未取得の間は null にしておく
    (01-openquestion.md AUTH-07)。google_idp_enabled が true のとき必須。
  EOT
  type        = string
  default     = null
}

variable "google_client_secret" {
  description = <<-EOT
    Google OAuth クライアントシークレット。google_idp_enabled が true のとき必須。
    値は tfvars ではなく Secrets Manager から TF_VAR で渡す
    (envs/dev/secrets.tf)。
  EOT
  type        = string
  default     = null
  sensitive   = true
}

variable "callback_urls" {
  description = "OAuth のコールバック URL。カスタムドメイン確定前は CloudFront の既定ドメインを入れる。"
  type        = list(string)
}

variable "logout_urls" {
  description = "ログアウト後のリダイレクト先(03-web.md §1.4.2)。"
  type        = list(string)
}
