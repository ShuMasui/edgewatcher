# FGS + Wake Lock + AlarmManager 二重化スパイク 実行手順書

最終更新: 2026-08-25
ステータス: 手順確定・**未実施**(実機での計測はこれから)

関連: `04-native.md` §3.0・§3.1(検証対象の設計判断)、`01-openquestion.md` APP-01 / APP-03 / APP-04、
`05-backend.md` §1.3・§3.4(画像サイズ上限・Pull 型 `nextConfig`)

**本書がこのスパイクの唯一の恒久成果物である。** 検証に使ったコード
(`spike/android-fgs-timer/`、パッケージ `com.example.ewspike`)は使い捨てで、
計測後に削除される(§8)。削除された後に残るのは本書だけなので、
コードを読まないと分からない挙動は書かない。

---

## 1. 目的

`04-native.md` §3.0 は、次の主張を**実証データなしに**行っている。

> Foreground Service + 常時保持する `PARTIAL_WAKE_LOCK` + `AlarmManager.setExactAndAllowWhileIdle`
> の二重化があれば、画面 OFF・5分間隔のタイマーは屋外常設環境でも止まらない。

この主張が崩れると、影響は 04-native.md 単体にとどまらない。

- 5 / 10 / 15 分という送信間隔そのもの(`00-overview.md` §6、`01-openquestion.md` APP-01)
- Pull 型で `nextConfig` を配る設計。サーバから端末へ能動的に push する経路を持たないため、
  端末側のタイマーが唯一の起点になる(`05-backend.md` §3.4)
- `WorkManager`(最短15分)を採らずに自前タイマーを選んだ判断そのもの(`04-native.md` §3.1)

つまりこのスパイクは、**本番の Android 実装に着手する前に**、上記の前提が実機で成立するかを
確認するためのものである。成立しなければ設計を書き直す必要があり、その手戻りは
実装後より実装前の方が圧倒的に安い。

検証したい問いは `01-openquestion.md` の APP-04 に集約されるが、特に以下の4点。

| 問い | 内容 |
| --- | --- |
| Q1 | Wake Lock は画面 OFF 中のタイマーを本当に守るか。守れているなら `handler` 側のタイマーも `alarm` 側とほぼ同時に発火し続けるはず |
| Q2 | メーカー独自の省電力機構やプロセスキルによって、欠測(タイマーが一度も発火しない)は起きるか |
| Q3 | 発熱は屋外常設に耐える水準か(`batteryTempC`) |
| Q4 | CameraX を毎サイクル bind/unbind し、プレビュー Surface なしで `ImageCapture` だけを回す実装は成立するか。`captureMs` / `jpegBytes` の分布はどうか |

### スパイクの中心的な設計(なぜ2本のタイマーが両方記録を残すのか)

検証コードは1サイクルごとに **2本の独立したタイマー**を同時に仕掛ける。

- プロセス内の `Handler.postDelayed`(`source=handler`)
- `AlarmManager.setExactAndAllowWhileIdle`(`source=alarm`)

**どちらも他方をキャンセルしない。両方が発火し、両方が自分の CSV 行を書く。** 同一サイクルの
2行は同じ `seq` を共有し、`source` 列でのみ区別される。次サイクルのタイマーは、
その `seq` に対して**先に到着した方が1回だけ**仕掛ける(2本目が到着しても再度仕掛け直さない)。

これは実装上の手抜きではなく、このスパイクが答えるべき問い(Q1)そのものと直結した設計判断である。
もし「先着した方だけ記録し、負けた方は捨てる」実装にしていたら、CSV からは
「`handler` が遅れて発火した」のか「`handler` が一度も発火しなかった」のかを区別できない。
両方を記録して初めて、Wake Lock がタイマーを守れているのか、実質 `AlarmManager` だけが
生命線なのかを判別できる。

---

## 2. セットアップ手順

以下はコピペで実行できる。`assembleDebug` までは本タスクで実際に実行し、成功を確認済み
(§2.1)。`adb install` 以降は実機が無いため**未検証**。

```bash
export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
cd spike/android-fgs-timer && ./gradlew assembleDebug
~/Library/Android/sdk/platform-tools/adb install -r app/build/outputs/apk/debug/app-debug.apk
```

- `JAVA_HOME` を明示するのは、PATH 上の `java` が JDK 26 であり AGP がこれを拒否するため。
  `/Applications/Android Studio.app/Contents/jbr/Contents/Home` の JBR を指す必要がある
- `adb` は PATH に無いため、`~/Library/Android/sdk/platform-tools/adb` のフルパスで呼ぶ
- パッケージ名は `com.example.ewspike`。本番の EdgeWatcher アプリとは無関係の使い捨てID

### 2.1 検証済みの事実

このタスクの実施中に、上記のうち **`./gradlew clean assembleDebug` を実際に実行し**、
`JAVA_HOME` を上記の値に設定した状態で `BUILD SUCCESSFUL` になることを確認した。
成果物 `app/build/outputs/apk/debug/app-debug.apk`(約 4.1MB)の生成も確認済み。

`adb install` 以降は実機が存在しないため、コマンドの構文以上のことは確認していない。
**未来の実施者は、ここから先で初めて未検証の領域に入る**ことを踏まえてほしい。

---

## 3. 端末側の準備

計測を始める前に、対象端末で以下をすべて済ませておく。抜けがあると、
タイマーが本当に落ちたのか準備不足だったのかを区別できなくなる。

1. **開発者オプションを有効化し、USBデバッグを許可する**(`adb install` に必要)
2. アプリ初回起動時のランタイム権限ダイアログで、**カメラ**と(Android 13 以上なら)
   **通知**を許可する。カメラ権限が無いと `CaptureService` は `foregroundServiceType=camera`
   での起動に失敗し、`error=fgs_start_failed:<Exception>` の行だけが残って計測が始まらない
3. アプリ内の **`[電池最適化の除外を開く]`** ボタンで、この端末固有の電池最適化対象から除外する
4. **Android 12 以上では**、アプリ内の **`[正確なアラームの許可を開く]`** ボタンで
   `SCHEDULE_EXACT_ALARM` を許可する。許可しないまま `[開始]` を押すと
   「正確なアラームの許可がありません」というトーストが出た上で計測は始まるが、
   `alarm` 側のタイマーは一度も仕掛からず、CSV に `error=alarm_schedule_denied` の行が
   毎サイクル残る(§6 の判定表・APP-04 参照)
5. **充電器に挿しっぱなしにする。** これは利便性の問題ではなく計測の前提そのもの。
   `04-native.md` §3.0 の Wake Lock 常時保持は「常時給電」を前提にしており、
   バッテリー駆動では Doze の対象になり得るため、給電が切れた状態のデータは
   この検証の答えにならない

---

## 4. 計測プロトコル

アプリ画面の **`[間隔: 1分 / 5分]`** ボタンで間隔を切り替えられる(トグル)。

1. **スモークテスト**: 間隔を **1分**にし、画面 ON のまま `[開始]` を押して10分放置する。
   `adb pull` するか、あるいは**画面上に表示される CSV 末尾10行**で、
   10行程度(§4.1 の理由により実際は約20行)出ていることを確認する。
   ここで CSV が全く増えない、あるいは `error` 列がすべて埋まっているようなら、
   §3 の準備不足を疑い、本番相当の計測に進まない
2. **本番相当の計測**: 間隔を **5分**にし、画面OFF・充電中の状態で **24時間**放置する
3. **可能なら、メーカーの異なる2機種で回す。** `04-native.md` §3.0 が警戒しているのは
   「メーカー独自の省電力機構(タイマーを積極的に殺す実装)」であり、これは機種依存の挙動なので、
   1機種だけの結果では「この設計は一般に成立する」とは言えない。2機種目は判定表(§6)の
   ❌行の切り分けに直接使う

### 4.1 CSV の行数について(重要 — 壊れているわけではない)

**1サイクルにつき CSV に約2行が書かれる。** 5分間隔・24時間なら 288 サイクル ×約2行 = **約576行**
になるのが正常であり、288行ではない。

理由は §1 で述べた設計そのもの: `handler` と `alarm` の両方が発火し、両方が自分の行を書くため。
写真の撮影は「そのサイクルで先に到着した方」だけが行うので、**後から到着した2行目は
`captureMs` / `jpegBytes` が空欄になる。** これは欠陥ではない。CSV を見て
「行数が半分しかない」「半分の行に写真の情報が無い」と考えて調査を始めないこと。

逆に、1サイクルにつき1行しか無い、あるいは `source` が片方しか出てこないサイクルが
連続するようであれば、それこそが §6 の判定に関わる異常である。

---

## 5. 結果の取り出し

以下のいずれかで CSV を取得する。ファイルは
`getExternalFilesDir(null)/spike-log.csv`(アプリ専用の外部ストレージ領域)にある。

```bash
~/Library/Android/sdk/platform-tools/adb pull \
  /sdcard/Android/data/com.example.ewspike/files/spike-log.csv .
```

または、アプリ画面の **`[CSVを共有]`** ボタンから共有シートを開き、任意の方法
(メール添付・クラウド保存等)で端末外に持ち出す。

CSV の列は次の順で固定(`TickCsv.HEADER`、変更不可)。

```
seq,source,scheduledAtMs,firedAtMs,driftMs,screenOn,batteryPct,batteryTempC,
charging,instanceSeq,captureMs,jpegBytes,freeBytes,error
```

| 列 | 意味 |
| --- | --- |
| `seq` | サイクル番号。1から始まる連番 |
| `source` | `handler` または `alarm`。どちらのタイマーがこの行を書いたか |
| `scheduledAtMs` / `firedAtMs` | 予定時刻と実際に発火した時刻(epoch ms) |
| `driftMs` | `firedAtMs - scheduledAtMs`。正なら遅延 |
| `screenOn` | 発火時点で画面が ON だったか(`PowerManager.isInteractive`) |
| `batteryPct` / `batteryTempC` / `charging` | バッテリー残量・温度(℃)・充電中か |
| `instanceSeq` | プロセス起動のたびに+1される連番。**この値が上がっている箇所は、
  プロセスが一度死んで `START_STICKY` で再生成されたことを意味する** |
| `captureMs` / `jpegBytes` | 撮影所要時間(ms)とJPEGサイズ(byte)。そのサイクルで
  撮影を担当しなかった行(2行目)は空欄 |
| `freeBytes` | 発火時点の空き容量(byte) |
| `error` | §5.1 参照。空欄なら正常 |

### 5.1 `error` 列に出現しうる値

| 値 | 意味 |
| --- | --- |
| `alarm_schedule_denied` | `SCHEDULE_EXACT_ALARM` が許可されておらず、**そのサイクルの `alarm` 側は最初から仕掛けられていない**。「仕掛けたのに発火しなかった」のとは全く異なる状態なので混同しないこと |
| `alarm_broadcast_missing_extras` | `AlarmReceiver` からの中継 Intent に `seq`/`scheduledAtMs` が載っていなかった(通常発生しないはずの内部異常) |
| `fgs_start_failed:<Exception>` | `startForeground` が失敗し、サービスがそのまま停止した(多くはカメラ権限未許可) |
| `capture_timeout` | 撮影(bind〜保存)が15秒以内に終わらず、タイムアウトした。カメラは bind されないまま |
| `capture_timeout_camera_bound` | 同上のタイムアウトだが、カメラの bind 自体は成功していた(`takePicture` のコールバックがハングした) |
| `ImageCaptureException:<code><cause>` | CameraX の撮影自体が失敗した。`<code>` は `ImageCaptureException.imageCaptureError`、`<cause>` はラップされた例外クラス名 |

---

## 6. 合否判定基準

CSV は単なるテキストなので、以下の一行コマンドで判定に必要な値を機械的に出せる。
**「なんとなく良さそう」で判定しない。** 数値を出し、下表のどの行に当たるかを機械的に決める。

以降のコマンドは、取り出した CSV を `spike-log.csv` という名前でカレントディレクトリに
置いた前提。ヘッダー行を除くため `tail -n +2` を通している。

### 6.1 集計コマンド

**`source` の内訳(行数)**

```bash
tail -n +2 spike-log.csv | awk -F',' '{print $2}' | sort | uniq -c
```

**欠測サイクル数(`handler`・`alarm` それぞれ、本来あるべき seq が欠けていないか)**

```bash
python3 - <<'PY'
import csv
with open("spike-log.csv") as f:
    rows = list(csv.DictReader(f))
max_seq = max(int(r["seq"]) for r in rows)
for source in ("handler", "alarm"):
    seen = {int(r["seq"]) for r in rows if r["source"] == source}
    missing = sorted(set(range(1, max_seq + 1)) - seen)
    print(f"{source}: {len(missing)} missing / {max_seq} expected -> {missing[:20]}")
PY
```

**`driftMs` のパーセンタイル(遅延分布。p95 を判定に使う)**

```bash
python3 - <<'PY'
import csv, statistics
with open("spike-log.csv") as f:
    drift = [int(r["driftMs"]) for r in csv.DictReader(f) if r["driftMs"] != ""]
drift.sort()
def pct(p):
    idx = min(len(drift) - 1, int(len(drift) * p))
    return drift[idx]
print("n=", len(drift), "p50=", pct(0.50), "p95=", pct(0.95), "max=", max(drift))
PY
```

**`batteryTempC` の最大値・持続時間の目視確認**

```bash
python3 - <<'PY'
import csv
with open("spike-log.csv") as f:
    rows = [r for r in csv.DictReader(f) if r["batteryTempC"] != ""]
temps = [(r["firedAtMs"], float(r["batteryTempC"])) for r in rows]
print("max=", max(t for _, t in temps))
over45 = [t for t in temps if t[1] > 45.0]
print("count(>45C)=", len(over45), "/ total=", len(temps))
PY
```

**`instanceSeq` の変化点(プロセス再生成の有無・欠測との近接確認)**

```bash
tail -n +2 spike-log.csv | awk -F',' '{print $1","$2","$9}' | \
  awk -F',' 'prev!="" && $3!=prev{print "instanceSeq changed at seq="$1": "prev"->"$3} {prev=$3}'
```

**`error` の内訳**

```bash
tail -n +2 spike-log.csv | awk -F',' '{print $14}' | sort | uniq -c | sort -rn
```

### 6.2 判定表

先に §6.3 の方法論的な注意を読んでから判定すること。特に、`instanceSeq` の変化点に
隣接する単発の欠測1件は、この判定表の「欠測」として数えない(§6.3-1)。

| 判定 | 条件 | 意味と次のアクション |
| --- | --- | --- |
| ✅ 設計どおり | 24時間で欠測0(`instanceSeq` 変化点隣接の単発欠測を除く)、`driftMs` の p95 < 30秒 | §3.0 の二重化は成立している。本番実装をそのまま進めてよい。APP-04 の残課題(カメラAPI選定・位置情報取得方式・テスト方針)に進む |
| ⚠️ Alarm 依存 | 欠測0だが `source` の内訳で `alarm` が大半(目安: `handler` が全体の1割未満) | Wake Lock はタイマーを守れていない。`AlarmManager` が単独の生命線になっている。`04-native.md` §3.0 の記述を「二重化で担保」から「`AlarmManager` が主、`Handler`/Wake Lock は補助」に改める。挙動自体は許容範囲(欠測が無いため)だが、設計文書の説明責任として直す |
| ⚠️ ドリフト大 | 欠測0だが `driftMs` の p95 が数分規模(目安: 60,000ms 超) | 「5分間隔」という表記が実態と乖離する。`00-overview.md` §6 の間隔の意味づけ(「送信間隔」であって「厳密な周期」ではない、等)と `nextConfig` の解釈を見直す |
| ❌ 欠測あり | `instanceSeq` 変化点に隣接しない欠測が24時間で発生 | §3.0 の前提が崩れている。**まず2機種目のデータで機種固有かどうかを切り分ける。** 機種固有なら該当機種を対象から外すか個別対応、機種を問わず起きるなら設計の問題(`WorkManager` 15分への妥協、外部トリガの導入等)を検討する。**`01-openquestion.md` APP-01 と `00-overview.md` §6 まで戻って再検討する** |
| 🔥 発熱 | `batteryTempC` が持続的に(目安: 10分以上連続して)45°C 超 | 屋外常設(直射日光下ではさらに悪化)に耐えない可能性がある。Wake Lock の常時保持を諦め、`AlarmManager` 単独 + 撮影時のみ Wake Lock を取得する方式に切り替えることを検討する |
| 📷 Q4 への入力 | 常に確認する(合否とは独立) | `captureMs` の分布(cycle 1 とプロセス再起動直後の1サイクルを除外して集計すること。§6.3-4)、`jpegBytes` の分布、プレビュー無しの CameraX bind がクラッシュせず動いたか。結果は APP-04(CameraX か Camera2 か)と APP-03(画像仕様・4.5MB 上限)の判断材料にする |

複数の行に同時に該当する場合(例: 欠測は無いが `alarm` 依存かつ発熱もある)は、
**より深刻な行を優先して次のアクションを取る。** 欠測系(❌)>発熱(🔥)>Alarm依存/ドリフト大(⚠️)
の順で優先する。

### 6.3 方法論的な注意(判定の前に必ず読む)

判定に影響する既知の制約を、レビューで洗い出した順に記す。書かないと、正常な計測結果を
「壊れている」と誤読することになる。

1. **先着した側の行は撮影完了後に非同期で書かれる。** そのため、CSV への書き込みが
   終わる前にプロセスが死ぬと、その行はまるごと失われる。行が無いことと
   「タイマーが一度も発火しなかったこと」は CSV 上では区別できない。
   **`instanceSeq` が変化した直後・直前に隣接する単発の欠測1件は、この理由による
   ものとみなし、§6.2 の「欠測」からは除外して数える。** 除外せずに数えると、
   Q2(欠測)の判定が本来より悪く出る
2. **撮影が15秒を超えると `capture_timeout` として記録され、その時点で `captureMs` は
   空欄のまま撮影自体が打ち切られる。** つまり `captureMs` の分布は **15秒で右側が
   打ち切られている(censored)**。実際にはもっと長くかかった撮影も「15秒」として
   扱われるわけではなく、単に `captureMs` が欠落する。また、タイムアウト後に
   JPEG ファイル自体は保存されることがあり、その場合そのファイルを参照する CSV 行は
   存在しない(ファイルと行が1対1に対応しない)
3. **`ImageCapture.Builder()` は解像度もJPEG品質も一切指定していない。** そのため
   `jpegBytes` が示すのは、この端末の**カメラが出す既定のフル解像度JPEG**のサイズであり、
   「選んだ設定」ではない。この値は 4.5MB という上限(`05-backend.md` §1.3)に対する
   **最悪ケースの参考値**としては正しいが、将来これを見て「この解像度を選定した」と
   誤解しないこと。APP-03 の解像度・圧縮率はこれから別途決める
4. **1サイクル目の `captureMs` は他のサイクルと比較できない。** `ProcessCameraProvider` の
   解決(初回のみ発生する非同期の初期化)がタイマー開始前ではなく `captureMs` の
   計測区間に一部含まれてしまうため、1サイクル目だけ実態より長く出る。
   **同じことがプロセス再起動直後の最初のサイクルにも当てはまる。** 分布の集計からは
   1サイクル目とプロセス再起動直後の1サイクルを除外すること(§6.1 のコマンド例には
   含めていないので、必要なら `seq` で除外して再計算する)
5. 15秒のタイムアウト用の遅延処理(Runnable)は、先着側が正常に完了しても
   キャンセルされず、後から無害に発火するだけの実装になっている。CSV には影響しないので
   解釈上は無視してよいが、logcat 上に「今さら何か起きた」ように見えるログが出ても
   異常ではない

---

## 7. 結果記入欄

計測後にここへ追記する。機種ごとに1セクション追加すること。

### 機種1

| 項目 | 値 |
| --- | --- |
| 機種名 | |
| Android バージョン | |
| 計測期間(開始〜終了、タイムゾーン) | |
| 送信間隔設定 | |
| §6.2 判定表のどれに該当したか | |
| 総行数 / 期待行数(約576) | |
| 欠測数(§6.3-1 の除外後) | |
| `source` 内訳(handler / alarm) | |
| `driftMs` p50 / p95 / max | |
| `batteryTempC` max / 45°C超の件数 | |
| `error` 内訳 | |
| `captureMs` / `jpegBytes` の傾向(1サイクル目・再起動直後を除く) | |
| 備考 | |

### 機種2(実施した場合)

| 項目 | 値 |
| --- | --- |
| 機種名 | |
| Android バージョン | |
| 計測期間(開始〜終了、タイムゾーン) | |
| 送信間隔設定 | |
| §6.2 判定表のどれに該当したか | |
| 総行数 / 期待行数(約576) | |
| 欠測数(§6.3-1 の除外後) | |
| `source` 内訳(handler / alarm) | |
| `driftMs` p50 / p95 / max | |
| `batteryTempC` max / 45°C超の件数 | |
| `error` 内訳 | |
| `captureMs` / `jpegBytes` の傾向(1サイクル目・再起動直後を除く) | |
| 備考 | |

### 総合判定

（§6.2 のどの行に最終的に落ち着いたか、複数機種の結果に差があった場合の解釈、
次に着手する開放課題を1〜2段落で記す）

---

## 8. 後始末

計測とその結果の記入(§7)が終わったら、次を行う。

1. **`rm -rf spike/android-fgs-timer` を実行し、検証コードを削除する。**
   このディレクトリは `.gitignore` 対象であり、そもそも一度も commit されていない。
   削除しても本書(このファイル)以外には何も失われない
2. **`docs/01-openquestion.md` の APP-04 を更新する。** §6.2 の判定結果に応じて、
   「実装レベルの選定が残る」という `pending` 状態から、カメラAPI(CameraX/Camera2)の
   選定結果を反映して `decided` に進める、または ❌判定だった場合はタイマー方式自体の
   再検討が必要である旨を追記する
3. **判定が ❌ または ⚠️(Alarm依存 / ドリフト大)だった場合は、`APP-01` も更新する。**
   送信間隔の初期値の議論に、実測の欠測率・ドリフト実績を反映する
4. **判定が ❌・⚠️・🔥 のいずれかだった場合は、`04-native.md` §3.0 自体の記述を修正する。**
   本書はあくまで検証記録であり、設計書本体の記述を実測に合わせて直す責任は
   `04-native.md` 側にある。本書を「そのうち直す根拠」として放置しないこと
5. **📷 Q4 の結果は `01-openquestion.md` APP-04(カメラAPI選定)と APP-03
   (画像仕様・4.5MB上限)の両方に反映する**
