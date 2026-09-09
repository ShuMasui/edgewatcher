variable "name_prefix" {
  description = "リソース名のプレフィックス。"
  type        = string
}

variable "cognito_issuer" {
  description = "JWT オーソライザの issuer URL(cognito-web モジュールの出力)。"
  type        = string
}

variable "cognito_client_id" {
  description = "JWT の audience として検証する App Client ID。"
  type        = string
}

variable "authorizer_invoke_arn" {
  description = "Lambda オーソライザの invoke ARN。"
  type        = string
}

variable "authorizer_function_name" {
  description = "Lambda オーソライザの関数名。invoke 権限の付与に使う。"
  type        = string
}

variable "integrations" {
  description = "統合先の map。キーは関数の短い名前、値は invoke ARN。"
  type        = map(string)
}

variable "integration_function_names" {
  description = <<-EOT
    統合先の関数名の map。キーは integrations と同じ。
    invoke 権限の付与に使う。ARN ではなく関数名が必要なため別変数にしている。
  EOT
  type        = map(string)
}

variable "web_routes" {
  description = "JWT オーソライザを通す経路(\"GET /devices\" 形式)。"
  type        = list(string)
}

variable "device_public_routes" {
  description = "オーソライザを通さない経路。Lambda 内でコード/シークレットを検証する。"
  type        = list(string)
}

variable "device_routes" {
  description = "Lambda オーソライザを通す経路。"
  type        = list(string)
}

variable "cors_allow_origins" {
  description = "CORS で許可するオリジン。Web の配信元を入れる。"
  type        = list(string)
}

variable "api_domain" {
  description = "API のカスタムドメイン。null なら execute-api の既定 URL を使う(OPS-01)。"
  type        = string
  default     = null
}

variable "acm_certificate_arn" {
  description = "API Gateway 用の ACM 証明書 ARN。us-east-1 に必要(02-infra.md §3)。"
  type        = string
  default     = null
}

variable "throttling_burst_limit" {
  description = "スロットリングのバースト上限。暴走時の課金を抑える保険。"
  type        = number
  default     = 50
}

variable "throttling_rate_limit" {
  description = "スロットリングのレート上限(req/s)。"
  type        = number
  default     = 20
}

variable "log_retention_days" {
  description = "アクセスログの保持期間(日)。"
  type        = number
  default     = 30
}
