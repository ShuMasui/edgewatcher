variable "name_prefix" {
  description = "リソース名のプレフィックス。"
  type        = string
}

variable "account_id" {
  description = "アカウント ID。S3 のグローバル一意名の衝突回避に使う。"
  type        = string
}

variable "web_domain" {
  description = <<-EOT
    Web のカスタムドメイン(例: app.dev.example.com)。
    null の場合は CloudFront の既定ドメイン(*.cloudfront.net)で配信する
    (01-openquestion.md OPS-01)。
  EOT
  type        = string
  default     = null
}

variable "acm_certificate_arn" {
  description = <<-EOT
    CloudFront 用の ACM 証明書 ARN。us-east-1 に存在する必要がある
    (02-infra.md §3)。web_domain とセットで指定する。
  EOT
  type        = string
  default     = null
}

variable "upload_placeholder" {
  description = "疎通確認用の index.html を配置するか。フロントエンドの初回デプロイ後は false でよい。"
  type        = bool
  default     = false
}

variable "placeholder_html" {
  description = "疎通確認用 index.html の中身。"
  type        = string
  default     = "<!doctype html><meta charset=\"utf-8\"><title>EdgeWatcher</title><h1>EdgeWatcher</h1>"
}
