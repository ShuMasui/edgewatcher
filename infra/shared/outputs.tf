output "github_oidc_provider_arn" {
  description = "GitHub Actions 用 OIDC プロバイダの ARN。"
  value       = aws_iam_openid_connect_provider.github.arn
}

output "ci_role_arns" {
  description = "GitHub Actions のワークフローに設定するロール ARN。"
  value       = { for name, role in aws_iam_role.ci : name => role.arn }
}

output "route53_zone_id" {
  description = "ルートドメインのホストゾーン ID。root_domain が null のときは null。"
  value       = var.root_domain == null ? null : aws_route53_zone.root[0].zone_id
}

output "route53_name_servers" {
  description = "レジストラに設定する NS レコード。root_domain が null のときは null。"
  value       = var.root_domain == null ? null : aws_route53_zone.root[0].name_servers
}
