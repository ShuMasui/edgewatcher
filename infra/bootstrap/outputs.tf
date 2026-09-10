output "tfstate_bucket" {
  description = <<-EOT
    tfstate を置く S3 バケット名。

    この値は各スタックの backend ブロックにリテラルで書いてある
    (infra/shared/main.tf、infra/envs/dev/main.tf)。作られた名前が
    そこと一致していることを確認するために出力している。
  EOT
  value       = aws_s3_bucket.tfstate.bucket
}

output "account_id" {
  description = "このアカウントの ID。"
  value       = data.aws_caller_identity.current.account_id
}
