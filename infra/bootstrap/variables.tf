variable "region" {
  description = "リソースを作成するリージョン。観測デバイス・利用者ともに国内を想定(02-infra.md §1)。"
  type        = string
  default     = "us-east-1"
}

variable "profile" {
  description = <<-EOT
    apply に使う AWS profile 名。

    **既定は null（＝指定しない）。**profile を明示すると、AWS プロバイダは
    環境変数の資格情報を見に行かなくなる。GitHub Actions のランナーには
    ~/.aws が無く、OIDC で渡ってくるのは環境変数なので、ここに名前が
    入っていると infra-apply が provider の初期化で必ず失敗する。

    null にしておけば通常の資格情報チェーンで解決される。手元では

      export AWS_PROFILE=edgewatcher

    で与える。環境変数にするのは backend のためでもある。**backend ブロックは
    provider とは別系統で資格情報を解決し、この変数は届かない。**profile を
    provider にだけ書くと、プロバイダと state で別の資格情報を使うことになる。
  EOT
  type        = string
  default     = null
}
