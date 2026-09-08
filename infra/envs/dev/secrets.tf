# ---------------------------------------------------------------------------
# Google OAuth クライアントシークレット (AUTH-07)
#
# Terraform が管理するのは**入れ物だけ**で、値は管理しない。
# aws_secretsmanager_secret_version をあえて置いていないのはそのためである。
# 値を Terraform に持たせると、それは tfvars か TF_VAR か state のいずれかに
# 平文で現れることになり、「シークレットを ASM に置いた」意味がなくなる。
#
# 値の投入は人間が一度だけ行う(infra/README.md の Phase 2)。
#
#   aws secretsmanager put-secret-value \
#     --secret-id edgewatcher-dev-google-oauth-client-secret \
#     --secret-string '<Google Cloud Console で発行したクライアントシークレット>'
#
# apply 時は GitHub Actions(.github/workflows/infra-apply.yml)がこの値を
# 読み出して TF_VAR_google_client_secret として渡す。手元で apply する場合も
# 同じで、README にコマンドがある。
#
# なお docs/05-backend.md §3.6 の「Secrets Manager も Parameter Store も
# 使わない」は**Lambda のランタイムに秘密を渡さない**という趣旨であり、
# ここには適用されない。この値は Lambda が読むものではなく、Cognito を
# 構成するために apply 時にだけ必要になる、性質の異なる秘密である。
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "google_oauth_client_secret" {
  name        = "${local.name_prefix}-google-oauth-client-secret"
  description = "Google OAuth client secret for the Cognito Web user pool (AUTH-07). Value is set out of band; Terraform manages only the container."

  # 誤削除しても取り戻せる猶予を残す。0(即時削除)にすると、同名で作り直す
  # ときに「削除待ち」と衝突せず作れてしまい、取り違えに気づけない。
  recovery_window_in_days = 7
}
