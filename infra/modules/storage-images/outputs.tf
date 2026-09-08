output "bucket_name" {
  description = "画像バケット名。Lambda の環境変数 IMAGES_BUCKET に渡す。"
  value       = aws_s3_bucket.images.bucket
}

output "bucket_arn" {
  description = "画像バケットの ARN。IAM ポリシーのリソース指定に使う。"
  value       = aws_s3_bucket.images.arn
}
