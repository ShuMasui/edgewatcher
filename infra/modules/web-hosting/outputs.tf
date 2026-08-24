output "bucket_name" {
  description = "配信バケット名。CI がフロントエンドの成果物を同期する先。"
  value       = aws_s3_bucket.web.bucket
}

output "bucket_arn" {
  description = "配信バケットの ARN。"
  value       = aws_s3_bucket.web.arn
}

output "distribution_id" {
  description = "CloudFront ディストリビューション ID。CI の invalidation に使う。"
  value       = aws_cloudfront_distribution.web.id
}

output "distribution_domain_name" {
  description = "CloudFront の既定ドメイン。カスタムドメイン確定までの接続先。"
  value       = aws_cloudfront_distribution.web.domain_name
}

output "url" {
  description = "Web の入口 URL。"
  value       = var.web_domain == null ? "https://${aws_cloudfront_distribution.web.domain_name}" : "https://${var.web_domain}"
}
