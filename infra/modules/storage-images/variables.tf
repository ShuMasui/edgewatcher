variable "name_prefix" {
  description = "リソース名のプレフィックス。"
  type        = string
}

variable "account_id" {
  description = "アカウント ID。S3 のグローバル一意名の衝突回避に使う(02-infra.md §9)。"
  type        = string
}

variable "retention_days" {
  description = <<-EOT
    画像の保持期間(日)。dev = 1 / prod = 7(01-openquestion.md APP-03)。
    DynamoDB の TTL と同じ値を使うこと。ずれると
    「画像はないのにレコードだけ残る」状態が生じる。
  EOT
  type        = number
}
