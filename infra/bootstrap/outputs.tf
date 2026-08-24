output "tfstate_bucket" {
  description = "tfstate を置く S3 バケット名。各スタックの terraform init に -backend-config で渡す。"
  value       = aws_s3_bucket.tfstate.bucket
}

output "account_id" {
  description = "このアカウントの ID。"
  value       = data.aws_caller_identity.current.account_id
}

output "init_command" {
  description = "他のスタックを初期化するためのコマンド。"
  value       = "terraform init -backend-config=\"bucket=${aws_s3_bucket.tfstate.bucket}\""
}
