# 関数ごとの最小権限。分割線が認可の境界と一致しているため、
# 各関数に必要な権限が自然に絞られる(05-backend.md §2.4)。
#
# リソース指定はテーブル ARN とインデックス ARN を明示し、ワイルドカードを使わない。

# authorizer: DEVICE#<id> への GetItem 1回のみ。
data "aws_iam_policy_document" "authorizer" {
  statement {
    sid       = "ReadDeviceForAuthorization"
    effect    = "Allow"
    actions   = ["dynamodb:GetItem"]
    resources = [module.data.table_arn]
  }
}

# web-api: 端末の管理と観測の閲覧。画像は署名生成のための GetObject のみ。
# PutObject は与えない — Web から画像が書き込まれる経路は存在しない。
data "aws_iam_policy_document" "web_api" {
  statement {
    sid    = "ManageDevicesAndObservations"
    effect = "Allow"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:Query",
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
      "dynamodb:DeleteItem",
      "dynamodb:TransactWriteItems",
    ]
    resources = concat([module.data.table_arn], module.data.index_arns)
  }

  # 署名付き URL の権限は署名者の権限を継承する。署名生成そのものは
  # API 呼び出しを伴わないが、この権限がないと発行した URL が使えない
  # (05-backend.md §2.4)。
  statement {
    sid       = "SignImageUrls"
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = ["${module.storage_images.bucket_arn}/*"]
  }
}

# device-auth: ペアリングの成立とトークン発行。S3 には触れない。
data "aws_iam_policy_document" "device_auth" {
  statement {
    sid    = "ConsumePairingSession"
    effect = "Allow"
    actions = [
      "dynamodb:GetItem",
      "dynamodb:Query",
      "dynamodb:UpdateItem",
      "dynamodb:TransactWriteItems",
    ]
    resources = concat([module.data.table_arn], module.data.index_arns)
  }
}

# device-api: 画像の受信。Query は与えない — 端末が他の観測を読む経路は存在しない。
data "aws_iam_policy_document" "device_api" {
  statement {
    sid    = "RecordObservation"
    effect = "Allow"
    actions = [
      "dynamodb:PutItem",
      "dynamodb:UpdateItem",
    ]
    resources = [module.data.table_arn]
  }

  statement {
    sid       = "StoreImages"
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = ["${module.storage_images.bucket_arn}/observations/*"]
  }
}
