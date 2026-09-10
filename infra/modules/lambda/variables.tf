variable "name_prefix" {
  description = "リソース名のプレフィックス。edgewatcher-<env> の形。"
  type        = string
}

variable "name" {
  description = "関数の短い名前(web-api / device-auth / device-api / authorizer)。"
  type        = string
}

variable "memory_size" {
  description = "割り当てメモリ(MB)。Lambda は CPU がこれに比例するため、実質 CPU の設定でもある。"
  type        = number
  default     = 256
}

variable "timeout" {
  description = "タイムアウト(秒)。"
  type        = number
  default     = 10
}

variable "environment" {
  description = "環境変数。シークレットは渡さない(05-backend.md §3.6)。"
  type        = map(string)
  default     = {}
}

variable "policy_json" {
  description = <<-EOT
    この関数に固有の IAM ポリシー(JSON)。必須。
    optional にして count で分岐すると、data source 由来の未確定値に
    count が依存してしまい plan が通らなくなる。
  EOT
  type        = string
}

variable "log_retention_days" {
  description = "CloudWatch Logs の保持期間(日)。"
  type        = number
  default     = 30
}
