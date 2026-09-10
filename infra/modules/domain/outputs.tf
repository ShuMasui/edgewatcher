output "web_certificate_arn" {
  description = "CloudFront 用 ACM 証明書の ARN(us-east-1)。"
  value       = aws_acm_certificate_validation.web.certificate_arn
}

output "api_certificate_arn" {
  description = "API Gateway 用 ACM 証明書の ARN(us-east-1)。"
  value       = aws_acm_certificate_validation.api.certificate_arn
}
