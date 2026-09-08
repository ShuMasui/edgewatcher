# dev 環境の設定値。環境ごとの差分はこのファイルに集約する(02-infra.md §2)。

env            = "dev"
retention_days = 1

# --- 確定したら埋める --------------------------------------------------------
# root_domain        = "example.com"                     # OPS-01
# google_client_id   = "xxx.apps.googleusercontent.com"  # AUTH-07(秘密ではない)
# google_idp_enabled = true                              # AUTH-07。2段階目の apply で立てる
#
# client secret はここには**書かない**。Secrets Manager
# (edgewatcher-dev-google-oauth-client-secret)に置き、apply 時に
# TF_VAR_google_client_secret として注入する。手順は infra/README.md の
# 「Google IdP を有効にする」。
