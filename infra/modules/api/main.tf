# HTTP API を1つ作り、経路によってオーソライザを使い分ける(02-infra.md §6)。
#
#   /app-config, /devices/*, /observations/*  → JWT オーソライザ(Cognito)
#   /device/pair, /device/token               → なし(認証前)
#   /device/uploads, /device/logout           → Lambda オーソライザ

resource "aws_apigatewayv2_api" "main" {
  name          = "${var.name_prefix}-api"
  protocol_type = "HTTP"
  description   = "EdgeWatcher ${var.name_prefix} API"

  cors_configuration {
    allow_origins = var.cors_allow_origins
    allow_methods = ["GET", "POST", "PATCH", "DELETE", "OPTIONS"]
    allow_headers = ["authorization", "content-type"]
    max_age       = 300
  }

  # 端末は本画像とサムネイルを multipart で送る(05-backend.md §1.3)。
  # 実質の上限は Lambda 側の 6MB(base64 で約1.33倍に膨らむため約4.5MB)。
  body = null
}

resource "aws_cloudwatch_log_group" "access" {
  name              = "/aws/apigateway/${var.name_prefix}-api"
  retention_in_days = var.log_retention_days
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.main.id
  name        = "$default"
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.access.arn

    # 構造化ログ。資格情報や署名付き URL は出さない(05-backend.md §2.5)。
    format = jsonencode({
      requestId      = "$context.requestId"
      httpMethod     = "$context.httpMethod"
      path           = "$context.path"
      status         = "$context.status"
      responseLength = "$context.responseLength"
      responseTime   = "$context.responseLatency"
      sourceIp       = "$context.identity.sourceIp"
      errorMessage   = "$context.error.message"
      authorizerErr  = "$context.authorizer.error"
    })
  }

  default_route_settings {
    detailed_metrics_enabled = true
    throttling_burst_limit   = var.throttling_burst_limit
    throttling_rate_limit    = var.throttling_rate_limit
  }
}

# ---------------------------------------------------------------------------
# オーソライザ
# ---------------------------------------------------------------------------

# Web 側は Cognito 組み込みの JWT オーソライザを使う。Lambda の呼び出しも
# コールドスタートも発生しないため、自前で検証する理由がない(02-infra.md §6)。
resource "aws_apigatewayv2_authorizer" "jwt" {
  api_id           = aws_apigatewayv2_api.main.id
  authorizer_type  = "JWT"
  identity_sources = ["$request.header.Authorization"]
  name             = "cognito-jwt"

  jwt_configuration {
    audience = [var.cognito_client_id]
    issuer   = var.cognito_issuer
  }
}

# 端末側は DynamoDB を引いて検証する必要があるため Lambda オーソライザ。
resource "aws_apigatewayv2_authorizer" "device" {
  api_id                            = aws_apigatewayv2_api.main.id
  authorizer_type                   = "REQUEST"
  authorizer_uri                    = var.authorizer_invoke_arn
  identity_sources                  = ["$request.header.Authorization"]
  name                              = "device-session"
  authorizer_payload_format_version = "2.0"

  # simple response 形式({ isAuthorized, context })を使う。
  enable_simple_responses = true

  # レスポンスキャッシュは無効(TTL = 0)。
  # 「Web から端末を削除した瞬間に、有効期限が残っているトークンでも拒否される」
  # という即時失効が、毎リクエストの GetItem に支えられている(06-auth.md §4)。
  # キャッシュを効かせるとその性質が壊れる。
  authorizer_result_ttl_in_seconds = 0
}

# ---------------------------------------------------------------------------
# 統合とルート
# ---------------------------------------------------------------------------

locals {
  # 経路 → どの関数に流すか。オーソライザの種類ごとにまとめる。
  routes = merge(
    { for r in var.web_routes : r => { target = "web-api", auth = "jwt" } },
    { for r in var.device_public_routes : r => { target = "device-auth", auth = "none" } },
    { for r in var.device_routes : r => { target = "device-api", auth = "device" } },
  )
}

resource "aws_apigatewayv2_integration" "lambda" {
  for_each = var.integrations

  api_id                 = aws_apigatewayv2_api.main.id
  integration_type       = "AWS_PROXY"
  integration_uri        = each.value
  payload_format_version = "2.0"
  timeout_milliseconds   = 29000
}

resource "aws_apigatewayv2_route" "this" {
  for_each = local.routes

  api_id    = aws_apigatewayv2_api.main.id
  route_key = each.key
  target    = "integrations/${aws_apigatewayv2_integration.lambda[each.value.target].id}"

  authorization_type = each.value.auth == "jwt" ? "JWT" : (each.value.auth == "device" ? "CUSTOM" : "NONE")
  authorizer_id = (
    each.value.auth == "jwt" ? aws_apigatewayv2_authorizer.jwt.id :
    each.value.auth == "device" ? aws_apigatewayv2_authorizer.device.id :
    null
  )
}

# ---------------------------------------------------------------------------
# カスタムドメイン(root_domain が確定するまでは作らない)
# ---------------------------------------------------------------------------
resource "aws_apigatewayv2_domain_name" "this" {
  count = var.api_domain == null ? 0 : 1

  domain_name = var.api_domain

  domain_name_configuration {
    # API と同じ us-east-1 の証明書が必要(02-infra.md §3)。
    certificate_arn = var.acm_certificate_arn
    endpoint_type   = "REGIONAL"
    security_policy = "TLS_1_2"
  }
}

resource "aws_apigatewayv2_api_mapping" "this" {
  count = var.api_domain == null ? 0 : 1

  api_id      = aws_apigatewayv2_api.main.id
  domain_name = aws_apigatewayv2_domain_name.this[0].id
  stage       = aws_apigatewayv2_stage.default.id
}

# 統合先の Lambda を API Gateway が呼べるようにする。
#
# for_each のキーは "web-api" などの静的な文字列なので plan 時に確定する。
# 各 Lambda 側に置くと api の execution ARN を参照することになり、
# count / for_each が「apply まで確定しない値」に依存してしまう。
resource "aws_lambda_permission" "integrations" {
  for_each = var.integration_function_names

  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = each.value
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.main.execution_arn}/*"
}

# Lambda オーソライザを API Gateway が呼べるようにする。
resource "aws_lambda_permission" "authorizer" {
  statement_id  = "AllowExecutionFromAPIGatewayAuthorizer"
  action        = "lambda:InvokeFunction"
  function_name = var.authorizer_function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.main.execution_arn}/authorizers/${aws_apigatewayv2_authorizer.device.id}"
}
