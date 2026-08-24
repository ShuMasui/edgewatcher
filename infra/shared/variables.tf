variable "region" {
  description = "リージョン。"
  type        = string
  default     = "ap-northeast-1"
}

variable "tfstate_bucket" {
  description = "bootstrap が作った tfstate バケット名。ci-plan ロールの読み取り許可に使う。"
  type        = string
}

variable "github_owner" {
  description = "GitHub のオーナー名。OIDC の sub 条件に使う。"
  type        = string
}

variable "github_repo" {
  description = "GitHub のリポジトリ名。OIDC の sub 条件に使う。"
  type        = string
}

variable "root_domain" {
  description = <<-EOT
    ルートドメイン名。未確定の間は null にしておく(01-openquestion.md OPS-01)。
    null の場合 Route53 ホストゾーンを作らず、各環境は CloudFront と
    API Gateway の既定ドメインで動作する。確定したら値を入れて apply するだけでよい。
  EOT
  type        = string
  default     = null
}
