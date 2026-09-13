# 実装順

移植の段階を、今書くコードの順に落としたもの。

## いまやること

エージェント連携（Web 検索はその内部ツールとして扱う。人間向けの Web コマンドは作らない）。

## できたこと

### Phase 1 — 接続とログイン

Go モジュール、SQLite Store、SSH（パスワード＝BBS 認証）、二重ログイン拒否、制限時間、切断ログ、Docker Compose port 2222。

### Phase 2 — シェルと権限

メニュー階層（番号、`;`、`.` `/`）、COMMAND / OPTION / FLAG、alias、`?` / `-?`。
`who` `version` `echo` `expert` `handle` `password` `terminal` `private` `scanlist` `regsign` `setseque`。MAIN → SETUP で自己情報を変えられる。権限の無いコマンドは拒否。

### Phase 3 — HyperNotes

ボード、`open` の INDEX/OPEN、投稿・削除・Important・Close・AWO、シーケンサ、`new`。
INDEX は 20 件ずつ（Space/BS/=/* で頁送り、`f` 検索、`A` 時刻未読、`k` 看板）。看板は `board`（sys/cos）の `[7] sign`。NOTES は `カテゴリ.名前` で階層（NOTES → junk → junk.test）。Store は SQLite と PostgreSQL（`WICK_DB_DRIVER=postgres`）。

### Phase 4 — 電報とチャット

`! id 本文` または `! チャネル 本文` でオンラインへ電報。`who` の先頭がチャネル番号。入力待ちに割り込む。
`chat` で部屋一覧・入室。`.` / `q` で退出。`/who` `/title` `/?` `/e`（エコーバック切替、仮）。

`talk` はライン会議。ログは残る。`talk -n` が未読巡回、`roomlist` が在室と進行 line。部屋内キー（`/knock` `/seat` `/open` など）は仮。

人間の UI は SSH のみ。ブラウザは無い。

### Phase 5 — 仮想ライブラリ（廃止）

当初は SFTP と仮想ライブラリ（`file` / `filer` / `pfile` / `upload` / `download` / `type`）でファイルを扱っていたが、Phase 9 で**全廃**した（下記）。この節は履歴。

### Phase 6 — メールと公開プロフィール

`postmail` `readmail` `deletema` `killmail` `lookreco` `multipos` `regmbox` `reggroup`。本文は Store。バイナリメール送信（`postmail -b`）は撤去（ファイル入出力全廃のため）。
公開は `regprof` `profile` `readprof` `searchprof`。`private` の本名住所は出さない。`pfile` は個人箱（`file ls`）。
ログイン時に未読通数。フルネーム alias（`deletemail` 等）。

### Phase 7 — トピックス（news）

局内グループの `readnews` / `postnews`。操作は UNIX `readnews` の部分集合。`-n` ノンストップ（MD）、`-l` 見出しだけ、`-c` 有無だけ。部屋内キーは `n`/`+`/`;`（次）、`N [group]`（次／指定グループ）、`U`（購読解除）、`r`（著者へメール）、`s [file]`（箱へ保存）、`q`/`x`、`f`（フォローアップ）。既定グループ `local`。投稿は `sys` / `cos` のみ。NNTP は無い。

### 補い（0.7.1・場は増やさない）

出典のある「動いているが欠けている」を埋めた。INDEX の頁送り・検索・看板・`A`、news の UNIX 残り（`-l` `-c` `U` `N group` `r` `s`）、chat の `/e`。正本はコマンド仕様（内部設計メモ）。出典の無い `talk` 本キー・`pack`・OPEN からのメールは触っていない。

### Phase 8 — オンライン登録（0.8.0）

共有ゲスト口 `guest`（環境変数 `WICK_GUEST_PASSWORD`、既定 `guest`）でログインすると、`signup` と `off` だけの専用ループに入る（MAIN も他メニューも見せない）。`signup` はハンドルとパスワードを取り、**ログイン ID は登録順に自動採番**して見習い会員（FLAG bit12 = `pro`）を作る（昔の BBS 流にユーザは ID を選べない）。ID は 8 桁固定＝プレフィックス `prd` ＋ 5 桁ゼロ詰め連番（例 `prd00001`、小文字）で、`store.AllocMemberID` が同プレフィックスの最大番号＋1 を払い出す（同時登録の衝突は `CreateUser` の一意制約で検出し採番からやり直す）。人に見える名前はハンドル（自由入力）で担保する。見習いは閲覧のみで、投稿・送信・添付・公開文は不可（COMMAND.TXT の `pro` 列と、ボードの write/basenote マスクから `pro`/`gst` を外して二重に防ぐ）。sysop / co-sysop は `useredit` でレベルと制限時間を変え、見習いを `gen` に承認する。二重ロック（ゲスト専用ループ＋ ACL）と承認制で、不正登録の実害を抑える。出典未取得の Wick 追加として内部設計メモに理由を残した。
接続前（認証前）に SSH バナーで登録手順を案内する。文面は `data/msg/banner.msg`（無ければ組み込みの既定文）。

### Phase 9 — ファイル入出力の全廃と外部リンク警告（0.9.0）

この局ではファイルの送受信を一切行わない。SFTP サブシステムと `internal/files` ライブラリ、`file` / `filer` / `pfile` / `upload` / `download` / `type` コマンドを撤去した。MAIN [5] filer・PPROF pfile・SYSTEM type もメニューから外し、`whoami` / `time` は `type` エイリアスから単独コマンドへ移した。readnews の `s`（保存）も廃止。旧 `attachments` テーブルは定義ごと削除（`DROP TABLE IF EXISTS` で既存 DB も掃除）。

外部ファイルを共有したいときは、外部にアップした **URL を本文にそのまま書く**（ノート・レス・メール・news）。本文に `http(s)://` を含むメッセージを表示するとき、`internal/warn` が「外部サイトからのDLで不審ファイルの可能性がある」注意書きを一度だけ自動で添える。URL がテキストで出るだけなら入出力機構は不要、という判断。正本はコマンド仕様（内部設計メモ）。

### エージェント自律参加フレームワーク（フレームワーク先行／パイプ駆動）

方針転換：**Wick 内で人間と同じように自律動作するエージェントを内包する**（[agent-extension.md](agent-extension.md) §6.1）。モデルは未接続、頭脳は差し替え可能な偽頭脳。
- **AgentIO**：`session.NewPipe` がエージェント用セッションを作る。入力はパイプ（`Feed`）、出力は非ブロッキング観測シンク（`Snapshot`/`Drain`、上限付き）。`Print` が固まらない。`Close` で入力を閉じ EOF 終了。`Kind`（human/agent）で `who`/`ps` に種別（人／AI）表示。
- **Pilot/Manager**（`internal/agent`）：`Pilot` が人間と同じ `menu.Engine.Enter(MAIN)` をゴルーチンで回し、心拍ごとに `Brain` の返すキー列を `Feed` する。`Manager` が Start/Stop/List を持ち `command.AgentControl` として `Env` に注入。
- **Brain**：`Next(obs) (script, done)`。偽頭脳は `greeter`（電報→メール→ノート）と `chatter`（チャット参加）。実モデルはこの実装を差し替えるだけ。
- **配置**：`data/etc/AGENTS.txt`（1 行 1 体）。口座は `Store.SeedAgents`（`gen|agt`、Expert=2、時間無制限）。`agt`＝`FLAG.TXT` bit11。
- **起動口** `agent`（SYSTEM [6]・sys/cos）：`agent`/`agent list`・`agent start <id>`・`agent stop <id>`。
- **置き場**：`host` は `EnterAgent`/`LeaveAgent`（presence）だけ。オーケストレーションは `internal/agent`。`host`/`command` は `agent` を import しない（循環回避）。
- ACL・未読・doscmd 不可・Web 直接操作なしは人間と同じ。実モデル接続・floor・予算・ジョブ表は未実装。

## そのあと

| 順 | 内容 |
|---|---|
| 次 | 実 `Brain`（モデル接続）、floor／連続ターン上限／トークン予算、ジョブ表、常駐ラウンジの自律会話、`web` ツール |
