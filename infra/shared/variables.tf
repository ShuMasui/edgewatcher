variable "region" {
  description = "リージョン。"
  type        = string
  default     = "us-east-1"
}

variable "profile" {
  description = "AWS profile名です"
  type        = string
  default     = "edgewatcher"
}

variable "github_owner" {
  description = "GitHub のオーナー名。OIDC の sub 条件に使う。"
  type        = string
}

variable "github_repo" {
  description = "GitHub のリポジトリ名。OIDC の sub 条件に使う。"
  type        = string
}

# GitHub の既定の sub はオーナー名・リポジトリ名だけでなく、それぞれの
# 数値 ID も含む(main.tf の repo_subject_prefixes を参照)。値は
#   gh api users/<owner> --jq .id
#   gh api repos/<owner>/<repo> --jq .id
# で取れる。名前は変えられるが ID は変わらず再利用もされない。
variable "github_owner_id" {
  description = "GitHub オーナーの数値 ID。OIDC の sub 条件に使う。"
  type        = string
}

variable "github_repo_id" {
  description = "GitHub リポジトリの数値 ID。OIDC の sub 条件に使う。"
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
