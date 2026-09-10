variable "name_prefix" {
  description = "リソース名のプレフィックス。edgewatcher-<env> の形(02-infra.md §9)。"
  type        = string
}

variable "point_in_time_recovery" {
  description = <<-EOT
    PITR を有効化するか。有効にすると ExportTableToPointInTimeAction による
    S3 エクスポートが使えるが、ストレージ量に応じた課金が発生する。
    方針は 01-openquestion.md DATA-04 で検討中のため、既定は無効。
  EOT
  type        = bool
  default     = false
}
