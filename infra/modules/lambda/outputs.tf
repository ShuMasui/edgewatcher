output "function_name" {
  description = "関数名。CI の update-function-code の対象。"
  value       = aws_lambda_function.this.function_name
}

output "function_arn" {
  description = "関数 ARN。"
  value       = aws_lambda_function.this.arn
}

output "invoke_arn" {
  description = "API Gateway の統合に渡す invoke ARN。"
  value       = aws_lambda_function.this.invoke_arn
}

output "role_name" {
  description = "実行ロール名。"
  value       = aws_iam_role.this.name
}
