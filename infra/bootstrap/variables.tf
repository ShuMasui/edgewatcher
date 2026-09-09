variable "region" {
  description = "リソースを作成するリージョン。観測デバイス・利用者ともに国内を想定(02-infra.md §1)。"
  type        = string
  default     = "us-east-1"
}

variable "profile" {
  description = "AWS profile名です"
  type        = string
  default     = "edgewatcher"
}
