output "web_url" {
  description = "Web の入口 URL。ブラウザで開いて疎通確認する。"
  value       = module.web_hosting.url
}

output "api_url" {
  description = "API の入口 URL。"
  value       = module.api.url
}

output "cloudfront_distribution_id" {
  description = "CI の invalidation に使う。"
  value       = module.web_hosting.distribution_id
}

output "web_bucket" {
  description = "フロントエンドの成果物を同期する先。"
  value       = module.web_hosting.bucket_name
}

output "images_bucket" {
  description = "観測画像のバケット。"
  value       = module.storage_images.bucket_name
}

output "table_name" {
  description = "DynamoDB のテーブル名。"
  value       = module.data.table_name
}

# Web のビルドに渡す値(web/.env)。Hosted UI へのリダイレクトに使う。
output "cognito_client_id" {
  description = "Cognito App Client ID。Web の VITE_COGNITO_CLIENT_ID に入れる。"
  value       = module.cognito_web.user_pool_client_id
}

output "cognito_hosted_ui" {
  description = "Hosted UI のドメイン。"
  value       = module.cognito_web.hosted_ui_domain
}

output "google_idp_enabled" {
  description = "Google IdP が構成されているか。false の間はログインできない(AUTH-07)。"
  value       = module.cognito_web.google_idp_enabled
}

output "lambda_function_names" {
  description = "CI の update-function-code の対象。"
  value = {
    authorizer  = module.lambda_authorizer.function_name
    web_api     = module.lambda_web_api.function_name
    device_auth = module.lambda_device_auth.function_name
    device_api  = module.lambda_device_api.function_name
  }
}
