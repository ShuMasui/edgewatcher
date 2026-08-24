terraform {
  required_version = "~> 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # bucket は bootstrap の出力に依存するため、init に -backend-config で渡す。
  backend "s3" {
    key          = "envs/dev/terraform.tfstate"
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project     = "edgewatcher"
      Environment = var.env
      ManagedBy   = "terraform"
    }
  }
}

# CloudFront 用の ACM 証明書は us-east-1 に存在しなければならない
# (02-infra.md §3)。root_domain 確定後に domain モジュールが使う。
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  default_tags {
    tags = {
      Project     = "edgewatcher"
      Environment = var.env
      ManagedBy   = "terraform"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  name_prefix = "edgewatcher-${var.env}"

  # root_domain が確定するまでは null のままで、既定ドメインで動作する
  # (01-openquestion.md OPS-01)。
  web_domain = var.root_domain == null ? null : "app.${var.env}.${var.root_domain}"
  api_domain = var.root_domain == null ? null : "api.${var.env}.${var.root_domain}"
}

# ---------------------------------------------------------------------------
# データ層
# ---------------------------------------------------------------------------

module "data" {
  source = "../../modules/data"

  name_prefix            = local.name_prefix
  point_in_time_recovery = var.point_in_time_recovery
}

module "storage_images" {
  source = "../../modules/storage-images"

  name_prefix    = local.name_prefix
  account_id     = data.aws_caller_identity.current.account_id
  retention_days = var.retention_days
}

# ---------------------------------------------------------------------------
# 配信層
# ---------------------------------------------------------------------------

module "web_hosting" {
  source = "../../modules/web-hosting"

  name_prefix = local.name_prefix
  account_id  = data.aws_caller_identity.current.account_id

  # ドメイン確定後は local.web_domain と証明書 ARN が入る。
  web_domain          = local.web_domain
  acm_certificate_arn = null

  upload_placeholder = var.upload_placeholder
  placeholder_html   = file("${path.module}/../../placeholder/index.html")
}

# ---------------------------------------------------------------------------
# 認証
# ---------------------------------------------------------------------------

module "cognito_web" {
  source = "../../modules/cognito-web"

  name_prefix = local.name_prefix
  account_id  = data.aws_caller_identity.current.account_id

  # 未取得の間は null。Google IdP は作られず、誰もログインできない状態になる
  # (01-openquestion.md AUTH-07)。
  google_client_id     = var.google_client_id
  google_client_secret = var.google_client_secret

  callback_urls = ["${module.web_hosting.url}/", "http://localhost:5173/"]
  logout_urls   = ["${module.web_hosting.url}/login", "http://localhost:5173/login"]
}

# ---------------------------------------------------------------------------
# Lambda(4関数)
#
# 認可の境界で分ける(05-backend.md §1.1)。分割線を認可の境界と一致させることで
# IAM ポリシーが関数ごとに自然に最小化される。
# ---------------------------------------------------------------------------

locals {
  common_env = {
    TABLE_NAME     = module.data.table_name
    IMAGES_BUCKET  = module.storage_images.bucket_name
    RETENTION_DAYS = tostring(var.retention_days)
    DEVICE_LIMIT   = tostring(var.device_limit)
    SIGNED_URL_TTL = tostring(var.signed_url_ttl)
    ENV            = var.env
  }
}

# 毎リクエスト走る。DynamoDB GetItem 1回のみ。
module "lambda_authorizer" {
  source = "../../modules/lambda"

  name_prefix = local.name_prefix
  name        = "authorizer"
  memory_size = 128
  timeout     = 3
  environment = local.common_env

  policy_json = data.aws_iam_policy_document.authorizer.json
}

module "lambda_web_api" {
  source = "../../modules/lambda"

  name_prefix = local.name_prefix
  name        = "web-api"
  memory_size = 256
  timeout     = 10

  environment = merge(local.common_env, {
    COGNITO_USER_POOL_ID = module.cognito_web.user_pool_id
  })

  policy_json = data.aws_iam_policy_document.web_api.json
}

module "lambda_device_auth" {
  source = "../../modules/lambda"

  name_prefix = local.name_prefix
  name        = "device-auth"
  memory_size = 256
  timeout     = 10
  environment = local.common_env

  policy_json = data.aws_iam_policy_document.device_auth.json
}

# 最大 4.5MB のペイロードを保持し S3 へ2回 Put する。
# メモリを厚く取るのは容量ではなく CPU 割り当てのため(05-backend.md §2.2)。
module "lambda_device_api" {
  source = "../../modules/lambda"

  name_prefix = local.name_prefix
  name        = "device-api"
  memory_size = 1024
  timeout     = 30
  environment = local.common_env

  policy_json = data.aws_iam_policy_document.device_api.json
}

# ---------------------------------------------------------------------------
# API Gateway
# ---------------------------------------------------------------------------

module "api" {
  source = "../../modules/api"

  name_prefix = local.name_prefix

  cognito_issuer    = module.cognito_web.issuer
  cognito_client_id = module.cognito_web.user_pool_client_id

  authorizer_invoke_arn    = module.lambda_authorizer.invoke_arn
  authorizer_function_name = module.lambda_authorizer.function_name

  integrations = {
    "web-api"     = module.lambda_web_api.invoke_arn
    "device-auth" = module.lambda_device_auth.invoke_arn
    "device-api"  = module.lambda_device_api.invoke_arn
  }

  integration_function_names = {
    "web-api"     = module.lambda_web_api.function_name
    "device-auth" = module.lambda_device_auth.function_name
    "device-api"  = module.lambda_device_api.function_name
  }

  # 経路一覧の根拠は 05-backend.md §1.2。
  web_routes = [
    "GET /app-config",
    "GET /devices",
    "POST /devices",
    "POST /devices/{id}/pairing-sessions",
    "GET /devices/{id}/pairing-sessions/latest",
    "POST /devices/{id}/disconnect",
    "PATCH /devices/{id}",
    "DELETE /devices/{id}",
    "GET /devices/{id}/observations",
    "GET /observations/{id}/image",
  ]

  device_public_routes = [
    "POST /device/pair",
    "POST /device/token",
  ]

  device_routes = [
    "POST /device/uploads",
    "POST /device/logout",
  ]

  cors_allow_origins = [module.web_hosting.url, "http://localhost:5173"]

  api_domain          = local.api_domain
  acm_certificate_arn = null
}
