# EdgeWatcher DynamoDB キー設計

最終更新: 2026-08-24
ステータス: 確定(`01-openquestion.md` DATA-01)

関連: `domain-model.md`(集約構成)、`06-auth.md`(認証フロー)、`03-web.md`(アクセスパターン)

シングルテーブル設計。テーブル名は `edgewatcher-<env>-main`(`02-infra.md` §9 の命名規則)。

## 1. 命名規則

PK / SK ともに **`<TYPE>#<ID>`** の形式で統一する。複合値は `#` で連結する。
型名は大文字、ID はそのまま埋める。

前置詞を付けるのは、単一テーブルに異種のアイテムが同居するためである。
`DEVICE#01J...` と `OBS#01J...` は前置詞だけで種別が判別でき、
Query の `begins_with` でそのまま種別による絞り込みになる。

## 2. ベーステーブル

| エンティティ | PK | SK |
| --- | --- | --- |
| Device | `DEVICE#<deviceId>` | `DEVICE#<deviceId>` |
| PairingSession | `DEVICE#<deviceId>` | `PAIRING#<pairingCode>` |
| Observation | `DEVICE#<deviceId>` | `OBS#<observationId>` |

**1端末 = 1パーティション**に集約する。Device 本体・ペアリングセッション・観測レコードが
すべて同じ PK に載るため、端末に閉じた操作はすべて単一パーティションで完結する。

Device の SK が PK と同じ値なのは、Lambda Authorizer が `deviceId` だけから
`GetItem` を1回で発行できるようにするため(`06-auth.md` §3)。

### パーティションサイズ

送信間隔5分・保持7日(prod の最大)で 1端末あたり観測 2,016 件。1アイテムは数百バイト程度なので
パーティションあたり数 MB に収まり、10GB の上限に対して大幅な余裕がある。
書き込みも5分に1回であり、ホットパーティションにはならない。

## 3. アイテム定義

### Device

| 属性 | 型 | 備考 |
| --- | --- | --- |
| `PK` / `SK` | S | `DEVICE#<deviceId>` |
| `entityType` | S | `Device` |
| `deviceId` | S | ULID。採番後は不変 |
| `ownerId` | S | Cognito の `sub` |
| `name` | S | オーナーが付けた表示名 |
| `status` | S | `PENDING` / `PAIRED` / `DISCONNECTED` / `ARCHIVED` |
| `interval` | N | 送信間隔(分)。5 / 10 / 15 |
| `deviceSecretHash` | S | 切断時に削除 |
| `sessionTokenHash` | S | 切断時に削除 |
| `sessionExpiresAt` | N | epoch 秒。**TTL ではなくコードで判定する**(§7) |
| `lastReceivedAt` | S | ISO 8601 |
| `latestThumbnailKey` | S | ダッシュボード用の非正規化(§5.1) |
| `latestCapturedAt` | S | 同上 |
| `createdAt` | S | ISO 8601 |
| `archivedAt` | S | `ARCHIVED` のときのみ |
| `GSI1PK` / `GSI1SK` | S | §4 |

### PairingSession

| 属性 | 型 | 備考 |
| --- | --- | --- |
| `PK` | S | `DEVICE#<deviceId>` |
| `SK` | S | `PAIRING#<pairingCode>` |
| `entityType` | S | `PairingSession` |
| `deviceId` / `ownerId` | S | |
| `pairingCode` | S | 256bit ランダムを base32 化したもの |
| `status` | S | `PENDING` / `CONSUMED` |
| `expiresAt` | N | epoch 秒。発行から5分(§7) |
| `createdAt` / `consumedAt` | S | ISO 8601 |
| `GSI1PK` / `GSI1SK` | S | §4 |
| `GSI2PK` / `GSI2SK` | S | §4 |

### Observation

| 属性 | 型 | 備考 |
| --- | --- | --- |
| `PK` | S | `DEVICE#<deviceId>` |
| `SK` | S | `OBS#<observationId>` |
| `entityType` | S | `Observation` |
| `observationId` | S | **ULID**(§5.2) |
| `deviceId` | S | |
| `capturedAt` | S | ISO 8601 |
| `imageKey` / `thumbnailKey` | S | S3 のキー |
| `lat` / `lng` | N | |
| `expiresAt` | N | `capturedAt` + `retention_days`(§7) |

GSI には一切載せない。観測は最も件数が多いため、インデックスに載せると
ストレージと WCU がそのまま倍増する。

## 4. GSI

### GSI1 — オーナー索引(疎)

| アイテム | `GSI1PK` | `GSI1SK` |
| --- | --- | --- |
| Device(`ARCHIVED` 以外) | `OWNER#<ownerId>` | `DEVICE#<deviceId>` |
| PairingSession | `OWNER#<ownerId>` | `PAIRING#<deviceId>` |
| Device(`ARCHIVED`) | `ARCHIVED` | `DEVICE#<deviceId>` |

**端末一覧が Query 1回で完結する。** `GSI1PK = OWNER#<sub>` を引くと、SK 順に
Device 群(`D...`)→ PairingSession 群(`P...`)がまとめて返る。Lambda 側で `deviceId` を
キーに結合すれば、`03-web.md` §1.3 の「有効な PairingSession がぶら下がっていれば
[QR を表示]」という表示ロジックを追加のクエリなしに組める。

**`ARCHIVED` はパーティションを付け替える。** 削除時に `GSI1PK` を `OWNER#<ownerId>` から
`ARCHIVED` へ書き換えることで、オーナーのクエリから自動的に外れる。フィルタ式も不要で、
読み捨てる RCU も発生せず、返ってきた Device の件数がそのまま端末数上限の判定に使える
(`03-web.md` §1.9 の「`ARCHIVED` は数えない」を構造で満たす)。

掃除処理は `GSI1PK = ARCHIVED` を引くだけでよい(処理自体は MVP スコープ外。DATA-02)。

> `ARCHIVED` を単一パーティションに集約するため、理屈のうえではホットパーティションになりうる。
> 削除は人間の手による低頻度の操作であり、この規模では問題にならないと判断する。

射影は `INCLUDE`。一覧表示に必要な属性(`name` / `status` / `interval` / `lastReceivedAt` /
`latestThumbnailKey` / `latestCapturedAt` / `deviceId` / `pairingCode` / `expiresAt`)のみを載せ、
資格情報のハッシュ類は載せない。

### GSI2 — ペアリングコード索引(疎)

| アイテム | `GSI2PK` | `GSI2SK` |
| --- | --- | --- |
| PairingSession | `PAIRING#<pairingCode>` | `PAIRING#<pairingCode>` |

端末は QR から読み取った `pairingCode` だけをバックエンドに送るため(`06-auth.md` §2)、
コードから逆引きする経路が要る。PairingSession アイテムにしか `GSI2PK` を付けないので、
インデックスにはペアリング中のセッションだけが載る(常に数件)。

射影は `KEYS_ONLY`。GSI2 の役割はベーステーブルの PK / SK を得ることだけであり、
実データは続くベーステーブルへの操作で読む。

#### 結果整合への対処

**GSI は結果整合である。** QR を発行した直後に端末がスキャンすると、GSI2 への伝播が
間に合わずヒットしない可能性がある。

実際には QR を画面に出してから人間が端末を向けるまでに数秒かかるため、伝播(通常1秒未満)は
十分に間に合う。それでも取りこぼしを許容しないため、**端末側は GSI2 が空振りした場合、
1秒間隔で最大3回リトライしてから「無効な QR」と表示する**。

この扱いは、コード自体が無効な場合(期限切れ・消費済み)とは区別する。前者は待てば解決し、
後者は待っても解決しないため。ベーステーブルまで到達したうえで条件付き更新に失敗したものだけを
「無効な QR」として扱う。

## 5. アクセスパターン

| # | パターン | 実装 |
| --- | --- | --- |
| 1 | オーナーの端末一覧 + ペアリング状態 | GSI1 Query(`GSI1PK = OWNER#<sub>`) |
| 2 | ダッシュボードの最新スナップショット | 上と同一の Query(§5.1) |
| 3 | 端末の観測履歴を指定日1日分・新しい順 | ベース Query(§5.2) |
| 4 | Authorizer の検証 | `GetItem`(`PK = SK = DEVICE#<id>`) |
| 5 | `pairingCode` からセッションを引く | GSI2 Query → ベースへ |
| 6 | ペアリング成立 | `TransactWriteItems`(§6) |
| 7 | 観測1件の取得(本画像URL発行) | `GetItem` |
| 8 | `ARCHIVED` の走査 | GSI1 Query(`GSI1PK = ARCHIVED`) |

### 5.1 ダッシュボードは Device 側に非正規化する

`03-web.md` §1.5 のダッシュボードは、全端末の**最新1枚**を60秒ごとに引く。
これを素直に作ると端末数ぶんの Query が並ぶ。

代わりに、アップロード受信時に Device の `latestThumbnailKey` / `latestCapturedAt` /
`lastReceivedAt` を更新し、**端末一覧の Query 1回でダッシュボードが完成する**ようにする。

この非正規化は `domain-model.md` §2 で定めた「`lastReceivedAt` は Observation からの射影であり、
厳密な整合性を要求しない」という方針の延長線上にある。Observation の Put と Device の Update は
トランザクションにしない。片方だけが書かれた場合の最悪の結果は、カードの画像が1サイクル古いことである。

#### 遅れて届いた観測で `latest*` を巻き戻さない

端末はオフラインから復旧すると、**最新の1枚を先に送り、残りを古い順に送る**(`04-native.md` §4)。
このとき素直に上書きすると、後から届く古い画像が `latestThumbnailKey` を書き戻してしまい、
ダッシュボードのカードが数時間前の画像に巻き戻る。

そのため `latest*` の更新には条件を付ける。

```
ConditionExpression:
  attribute_not_exists(latestCapturedAt) OR latestCapturedAt < :capturedAt
```

一方 `lastReceivedAt` は**条件なしで更新する**。古い画像であっても「たった今この端末から受信した」
事実に変わりはなく、これは「応答なし」判定に使う値だからである(`03-web.md` §1.3)。

したがって受信処理は次の形になる。

1. `latest*` と `lastReceivedAt` をまとめて条件付きで更新する
2. `ConditionalCheckFailedException` が返ったら、`lastReceivedAt` だけを条件なしで更新する

平常時は 1 が成功するため追加のコストは発生せず、2 が走るのは復旧時のバックフィル中だけである。

### 5.2 `observationId` に ULID を使う

ULID は**先頭48ビットがミリ秒精度のタイムスタンプ**であり、文字列としての辞書順が
生成時刻順と一致する。したがって `OBS#<ULID>` という命名規則を保ったまま、
SK がそのまま時系列インデックスになる。

- **指定日1日分の取得**: `SK BETWEEN 'OBS#<下限>' AND 'OBS#<上限>'`
  - 下限 = その日 00:00:00.000 のタイムスタンプ + ランダム部を `0` で埋めた16文字
  - 上限 = 翌日 00:00:00.000 のタイムスタンプ + ランダム部を `Z` で埋めた16文字
  - Crockford base32 では `0` が最小・`Z` が最大の文字であるため、この範囲で確実に1日を覆う
- **新しい順**: `ScanIndexForward = false`
- **最新1枚**: 上に `Limit = 1`

**時刻の範囲は必ずクエリ条件で指定する。** TTL 削除は最大48時間遅れるため、
パーティションを丸ごと読むと期限切れの観測が混ざる(`03-web.md` §1.6)。
5分間隔なら1日あたり最大288件で、1回の Query で全件が返る。
履歴画面が日付ナビで1日ずつ遡る設計なのは、この取得単位に合わせたものである。

`OBS#<日付>#<UUID>` のように日付を挟む設計も可能だが、`<TYPE>#<ID>` の規則が崩れる。
ULID なら規則を保ったまま同じことができる。

`deviceId` も同じ理由で ULID とし、ID の形式をシステム全体で1種類に揃える。

## 6. トランザクション

### 端末を追加(`POST /devices`)

`TransactWriteItems` 1コール。

- `Put` Device(`status = PENDING`、`GSI1PK = OWNER#<sub>`)。条件 = `attribute_not_exists(PK)`
- `Put` PairingSession(`status = PENDING`、`expiresAt = now + 300`)

### ペアリング成立(`POST /device/pair`)

GSI2 で `pairingCode` からベースの PK / SK を得たうえで、`TransactWriteItems` 1コール。

- `Update` PairingSession → `status = CONSUMED`
  条件 = `status = PENDING AND expiresAt > :now`
- `Update` Device → `status = PAIRED`、`deviceSecretHash` を設定

**両方が同一パーティション(`PK = DEVICE#<deviceId>`)に載っている**ため、
このトランザクションはパーティションを跨がない。条件が満たされなければ両方とも書かれず、
QR の使い捨てが構造的に担保される(`06-auth.md` §2)。

`expiresAt > :now` を条件に入れるのは、DynamoDB の TTL 削除が最大48時間遅れうるためである。
**TTL は容量の後始末であって、有効期限の判定に使ってはならない。**

### アップロード受信

トランザクションにしない(§5.1)。`Put` Observation と `Update` Device を個別に発行する。

## 7. TTL

**TTL 属性はテーブルに1つしか指定できない。** 属性名は `expiresAt` とする。

| アイテム | `expiresAt` | 目的 |
| --- | --- | --- |
| PairingSession | 発行から5分 | 期限切れセッションの自動消滅 |
| Observation | `capturedAt` + `retention_days` | S3 ライフサイクルと歩調を揃える(dev 1日 / prod 7日) |
| Device | **持たない** | 消えてはならない |

Observation に TTL を張ることで、S3 側の画像削除(`02-infra.md` §7)と
DynamoDB 側のメタデータ削除の期間が揃い、「画像はないのにレコードだけ残る」状態を避けられる。

**どちらも遅延削除である。** S3 ライフサイクルの評価は UTC 深夜に1日1回、DynamoDB の TTL 削除は
期限到達後おおむね48時間以内。したがって実データは「最低1日、多くて2日強」存在する。
読み取り側は期限切れアイテムが返ってくる前提で、クエリ条件で24時間に絞る
(`03-web.md` §1.6)。TTL を有効期限の判定に使ってはならない(§6)。

副次的な効果として、DATA-02 の掃除処理の負担が大幅に減る。端末を削除しても
観測レコードは保持期間で自然に消滅するため、スケジューリング処理は Device 本体と
`ARCHIVED` パーティションの始末だけを見ればよい。

**`Device.sessionExpiresAt` は TTL ではない。** これはセッショントークンの有効期限であり、
過ぎたら Authorizer が拒否するだけで、Device アイテム自体は残り続ける必要がある。
TTL 属性と紛らわしいため、名前を明確に分けている。

## 8. キャパシティ

オンデマンドキャパシティを使う。端末10台・5分間隔でも書き込みは 1日あたり約3,000件であり、
プロビジョンドの最小構成でも過剰になる。トラフィックの予測が立たない段階で
プロビジョンドを選ぶ理由がない。

## 9. 未確定

| 項目 | 参照 |
| --- | --- |
| `ARCHIVED` の物理削除を行うスケジューリング処理 | DATA-02(MVP スコープ外) |
| バックアップ方針(PITR を有効化するか) | DATA-04 |
