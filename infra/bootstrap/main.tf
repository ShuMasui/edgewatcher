terraform {
  required_version = "~> 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # bootstrap は自分自身が作るバケットに state を置く(鶏卵問題)。
  #
  # そのため **初回だけはローカル state で apply する**。バケットが存在しない
  # 状態でこのブロックが有効だと、terraform init が「無いバケット」を見に行って
  # 失敗し、Terraform にはそれを自分で作る手段がない。
  #
  # apply が通ったら下のブロックのコメントを外し、
  #   terraform init -migrate-state
  # で state を移す。バケット名は決め打ちでよい(下記)。詳細は infra/README.md。
  #
  # backend "s3" {
  #   bucket       = "edgewatcher-tfstate-606030504329"
  #   key          = "bootstrap/terraform.tfstate"
  #   region       = "us-east-1"
  #   encrypt      = true
  #   use_lockfile = true
  # }
}

provider "aws" {
  region = var.region
  profile = var.profile

  default_tags {
    tags = {
      Project     = "edgewatcher"
      Environment = "shared"
      ManagedBy   = "terraform"
      Stack       = "bootstrap"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  # S3 のバケット名はグローバルに一意である必要があるため、アカウント ID を付ける
  # (02-infra.md §9)。
  bucket_name = "edgewatcher-tfstate-${data.aws_caller_identity.current.account_id}-001"
}

resource "aws_s3_bucket" "tfstate" {
  bucket = local.bucket_name

  # state を保持するバケットは、誤った terraform destroy で消えてはならない。
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# state のバージョンは無期限に溜まるため、古い世代は自動で落とす。
resource "aws_s3_bucket_lifecycle_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  rule {
    id     = "expire-noncurrent-versions"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  depends_on = [aws_s3_bucket_versioning.tfstate]
}

# HTTPS 以外でのアクセスを拒否する。
resource "aws_s3_bucket_policy" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "DenyInsecureTransport"
      Effect    = "Deny"
      Principal = "*"
      Action    = "s3:*"
      Resource = [
        aws_s3_bucket.tfstate.arn,
        "${aws_s3_bucket.tfstate.arn}/*",
      ]
      Condition = {
        Bool = { "aws:SecureTransport" = "false" }
      }
    }]
  })

  depends_on = [aws_s3_bucket_public_access_block.tfstate]
}
