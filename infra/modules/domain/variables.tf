variable "zone_id" {
  description = "Route53 ホストゾーン ID。shared スタックが持つゾーンを data で引いて渡す。"
  type        = string
}

variable "web_domain" {
  description = "Web のドメイン(例: app.dev.example.com)。"
  type        = string
}

variable "api_domain" {
  description = "API のドメイン(例: api.dev.example.com)。"
  type        = string
}

variable "cloudfront_domain_name" {
  description = "CloudFront ディストリビューションのドメイン名。A レコードの alias 先。"
  type        = string
}

variable "cloudfront_hosted_zone_id" {
  description = "CloudFront のホストゾーン ID(固定値 Z2FDTNDATAQYW2)。"
  type        = string
  default     = "Z2FDTNDATAQYW2"
}

variable "api_gateway_domain_name" {
  description = "API Gateway カスタムドメインのターゲットドメイン名。"
  type        = string
}

variable "api_gateway_hosted_zone_id" {
  description = "API Gateway カスタムドメインのホストゾーン ID。"
  type        = string
}
