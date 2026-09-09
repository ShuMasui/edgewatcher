terraform {
  required_version = "~> 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # tfstate の置き場。backend ブロックには変数も式も書けないため、値は
  # リテラルで持つ。バケット名は bootstrap が
  #   edgewatcher-tfstate-<アカウントID>-<version>
  # で決め打ちするので、-backend-config で外から渡す理由がない。
  # 渡す方式にすると、init のたびに正しい文字列を手で与える必要があり、
  # 間違えたときに「別の state に向いたまま plan が通る」という最悪の
  # 壊れ方をする。アカウント ID は秘密ではない(infra/README.md Phase 0)。
  backend "s3" {
    bucket       = "edgewatcher-tfstate-606030504329-001"
    key          = "shared/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region
  profile = var.profile

  default_tags {
    tags = {
      Project     = "edgewatcher"
      Environment = "shared"
      ManagedBy   = "terraform"
      Stack       = "shared"
    }
  }
}

data "aws_caller_identity" "current" {}

# ---------------------------------------------------------------------------
# Route53 ホストゾーン(root_domain が確定するまでは作らない)
#
# ホストゾーンは dev / prod で共有するため、どちらの env state にも属せない。
# 環境を跨ぐ・低頻度・消えると復旧が重い、という性質から shared に置く
# (02-infra.md §2)。
# ---------------------------------------------------------------------------
resource "aws_route53_zone" "root" {
  count = var.root_domain == null ? 0 : 1

  name    = var.root_domain
  comment = "EdgeWatcher root zone (managed by terraform)"
}

# ---------------------------------------------------------------------------
# GitHub Actions 用の OIDC プロバイダ
#
# 長期のアクセスキーは発行しない(02-infra.md §8)。
# thumbprint_list は指定しない。AWS は 2023 年以降、GitHub のような
# 既知の IdP について自前の信頼ストアで検証しており、指定は不要。
# ---------------------------------------------------------------------------
resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = []
}

locals {
  repo_subject_prefix = "repo:${var.github_owner}/${var.github_repo}"

  # plan は PR から走るため branch / pull_request の両方を許す。
  plan_subjects = [
    "${local.repo_subject_prefix}:pull_request",
    "${local.repo_subject_prefix}:ref:refs/heads/*",
  ]
}

# ---------------------------------------------------------------------------
# sub クレームの形は、ジョブが environment を宣言しているかで変わる
#
#   environment なし : repo:<owner>/<repo>:ref:refs/heads/<branch>
#   environment あり : repo:<owner>/<repo>:environment:<name>
#
# **environment を宣言するとブランチは sub から消える。**両方は入らない。
# backend-deploy / web-deploy / infra-apply はいずれも environment: を
# 宣言しているため、ここで ref 形を期待すると AssumeRoleWithWebIdentity が
# AccessDenied になる(実際になった)。
#
# ブランチの制限は sub とは別の ref クレームで担保する。GitHub の
# Environment 側の「デプロイ可能ブランチ」でも同じことはできるが、それだと
# 強制する場所が AWS の外に出てしまい、この state を読んでも何が許されて
# いるのか分からなくなる。
# ---------------------------------------------------------------------------

data "aws_iam_policy_document" "github_assume" {
  for_each = local.roles

  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = each.value.subjects
    }

    # ref は sub とは別のクレームとしてトークンに常に入っている。
    # refs/heads/* に限ることでタグや PR の ref を除外している
    # (これが environment 形の sub で失われたブランチ側の縛り)。
    dynamic "condition" {
      for_each = length(each.value.refs) == 0 ? [] : [each.value.refs]

      content {
        test     = "StringLike"
        variable = "token.actions.githubusercontent.com:ref"
        values   = condition.value
      }
    }
  }
}

locals {
  # 環境ごとのリソース名プレフィックス。IAM ポリシーのリソース指定に使う。
  envs = toset(["dev", "prod"])

  roles = merge(
    {
      # PR で自動実行される plan には読み取り権限しか渡らない。
      # PR 経由で本番リソースが変更される経路が構造的に閉じる(02-infra.md §8)。
      # plan を走らせるワークフローはまだ無い。追加するときに
      # environment: を宣言するなら、subjects も environment 形に
      # 変える必要がある(上のコメント)。
      "edgewatcher-ci-plan" = {
        subjects = local.plan_subjects
        refs     = []
        managed  = ["arn:aws:iam::aws:policy/ReadOnlyAccess"]
        inline   = data.aws_iam_policy_document.tfstate_read.json
      }
    },
    {
      # refs をブランチ全体に開けてあるのは暫定。ワークフローが main に
      # 乗ったら ["refs/heads/main"] に絞る。1行で戻せる。
      for env in local.envs : "edgewatcher-ci-apply-${env}" => {
        subjects = ["${local.repo_subject_prefix}:environment:${env}"]
        refs     = ["refs/heads/*"]
        managed  = ["arn:aws:iam::aws:policy/AdministratorAccess"]
        inline   = null
      }
    },
    {
      for env in local.envs : "edgewatcher-ci-deploy-${env}" => {
        subjects = ["${local.repo_subject_prefix}:environment:${env}"]
        refs     = ["refs/heads/*"]
        managed  = []
        inline   = data.aws_iam_policy_document.deploy[env].json
      }
    },
  )
}

# tfstate バケットの名前は bootstrap が
#   edgewatcher-tfstate-<アカウントID>-<version>
# で決めている(infra/bootstrap/main.tf の local.bucket_name)。変数で受け取るのをやめて同じ規則で組み立てるのは、
# 「apply のたびに正しい名前を渡す」という手順を消すため。渡し忘れると
# ci-plan ロールが実在しないバケットへの許可を持つだけになり、失敗が
# apply 時ではなく plan ワークフローの実行時まで遅れて現れる。
locals {
  tfstate_bucket = "edgewatcher-tfstate-${data.aws_caller_identity.current.account_id}-001"
}

data "aws_iam_policy_document" "tfstate_read" {
  statement {
    effect    = "Allow"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::${local.tfstate_bucket}"]
  }

  statement {
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = ["arn:aws:s3:::${local.tfstate_bucket}/*"]
  }
}

# アプリケーションコードのデプロイに必要な最小限。
# インフラは触らせない(Lambda のコード更新・S3 同期・CloudFront 無効化のみ)。
data "aws_iam_policy_document" "deploy" {
  for_each = local.envs

  statement {
    sid       = "UpdateLambdaCode"
    effect    = "Allow"
    actions   = ["lambda:UpdateFunctionCode", "lambda:GetFunction"]
    resources = ["arn:aws:lambda:${var.region}:${data.aws_caller_identity.current.account_id}:function:edgewatcher-${each.value}-*"]
  }

  statement {
    sid     = "SyncWebAssets"
    effect  = "Allow"
    actions = ["s3:ListBucket", "s3:PutObject", "s3:DeleteObject"]
    resources = [
      "arn:aws:s3:::edgewatcher-${each.value}-web-${data.aws_caller_identity.current.account_id}",
      "arn:aws:s3:::edgewatcher-${each.value}-web-${data.aws_caller_identity.current.account_id}/*",
    ]
  }

  statement {
    sid       = "InvalidateCloudFront"
    effect    = "Allow"
    actions   = ["cloudfront:CreateInvalidation", "cloudfront:GetInvalidation"]
    resources = ["*"]
  }
}

resource "aws_iam_role" "ci" {
  for_each = local.roles

  name               = each.key
  description        = "GitHub Actions OIDC role for EdgeWatcher"
  assume_role_policy = data.aws_iam_policy_document.github_assume[each.key].json

  # 誤設定時の被害を抑えるため、セッションは短めに固定する。
  max_session_duration = 3600
}

resource "aws_iam_role_policy_attachment" "ci_managed" {
  for_each = {
    for pair in flatten([
      for name, cfg in local.roles : [
        for arn in cfg.managed : {
          key    = "${name}|${arn}"
          role   = name
          policy = arn
        }
      ]
    ]) : pair.key => pair
  }

  role       = aws_iam_role.ci[each.value.role].name
  policy_arn = each.value.policy
}

resource "aws_iam_role_policy" "ci_inline" {
  for_each = { for name, cfg in local.roles : name => cfg if cfg.inline != null }

  name   = "${each.key}-inline"
  role   = aws_iam_role.ci[each.key].id
  policy = each.value.inline
}
