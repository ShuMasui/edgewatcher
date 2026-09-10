# ACM 証明書は環境ごとに2枚必要(02-infra.md §3)。
#
#   CloudFront 用          : us-east-1 に存在しなければならない(CloudFront の仕様)
#   API Gateway 用         : API と同じ us-east-1 に必要
#
# リージョン制約をこのモジュールの内側に閉じ込め、
# 呼び出し側のコードに us-east-1 が漏れないようにする。

terraform {
  required_providers {
    aws = {
      source                = "hashicorp/aws"
      configuration_aliases = [aws.us_east_1]
    }
  }
}

# --- CloudFront 用(us-east-1)---------------------------------------------

resource "aws_acm_certificate" "web" {
  provider = aws.us_east_1

  domain_name       = var.web_domain
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "web_validation" {
  for_each = {
    for dvo in aws_acm_certificate.web.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id         = var.zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 60
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "web" {
  provider = aws.us_east_1

  certificate_arn         = aws_acm_certificate.web.arn
  validation_record_fqdns = [for r in aws_route53_record.web_validation : r.fqdn]
}

# --- API Gateway 用(us-east-1)---------------------------------------

resource "aws_acm_certificate" "api" {
  domain_name       = var.api_domain
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "api_validation" {
  for_each = {
    for dvo in aws_acm_certificate.api.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  zone_id         = var.zone_id
  name            = each.value.name
  type            = each.value.type
  records         = [each.value.record]
  ttl             = 60
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "api" {
  certificate_arn         = aws_acm_certificate.api.arn
  validation_record_fqdns = [for r in aws_route53_record.api_validation : r.fqdn]
}

# --- A/AAAA レコード --------------------------------------------------------
# レコード名に環境が含まれるため、env 同士がレコードを奪い合うことはない
# (02-infra.md §3)。

resource "aws_route53_record" "web" {
  for_each = toset(["A", "AAAA"])

  zone_id = var.zone_id
  name    = var.web_domain
  type    = each.value

  alias {
    name                   = var.cloudfront_domain_name
    zone_id                = var.cloudfront_hosted_zone_id
    evaluate_target_health = false
  }
}

resource "aws_route53_record" "api" {
  for_each = toset(["A", "AAAA"])

  zone_id = var.zone_id
  name    = var.api_domain
  type    = each.value

  alias {
    name                   = var.api_gateway_domain_name
    zone_id                = var.api_gateway_hosted_zone_id
    evaluate_target_health = false
  }
}
