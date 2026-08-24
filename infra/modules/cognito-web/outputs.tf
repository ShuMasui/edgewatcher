output "user_pool_id" {
  description = "Web User Pool の ID。"
  value       = aws_cognito_user_pool.web.id
}

output "user_pool_client_id" {
  description = "App Client の ID。SPA が Hosted UI へリダイレクトする際に使う。"
  value       = aws_cognito_user_pool_client.web.id
}

output "issuer" {
  description = "JWT オーソライザに設定する issuer URL。"
  value       = "https://cognito-idp.${data.aws_region.current.region}.amazonaws.com/${aws_cognito_user_pool.web.id}"
}

output "hosted_ui_domain" {
  description = "Hosted UI のドメイン。ログイン・ログアウトのリダイレクト先。"
  value       = "https://${aws_cognito_user_pool_domain.web.domain}.auth.${data.aws_region.current.region}.amazoncognito.com"
}

output "google_idp_enabled" {
  description = "Google IdP が構成されているか。false の間は誰もログインできない。"
  value       = var.google_client_id != null
}
