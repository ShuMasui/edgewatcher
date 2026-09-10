# SPA の静的配信。S3 の静的ウェブサイトホスティングは使わず、
# CloudFront から OAC で参照する。バケットを一切公開せずに済み、
# HTTPS を CloudFront 側で終端できる(02-infra.md §7)。

resource "aws_s3_bucket" "web" {
  bucket = "${var.name_prefix}-web-${var.account_id}"

  tags = {
    Name = "${var.name_prefix}-web"
  }
}

resource "aws_s3_bucket_public_access_block" "web" {
  bucket = aws_s3_bucket.web.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "web" {
  bucket = aws_s3_bucket.web.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_cloudfront_origin_access_control" "web" {
  name                              = "${var.name_prefix}-web-oac"
  description                       = "OAC for EdgeWatcher web bucket"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_distribution" "web" {
  enabled             = true
  is_ipv6_enabled     = true
  default_root_object = "index.html"
  comment             = "EdgeWatcher ${var.name_prefix} web"

  # root_domain が未確定の間は空になり、*.cloudfront.net でアクセスする
  # (01-openquestion.md OPS-01)。
  aliases = var.web_domain == null ? [] : [var.web_domain]

  # 日本国内からの利用を想定しているため、最も安価な範囲に絞る。
  price_class = "PriceClass_200"

  origin {
    origin_id                = "s3-web"
    domain_name              = aws_s3_bucket.web.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.web.id
  }

  default_cache_behavior {
    target_origin_id       = "s3-web"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true

    # AWS マネージドポリシー CachingOptimized。
    cache_policy_id = "658327ea-f89d-4fab-a63d-7e88639e58f6"
  }

  # SPA のため、403 / 404 を /index.html に 200 で書き換える
  # (02-infra.md §7)。React Router がパスを解決する。
  custom_error_response {
    error_code            = 403
    response_code         = 200
    response_page_path    = "/index.html"
    error_caching_min_ttl = 0
  }

  custom_error_response {
    error_code            = 404
    response_code         = 200
    response_page_path    = "/index.html"
    error_caching_min_ttl = 0
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    # カスタムドメインが未設定の間は CloudFront の既定証明書を使う。
    cloudfront_default_certificate = var.acm_certificate_arn == null
    acm_certificate_arn            = var.acm_certificate_arn
    ssl_support_method             = var.acm_certificate_arn == null ? null : "sni-only"
    minimum_protocol_version       = var.acm_certificate_arn == null ? null : "TLSv1.2_2021"
  }

  tags = {
    Name = "${var.name_prefix}-web"
  }
}

# CloudFront ディストリビューションからのみ読める。
resource "aws_s3_bucket_policy" "web" {
  bucket = aws_s3_bucket.web.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "AllowCloudFrontServicePrincipal"
        Effect    = "Allow"
        Principal = { Service = "cloudfront.amazonaws.com" }
        Action    = "s3:GetObject"
        Resource  = "${aws_s3_bucket.web.arn}/*"
        Condition = {
          StringEquals = {
            "AWS:SourceArn" = aws_cloudfront_distribution.web.arn
          }
        }
      },
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource = [
          aws_s3_bucket.web.arn,
          "${aws_s3_bucket.web.arn}/*",
        ]
        Condition = {
          Bool = { "aws:SecureTransport" = "false" }
        }
      },
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.web]
}

# ---------------------------------------------------------------------------
# 疎通確認用のプレースホルダ
#
# 実物の Web アプリ(React/Vite のビルド成果物)は CI が S3 に同期する。
# ここで置くのは、インフラが組み上がったことを確認するための最小の HTML で、
# 最初のフロントエンドデプロイで上書きされる前提のもの。
# ---------------------------------------------------------------------------
resource "aws_s3_object" "placeholder" {
  count = var.upload_placeholder ? 1 : 0

  bucket       = aws_s3_bucket.web.id
  key          = "index.html"
  content      = var.placeholder_html
  content_type = "text/html; charset=utf-8"

  # CI がフロントエンドをデプロイした後に terraform が差分を出さないようにする。
  lifecycle {
    ignore_changes = [content, etag]
  }
}
