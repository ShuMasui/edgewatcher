output "table_name" {
  description = "DynamoDB テーブル名。Lambda の環境変数 TABLE_NAME に渡す。"
  value       = aws_dynamodb_table.main.name
}

output "table_arn" {
  description = "テーブル ARN。IAM ポリシーのリソース指定に使う。"
  value       = aws_dynamodb_table.main.arn
}

output "index_arns" {
  description = "GSI の ARN。IAM ポリシーでインデックスを明示するために使う。"
  value = [
    "${aws_dynamodb_table.main.arn}/index/GSI1",
    "${aws_dynamodb_table.main.arn}/index/GSI2",
  ]
}
