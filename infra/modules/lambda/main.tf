# Lambda 関数1つ分。関数・IAM ロール・ロググループ・権限をまとめて作る。
#
# Terraform は関数の「器」だけを管理し、コード本体は管理対象外とする
# (02-infra.md §5)。これをやらないと、コードをデプロイするたびに plan に
# 差分が出続け、「plan をレビューしてから prod に適用する」運用が
# ノイズに埋もれて機能しなくなる。

locals {
  function_name = "${var.name_prefix}-${var.name}"
}

# 初回作成用のダミー zip。以後 CI が update-function-code で置き換える。
data "archive_file" "placeholder" {
  type        = "zip"
  output_path = "${path.module}/.build/${var.name}-placeholder.zip"

  source {
    # provided.al2023 は bootstrap という名前の実行ファイルを起動する。
    filename = "bootstrap"
    content  = <<-EOT
      #!/bin/sh
      # EdgeWatcher placeholder handler.
      # 実際の Go バイナリは CI がデプロイする(02-infra.md §5)。
      echo "placeholder: no handler deployed yet" >&2
      exit 1
    EOT
  }
}

resource "aws_cloudwatch_log_group" "this" {
  # Lambda が暗黙に作るロググループには保持期間が設定されず無期限保存になる。
  # コストが際限なく積み上がるため、Terraform で明示的に作る(02-infra.md §4)。
  name              = "/aws/lambda/${local.function_name}"
  retention_in_days = var.log_retention_days
}

data "aws_iam_policy_document" "assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "this" {
  name               = "${local.function_name}-role"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

# ロググループを Terraform で作っているため、AWSLambdaBasicExecutionRole の
# CreateLogGroup は不要。書き込みだけを許可する。
data "aws_iam_policy_document" "logs" {
  statement {
    effect    = "Allow"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.this.arn}:*"]
  }
}

resource "aws_iam_role_policy" "logs" {
  name   = "logs"
  role   = aws_iam_role.this.id
  policy = data.aws_iam_policy_document.logs.json
}

# 関数ごとの最小権限。呼び出し側が JSON を組み立てて渡す(05-backend.md §2.4)。
#
# count は付けない。policy_json は data.aws_iam_policy_document 由来で、
# その中身がテーブル ARN などの「apply まで確定しない値」を含むため、
# count の条件に使うと plan 時にインスタンス数を決められない。
resource "aws_iam_role_policy" "custom" {
  name   = "custom"
  role   = aws_iam_role.this.id
  policy = var.policy_json
}

resource "aws_lambda_function" "this" {
  function_name = local.function_name
  role          = aws_iam_role.this.arn

  # Go は独自ランタイムで動かす。arm64 は x86 比で約2割安く、
  # Go はクロスコンパイルが容易(05-backend.md §2.1)。
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  handler       = "bootstrap"

  filename         = data.archive_file.placeholder.output_path
  source_code_hash = data.archive_file.placeholder.output_base64sha256

  # メモリは CPU 割り当ての代理変数。値の根拠は 05-backend.md §2.2。
  memory_size = var.memory_size
  timeout     = var.timeout

  environment {
    variables = var.environment
  }

  lifecycle {
    # コード本体は CI が管理する。ここを外すと plan がコード差分で埋まる。
    ignore_changes = [filename, source_code_hash]
  }

  depends_on = [
    aws_iam_role_policy.logs,
    aws_cloudwatch_log_group.this,
  ]

  tags = {
    Name = local.function_name
  }
}

# API Gateway からの invoke 権限は api モジュール側で付与する。
# ここで持つと api の出力を参照することになり、count が
# 「apply まで確定しない値」に依存してしまうため。
