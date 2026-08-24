# Web 利用者の認証。Cognito は Web User Pool のみを持つ。
# 端末の認証は Lambda Authorizer と DynamoDB で完結するため、
# Device User Pool は作らない(06-auth.md §1)。

resource "aws_cognito_user_pool" "web" {
  name = "${var.name_prefix}-web"

  # Google を唯一の IdP とし、メール/パスワード認証は提供しない
  # (06-auth.md §7)。そのためパスワードポリシーは実質使われないが、
  # Cognito が既定値を要求するため明示しておく。
  password_policy {
    minimum_length                   = 12
    require_lowercase                = true
    require_uppercase                = true
    require_numbers                  = true
    require_symbols                  = true
    temporary_password_validity_days = 1
  }

  # 管理者による作成のみ = セルフサインアップを閉じる。
  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  username_attributes = ["email"]

  account_recovery_setting {
    recovery_mechanism {
      name     = "verified_email"
      priority = 1
    }
  }

  # User Pool は属性変更で置換(destroy/create)されると登録済みユーザーが
  # 消失する。インフラを手動 apply にしている最大の理由がこれ
  # (00-overview.md §8)。plan で置換が出たら必ず止まること。
  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Name = "${var.name_prefix}-web"
  }
}

# Hosted UI のドメイン。カスタムドメインは使わず Cognito の既定プレフィックスを使う。
# ACM も Route53 も不要で、ドメイン未確定でもログイン経路を立てられる。
resource "aws_cognito_user_pool_domain" "web" {
  domain       = "${var.name_prefix}-${var.account_id}"
  user_pool_id = aws_cognito_user_pool.web.id
}

# ---------------------------------------------------------------------------
# Google IdP
#
# client_id / client_secret が未取得の間は作らない(01-openquestion.md AUTH-07)。
# 取得したら値を入れて apply するだけで有効になる。
# ---------------------------------------------------------------------------
resource "aws_cognito_identity_provider" "google" {
  count = var.google_client_id == null ? 0 : 1

  user_pool_id  = aws_cognito_user_pool.web.id
  provider_name = "Google"
  provider_type = "Google"

  provider_details = {
    client_id                     = var.google_client_id
    client_secret                 = var.google_client_secret
    authorize_scopes              = "openid email profile"
    attributes_url_add_attributes = "true"
  }

  attribute_mapping = {
    email    = "email"
    username = "sub"
    name     = "name"
    picture  = "picture"
  }
}

resource "aws_cognito_user_pool_client" "web" {
  name         = "${var.name_prefix}-web-client"
  user_pool_id = aws_cognito_user_pool.web.id

  # SPA なのでクライアントシークレットは持たせない(PKCE を使う)。
  generate_secret = false

  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]

  # Google が未設定の間は COGNITO のみ。設定後は Google が加わる。
  supported_identity_providers = compact([
    "COGNITO",
    var.google_client_id == null ? "" : "Google",
  ])

  callback_urls = var.callback_urls
  logout_urls   = var.logout_urls

  # ブラウザのトークンを破棄したうえで Hosted UI の /logout へ
  # リダイレクトする方針のため、既定の有効期間で足りる(03-web.md §1.4.2)。
  access_token_validity  = 1
  id_token_validity      = 1
  refresh_token_validity = 30

  token_validity_units {
    access_token  = "hours"
    id_token      = "hours"
    refresh_token = "days"
  }

  prevent_user_existence_errors = "ENABLED"

  depends_on = [aws_cognito_identity_provider.google]
}
