# 観測画像を置くバケット。パブリックアクセスは全遮断し、
# Web からの参照は署名付き URL のみ(02-infra.md §7)。
#
# キー設計(05-backend.md §1.4):
#   observations/<deviceId>/<YYYY-MM-DD>/<observationId>.jpg
#   observations/<deviceId>/<YYYY-MM-DD>/<observationId>_thumb.jpg

resource "aws_s3_bucket" "images" {
  bucket = "${var.name_prefix}-images-${var.account_id}"

  tags = {
    Name = "${var.name_prefix}-images"
  }
}

resource "aws_s3_bucket_public_access_block" "images" {
  bucket = aws_s3_bucket.images.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "images" {
  bucket = aws_s3_bucket.images.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# バージョニングは無効。上書きが発生しない書き込み専用の用途であり、
# コストに見合わない(02-infra.md §7)。
resource "aws_s3_bucket_versioning" "images" {
  bucket = aws_s3_bucket.images.id

  versioning_configuration {
    status = "Suspended"
  }
}

# 保持期間後に自動削除する。DynamoDB 側の TTL と同じ値を使い、
# 「画像はないのにレコードだけ残る」状態を避ける(02-infra.md §7)。
resource "aws_s3_bucket_lifecycle_configuration" "images" {
  bucket = aws_s3_bucket.images.id

  rule {
    id     = "expire-observations"
    status = "Enabled"

    filter {
      prefix = "observations/"
    }

    expiration {
      days = var.retention_days
    }
  }

  # 中断したマルチパートアップロードが課金対象として残り続けるのを防ぐ。
  rule {
    id     = "abort-incomplete-multipart"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

resource "aws_s3_bucket_policy" "images" {
  bucket = aws_s3_bucket.images.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource = [
        aws_s3_bucket.images.arn,
        "${aws_s3_bucket.images.arn}/*",
      ]
      Condition = {
        Bool = { "aws:SecureTransport" = "false" }
      }
    }]
  })

  depends_on = [aws_s3_bucket_public_access_block.images]
}
