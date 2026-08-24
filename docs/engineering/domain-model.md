# EdgeWatcher ドメインモデル

最終更新: 2026-08-24
ステータス: MVP の集約構成を確定

関連: `00-overview.md` §5、`06-auth.md`、`03-web.md`、`01-openquestion.md` DATA-01 / DATA-02

本書は DynamoDB のキー設計(DATA-01)に入る前段として、**トランザクション整合性の境界**を定める。
物理設計(PK/SK・GSI・属性名)は本書の範囲外で、`dynamodb.md` に書く。

## 1. 集約は2つ

MVP で必要な集約は `Device` と `Observation` の2つ。

```text
Device [AR]
  deviceId          不変。再ペアリングしても変わらない
  ownerId           Cognito の sub
  name              オーナーが付けた表示名
  status            PENDING | PAIRED | DISCONNECTED | ARCHIVED
  interval          5 / 10 / 15 分
  lastReceivedAt    最後にアップロードを受けた時刻
  ├─ Credentials [VO]     deviceSecretHash, sessionTokenHash, sessionExpiresAt
  └─ PairingSession [E]   pairingCode, expiresAt(TTL), status(PENDING | CONSUMED)

Observation [AR]
  observationId
  deviceId          Device への ID 参照
  capturedAt
  imageKey          S3 のキー(本画像)
  thumbnailKey      S3 のキー(サムネイル)
  location          lat, lng
```

集約を跨ぐ参照は **ID のみ**で行う。`Observation` は `Device` のオブジェクトを保持しない。

### `User` 集約を持たない

Web ユーザーの認証は Cognito Web User Pool が担い、サイドバーに出すメールアドレスとアバターは
ID トークンから取れる(`06-auth.md` §7)。DynamoDB 側に `User` アイテムを置いても
現時点で持たせる属性がないため、作らない。

アプリ固有のユーザー設定(通知先、表示設定など)を持ち始めた時点で追加する。

### `Membership` 集約を持たない

複数名で1台の端末を閲覧する要求に備えて `Membership` 集約(`userId` × `deviceId` × `role`)を
置くことも検討したが、採用しない(`01-openquestion.md` AUTH-10)。
**1端末 = 1オーナー**を確定仕様とし、`Device` が `ownerId` を直接持つ。

MVP では `Membership` は常に1レコードしか存在せず、全クエリがその中間層を経由することになる。
オーナーの端末一覧は `ownerId` を PK にした GSI 1本で引ける。

## 2. 集約境界の根拠

集約はトランザクション整合性の境界である。`Device` の内訳は、
`06-auth.md` が要求するトランザクションからそのまま導かれる。

| 操作 | 触る集約 | 方式 |
| --- | --- | --- |
| 端末を追加 | Device | `TransactWriteItems` 1コール(Device + PairingSession) |
| ペアリング成立 | Device | `TransactWriteItems` 1コール(Session→CONSUMED、Device→PAIRED、資格情報付与) |
| Authorizer の検証 | Device | `GetItem` 1回 |
| セッション切断 / 名前変更 / 間隔変更 / 削除 | Device | `UpdateItem` |
| アップロード受信 | Observation + Device | **別々の書き込み。トランザクションにしない** |

### PairingSession が `Device` の内部エンティティである理由

ペアリング成立時、PairingSession の `CONSUMED` 化と Device の `PAIRED` 化は
`TransactWriteItems` 1コールで行う。条件が満たされなければ両方とも書かれないことで、
QR の使い捨てが構造的に担保される(`06-auth.md` §2)。

同一トランザクションで更新される以上、両者は同じ集約に属する。

### Credentials が `Device` の内部にある理由

Lambda Authorizer は `DEVICE#<deviceId>` への `GetItem` 1回で、レコードの存在・`status`・
`sessionTokenHash`・`sessionExpiresAt` の4点を判定する(`06-auth.md` §3)。
これは資格情報が Device と同一アイテムにあることを要求している。

### 集約 ≠ DynamoDB アイテム

`PairingSession` は `Device` 集約に属するが、**アイテムとしては別**になる。
端末は `pairingCode` だけを送ってくるため、コードから引く GSI が必要だからである。

集約が同一であることは「同じトランザクションで更新される」ことを意味するだけで、
1アイテムに収まることは要求しない。

### `lastReceivedAt` は射影であり、厳密な整合性を要求しない

アップロード受信は `Observation` の Put と `Device.lastReceivedAt` の更新を伴い、集約を跨ぐ。
これをトランザクションにはしない。

`lastReceivedAt` は `Observation` からの**射影**であり、用途は「応答なし」バッジの表示だけである
(`03-web.md` §1.3)。5分ごとに走る最も高頻度の経路を、表示専用の値のためにトランザクションで
縛る価値がない。片方だけが書かれた場合の最悪の結果は、バッジが一時的に古いことである。

## 3. Device のライフサイクル

```text
        [端末を追加]
             ↓
         PENDING ──[QRをスキャン]──→ PAIRED
             │                        │  ↑
             │                        │  │[再ペアリング]
             │        [セッション切断 / 端末からログアウト]
             │                        ↓  │
             │                   DISCONNECTED
             │                        │
             └────────[削除]──────────┴──→ ARCHIVED(終端)
```

- `ARCHIVED` は終端であり、そこから戻る遷移はない。再登録すると新しい `deviceId` になる
- `deviceId` は `PENDING` で採番されて以後不変。これにより、端末を初期化しても別の Android 端末に
  置き換えても、再ペアリングすれば同じ観測地点として履歴が繋がる(`03-web.md` §1.8.3)

### 削除は論理削除

`DELETE /devices/{id}` は Device を物理削除せず、`status = ARCHIVED` に更新して
資格情報を失効させる。1端末あたり観測レコードが保持期間内に最大288件あり、
リクエスト内で消しきるのが現実的でないため。

`ARCHIVED` に落ちた時点で Authorizer が拒否し、Web の一覧からも消え、端末数の枠も空くので、
**オーナーから見た挙動は物理削除と区別がつかない**。実体の削除は後日のスケジューリング処理に
まとめる(`01-openquestion.md` DATA-02。処理自体は MVP スコープ外)。

この設計により、Authorizer の即時失効を支えているのは**レコードの不在ではなく `status` の判定**に
なる。「取得できたか」だけで通してはならない。

## 4. MVP 外

`Recording`(録画)と `Stream`(ライブ配信)は MVP のスコープ外(`00-overview.md` §3)。

これらを `Observation` と束ねる共通の親集約(`Resource` のような抽象)は**置かない**。
3者はライフサイクルが大きく異なる。`Observation` は5分ごとに増える不変レコード、
`Recording` は長さを持つ区間、`Stream` はそもそも永続化されない可能性がある(WebRTC セッション)。
共通点が「デバイス由来である」ことしかないものを1つの集約にまとめても、
共通の不変条件を書けない。

必要になった時点で、それぞれ独立した集約として追加する。

## 5. 未確定

| 項目 | 参照 |
| --- | --- |
| PK/SK・GSI の物理設計 | 確定済み。`dynamodb.md` を参照(DATA-01) |
| `ARCHIVED` な Device と観測レコードの物理削除方式 | DATA-02 |
| `location` を Device 側にも保持するか(現状は Observation のみ) | 未起票 |
