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
  description = "Google OAuth クライアントシークレット。"
  type        = string
  default     = null
  sensitive   = true
}

variable "upload_placeholder" {
  description = "疎通確認用の index.html を配置するか。"
  type        = bool
  default     = true
}
