# DynamoDB シングルテーブル。キー設計の根拠は docs/engineering/dynamodb.md。
#
#   Device          PK = DEVICE#<deviceId>   SK = DEVICE#<deviceId>
#   PairingSession  PK = DEVICE#<deviceId>   SK = PAIRING#<pairingCode>
#   Observation     PK = DEVICE#<deviceId>   SK = OBS#<observationId(ULID)>
#
# 1端末 = 1パーティションに集約し、端末に閉じた操作を単一パーティションで完結させる。

resource "aws_dynamodb_table" "main" {
  name = "${var.name_prefix}-main"

  # 端末10台・5分間隔では書き込みが1日3,000件程度。プロビジョンドの
  # 最小構成でも過剰になるためオンデマンドを使う(05-backend.md §2.3 と同じ判断)。
  billing_mode = "PAY_PER_REQUEST"

  # テーブルレベルの key_schema は provider v6.61 時点でまだ提供されていないため
  # hash_key / range_key を使う(非推奨の警告が出るが代替がない)。
  # GSI 側は key_schema に移行済み。
  hash_key  = "PK"
  range_key = "SK"

  attribute {
    name = "PK"
    type = "S"
  }

  attribute {
    name = "SK"
    type = "S"
  }

  attribute {
    name = "GSI1PK"
    type = "S"
  }

  attribute {
    name = "GSI1SK"
    type = "S"
  }

  attribute {
    name = "GSI2PK"
    type = "S"
  }

  attribute {
    name = "GSI2SK"
    type = "S"
  }

  # GSI1 — オーナー索引(疎)
  #   Device(ARCHIVED 以外)  GSI1PK = OWNER#<ownerId>  GSI1SK = DEVICE#<deviceId>
  #   PairingSession          GSI1PK = OWNER#<ownerId>  GSI1SK = PAIRING#<deviceId>
  #   Device(ARCHIVED)       GSI1PK = ARCHIVED         GSI1SK = DEVICE#<deviceId>
  #
  # 端末一覧が Query 1回で完結する。ARCHIVED はパーティションを付け替えるため
  # オーナーのクエリから自動的に外れる。
  global_secondary_index {
    name = "GSI1"

    key_schema {
      attribute_name = "GSI1PK"
      key_type       = "HASH"
    }

    key_schema {
      attribute_name = "GSI1SK"
      key_type       = "RANGE"
    }

    projection_type = "INCLUDE"

    # 一覧表示に必要な属性だけを載せ、資格情報のハッシュ類は載せない。
    non_key_attributes = [
      "deviceId",
      "name",
      "status",
      "interval",
      "lastReceivedAt",
      "latestThumbnailKey",
      "latestCapturedAt",
      "pairingCode",
      "expiresAt",
    ]
  }

  # GSI2 — ペアリングコード索引(疎)
  # 端末は QR から読み取った pairingCode だけを送るため、逆引きの経路が要る。
  # ベーステーブルの PK / SK を得るのが役割なので KEYS_ONLY で足りる。
  global_secondary_index {
    name = "GSI2"

    key_schema {
      attribute_name = "GSI2PK"
      key_type       = "HASH"
    }

    key_schema {
      attribute_name = "GSI2SK"
      key_type       = "RANGE"
    }

    projection_type = "KEYS_ONLY"
  }

  # TTL 属性はテーブルに1つしか指定できない。
  #   PairingSession : 発行から5分
  #   Observation    : capturedAt + retention_days
  #   Device         : 持たない(消えてはならない)
  #
  # TTL 削除は最大48時間遅れるため、有効期限の判定には使わない。
  # 判定は必ず条件式の expiresAt > now で行う(06-auth.md §2)。
  ttl {
    attribute_name = "expiresAt"
    enabled        = true
  }

  point_in_time_recovery {
    enabled = var.point_in_time_recovery
  }

  # 観測レコードは TTL で日常的に消えるが、テーブルそのものを
  # 取り違えて消すのは致命的。
  lifecycle {
    prevent_destroy = false
  }

  tags = {
    Name = "${var.name_prefix}-main"
  }
}
