output "api_id" {
  description = "HTTP API の ID。"
  value       = aws_apigatewayv2_api.main.id
}

output "execution_arn" {
  description = "Lambda の invoke 権限に使う execution ARN。"
  value       = aws_apigatewayv2_api.main.execution_arn
}

output "default_endpoint" {
  description = "execute-api の既定エンドポイント。カスタムドメイン確定までの接続先。"
  value       = aws_apigatewayv2_api.main.api_endpoint
}

output "url" {
  description = "API の入口 URL。"
  value       = var.api_domain == null ? aws_apigatewayv2_api.main.api_endpoint : "https://${var.api_domain}"
}
