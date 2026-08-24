variable "region" {
  description = "リソースを作成するリージョン。観測デバイス・利用者ともに国内を想定(02-infra.md §1)。"
  type        = string
  default     = "ap-northeast-1"
}
