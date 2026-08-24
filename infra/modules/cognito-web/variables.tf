variable "name_prefix" {
  description = "リソース名のプレフィックス。"
  type        = string
}

variable "account_id" {
  description = "アカウント ID。Hosted UI ドメインのグローバル一意名の衝突回避に使う。"
  type        = string
}

variable "google_client_id" {
  description = <<-EOT
    Google OAuth クライアント ID。未取得の間は null にしておく
    (01-openquestion.md AUTH-07)。null の場合 Google IdP を作らず、
    User Pool と Hosted UI だけが立つ。
  EOT
  type        = string
  default     = null
}

variable "google_client_secret" {
  description = "Google OAuth クライアントシークレット。google_client_id とセットで指定する。"
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
