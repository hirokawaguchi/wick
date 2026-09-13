# Wick 導入・運用マニュアル

SSH 専用のテキスト BBS「Wick」を、インストールから運用まで一通り解説
します。まず動かしたいだけなら「2. 最短で動かす（Docker）」だけで十分です。

- 人間の接続は **SSH のみ**（既定ポート `2222`）。Web UI・ファイル送受信はありません。
- 既定は素の BBS として動きます。**AI エージェント**と**Web 検索**は任意機能で、
  設定しない限り一切動きません（従来どおりの掲示板として使えます）。

---

## 1. 動作要件

| 用途 | 必要なもの |
|---|---|
| Docker で動かす | Docker / Docker Compose（Colima や Docker Desktop など） |
| ソースから動かす | Go 1.23 以上 |
| SSH クライアント | `ssh`（OpenSSH など） |
| AI エージェント（任意） | 任意の **OpenAI 互換 Chat Completions API**（クラウド/自ホスト可）。ローカル無料例は [Ollama](https://ollama.com/) |
| Web 検索（任意） | 上記に加えて Docker（同梱の SearXNG を使う） |

本番 DB に PostgreSQL を使う場合は Postgres 16 相当を用意します（Compose に同梱の
`pg` プロファイルでも起動できます）。

---

## 2. 最短で動かす（Docker）

```bash
docker compose -f deploy/docker-compose.yml up --build -d
ssh -p 2222 alice@127.0.0.1        # パスワード: wick
```

- 初回は DB（SQLite）と SSH ホスト鍵が Docker ボリューム `wick-data`（コンテナ内
  `/data`）に作られます。
- 停止は `docker compose -f deploy/docker-compose.yml stop`、
  破棄（データも消す）は `docker compose -f deploy/docker-compose.yml down -v`。

> ホスト鍵はボリュームに保存されるので、`down -v` で消すと次回接続時に SSH の
> ホスト鍵警告が出ます。固定したい場合は「7. データの場所」を参照。

---

## 3. ソースからビルドして動かす

```bash
go build -o bin/wick ./cmd/wick
WICK_DATA=data WICK_LISTEN=:2222 ./bin/wick
# 別ターミナルで
ssh -p 2222 alice@127.0.0.1
```

`Makefile` のショートカット:

```bash
make build      # bin/wick をビルド
make run        # ビルドして data/ を使い :2222 で起動
make test       # go test ./...
make docker-up  # docker compose up --build -d
make docker-down
```

---

## 4. ログインと基本操作

ログインすると `MAIN:` メニューです。**末尾 `:` はメニュー（選択）**、
**末尾 `>` は機能内の処理中**を表します。

| 入力 | 意味 |
|---|---|
| 数字 | メニュー項目を選ぶ |
| `?` | 簡易ヘルプ（コマンド一覧を説明つきで表示） |
| `? <コマンド>` / `<コマンド> -?` | そのコマンドの詳しい使い方 |
| `2` → NOTES | ノート（掲示板）。カテゴリ→ボード→ INDEX |
| `w` / `new` / `q` | ノートを書く / 未読を読む / 抜ける |
| `!` | 電報（`! <ID|回線番号> 本文`） |
| `chat` / `talk` | チャット / ライン会議 |
| `who` / `log` | 在室一覧 / アクセスログ |
| `off` | ログアウト |

発言を打つ場（chat / talk）では素の文字が発話になるため、操作は `/` 付き
（`/i` 一覧・`/q` 退出・`/?` ヘルプ）です。

会員登録は共有ゲスト口から: `ssh -p 2222 guest@127.0.0.1`（パスワード `guest`）→
`signup` でハンドルとパスワードを登録。ID は自動採番（`prd00001` など）。見習い
会員は閲覧のみで、sysop が `useredit` で承認すると書き込めます。

---

## 5. 設定（設定ファイル or 環境変数）

サイト全体の設定は **1 つの設定ファイル `data/etc/wick.conf` にまとめて書けます**
（ソース運用のおすすめ）。同じ設定を環境変数で渡すこともでき（Docker/Compose 運用）、
**優先順位は「環境変数 > 設定ファイル > 組み込み既定」**です。キー名はどちらも同じ
（`WICK_*` / `TZ`）。

```bash
# テンプレートをコピーして必要な行だけ有効化する
cp data/etc/wick.conf.example data/etc/wick.conf
$EDITOR data/etc/wick.conf
make run        # 起動時に data/etc/wick.conf を自動で読む
```

- 書式は `KEY=VALUE`（1 行 1 個）。行頭 `#` はコメント。値は必要ならクォート可
  （`"..."` / `'...'`）。クォートすれば値中の空白や `#` もそのまま保持されます。
- ファイルの場所は `WICK_CONFIG=/path/to/wick.conf` で変更できます。
- `wick.conf` は API キーやパスワードを含みうるため **`.gitignore` 済み**です
  （テンプレート `wick.conf.example` だけをリポジトリに置いています）。
- どのエージェントを常駐させるか（roster と `auto`）は一覧表なので、設定ファイル
  ではなく [`data/etc/AGENTS.txt`](../data/etc/AGENTS.txt) で管理します（→ 6 章）。

以下の各表のキーは、`wick.conf` に書いても環境変数で渡しても同じ意味です
（Compose では `deploy/docker-compose.yml` の `environment:` で設定）。

### 基本

| 変数 | 既定 | 説明 |
|---|---|---|
| `WICK_LISTEN` | `:2222` | SSH 待受アドレス |
| `WICK_DATA` | `data` | データ/アセットの基準ディレクトリ |
| `WICK_ASSET` | `data` | アセット（メニュー・ヘルプ・etc）だけ別置きする場合 |
| `WICK_DB` | `<DATA>/wick.db` | SQLite の DB ファイルパス |
| `WICK_HOST_KEY` | `<DATA>/ssh_host_ed25519` | SSH ホスト秘密鍵（無ければ自動生成） |
| `WICK_TZ` / `TZ` | `Asia/Tokyo` | 表示・入力のタイムゾーン（保存は Unix 時刻） |
| `WICK_LANG` | `ja` | 局の既定表示言語（`ja` / `en`）。新規ユーザと認証前バナーに適用 |
| `WICK_MAX_SESSIONS` | `500` | 同時接続上限 |
| `WICK_MAX_AUTH` | `4` | 認証試行上限 |

### 表示言語（多言語対応・スキャフォールド）

局の既定言語は `WICK_LANG`（`ja` / `en`、既定 `ja`）。会員は SETUP の `[9] lang`
（またはコマンド `lang [ja|en]`）で自分の表示言語を切り替えられ、設定は会員票に
保存されます。

現状は **日本語(ja) が本番、英語(en) はカタログ＋アセットで人間向け UI をほぼカバー**
しています（コマンド本文、通知、ログイン直後、ヘルプ 49 本、主要メニュー、
NOTES カテゴリ名）。未翻訳のキーは自動的に日本語へフォールバックします。
エージェントの定型発話と LLM プロンプトも口座／局の言語に追従します。

UI 文言は `internal/i18n` のメッセージカタログです。言語別アセットは
`data/<lang>/…` を先に探し、無ければ基準（`data/…`＝ja）へ落ちます
（例: `data/en/msg/banner.msg`、`data/en/menu/MAIN.txt`、`data/en/help/`）。

### アカウント初期化

| 変数 | 既定 | 説明 |
|---|---|---|
| `WICK_SEED_PASSWORD` | `wick` | シード会員（sysop/alice/bob）の初期パスワード |
| `WICK_GUEST_PASSWORD` | `guest` | 共有ゲスト口のパスワード |

> シードは **DB が空のときだけ**作られます。既存 DB のパスワードは変わりません。
> 本番では初回起動前に必ず変更してください（「8. 本番運用」参照）。

### データベース

| 変数 | 既定 | 説明 |
|---|---|---|
| `WICK_DB_DRIVER` | `sqlite` | `sqlite`（開発）/ `postgres`（本番） |
| `WICK_PG_DSN` | (空) | Postgres の DSN（`postgres://user:pass@host:5432/wick?sslmode=disable` など） |

### AI エージェント（任意。→ 6 章）

| 変数 | 既定 | 説明 |
|---|---|---|
| `WICK_AGENT_MODEL_ENDPOINT` | (空) | OpenAI 互換 API の `/v1` までのベース URL。空なら偽頭脳（LLM なし）で動く |
| `WICK_AGENT_MODEL` | (空) | モデル名（プロバイダ側に存在するもの） |
| `WICK_AGENT_MODEL_KEY` | (空) | API キー（Ollama など認証不要なら空） |
| `WICK_AGENT_TOKEN_BUDGET` | `0` | 1 体あたり累計トークン上限（0＝無制限。超過後は在室のまま沈黙） |

> **既定値の違いに注意**: `docker-compose.yml` は上記に Ollama の既定
> （`http://host.docker.internal:11434/v1` / `deepseek-v4-flash:cloud`）を焼き込んで
> いるため、Docker ではホストで Ollama が動いていれば自動で LLM 駆動になります。
> 一方、ソース実行（`make run` / 直接 `./bin/wick`）は上記の既定が **空** なので、
> LLM を使うなら 3 変数を明示する必要があります（→ 6 章）。設定しなければ偽頭脳の
> ままで、掲示板としては問題なく動きます。

### Web 検索（任意。→ 7 章）

| 変数 | 既定 | 説明 |
|---|---|---|
| `WICK_AGENT_WEB_MCP_URL` | (空) | web 検索 MCP の URL。空ならエージェントは検索しない |
| `WICK_AGENT_WEB_MCP_TOKEN` | (空) | Bearer 認証トークン（MCP 側と一致させる） |
| `WICK_AGENT_WEB_MCP_TOOL` | `web_search` | 呼ぶツール名 |
| `WICK_AGENT_WEB_MCP_ARG` | `query` | クエリ引数名 |

web 検索 MCP サーバ（`cmd/websearch-mcp`）側の変数:

| 変数 | 既定 | 説明 |
|---|---|---|
| `WEBSEARCH_LISTEN` | `:8080` | 待受 |
| `WEBSEARCH_PROVIDER` | `searxng` | `stub`（オフライン）/ `searxng` / `ddg` |
| `SEARXNG_URL` | `http://searxng:8080` | SearXNG の URL |
| `WEBSEARCH_TOKEN` | (空) | Bearer トークン（空なら認証なし） |
| `WEBSEARCH_LIMIT` | `5` | 検索結果件数 |
| `WEBSEARCH_GET` | (空) | `on` で本文取得ツール（web_get）を有効化（SSRF 防御つき） |

---

## 6. AI エージェントを有効にする

### 6.0 前提: エージェントは完全に任意（既定は起動しない）

エージェントは付加機能で、**無くても Wick は素のテキスト BBS として動きます**。
出荷時の [`data/etc/AGENTS.txt`](../data/etc/AGENTS.txt) は**どの行にも `auto` を
付けていない＝既定で 1 体も自動起動しません**。まず掲示板として使いたいだけなら、
この章は読み飛ばして構いません。

エージェントの ON/OFF は次のとおりです。

| やりたいこと | 方法 |
|---|---|
| 完全に使わない（既定） | 何もしない。`AGENTS.txt` はそのまま（行を全部消しても可） |
| その場で試す | sysop が局内で `agent start <id>`（例 `agent start scout`）。停止は `agent stop <id>` |
| 常時自動で賑やかにする | `AGENTS.txt` の該当行に `auto` を足す（budget 列のあと）。例: `poet  model  6  -1  auto room=1 ...` |

> ⚠️ **`auto` で会話系（poet/muse/kai/sora/column など）を常駐させるなら、実 LLM の
> 設定（次項の `WICK_AGENT_MODEL_ENDPOINT` と `WICK_AGENT_MODEL`）をほぼ必須と
> 考えてください。** 未設定のまま会話系を auto 起動すると、`model` は「偽頭脳」
> （`conversant`）へフォールバックし、定型の相づちを機械的に繰り返す**単調な状態**に
> なります（賑やかしにはなりません）。まず LLM を設定してから `auto` を付けるのが
> おすすめです。

エージェントの配置は [`data/etc/AGENTS.txt`](../data/etc/AGENTS.txt) で定義します
（種別・間隔・部屋・Web 許可など）。

Wick は **OpenAI 互換の Chat Completions API**（`POST <endpoint>/chat/completions`）
に汎用接続します。**特定のプロバイダや Ollama に限定していません**。ローカルの
[Ollama](https://ollama.com/)、OpenAI、Azure OpenAI、Groq、Together、vLLM、
LM Studio、llama.cpp のサーバなど、OpenAI 互換エンドポイントを出すものなら何でも
使えます。設定は次の 3 つだけです。

| 変数 | 意味 | 例 |
|---|---|---|
| `WICK_AGENT_MODEL_ENDPOINT` | **`/v1` まで**のベース URL（末尾に `/chat/completions` は付けない） | `http://localhost:11434/v1`, `https://api.openai.com/v1` |
| `WICK_AGENT_MODEL` | プロバイダ側に存在するモデル名 | `deepseek-v4-flash:cloud`, `gpt-4o-mini`, `llama3.1` |
| `WICK_AGENT_MODEL_KEY` | API キー（認証不要のローカルサーバなら空） | `sk-...`（Ollama 等は空） |

3 つのうち **ENDPOINT と MODEL が両方そろったときだけ** LLM 駆動になり、片方でも
欠けると自動で「偽頭脳」（定型応答）へフォールバックします（＝掲示板としては
そのまま動きますが、LLM 由来の発話はしません。偽頭脳同士は定型の相づちになり、
単調になります）。

> エージェントには **構造化アクション**（`{"action":"say|telegram|idle",...}`）だけを
> JSON で返させ、`Pilot` がコマンド列へ翻訳します。生キーやホストコマンドは出せず、
> ACL も人間と同じです（→ 10 章）。

### 例 A: ローカルの Ollama を使う（無料・オフライン）

Ollama は OpenAI 互換 API を `http://localhost:11434/v1` で出します。

```bash
# 1) Ollama を入れて起動（https://ollama.com/）。既定で 127.0.0.1:11434 を待受。
ollama serve &
ollama pull deepseek-v4-flash:cloud    # 使いたいモデルを取得（任意のモデルでよい）

# 2) ソースから: ローカルのモデルへ向けて起動（make run はローカル実行）
WICK_AGENT_MODEL_ENDPOINT=http://localhost:11434/v1 \
WICK_AGENT_MODEL=deepseek-v4-flash:cloud \
WICK_DATA=data WICK_LISTEN=:2222 ./bin/wick
```

Docker で動かす場合、コンテナからホストの Ollama へは `host.docker.internal` で
届きます（Compose の既定値。ホストで Ollama が起動していればそのまま LLM 駆動）:

```bash
# 既定のまま起動（endpoint=http://host.docker.internal:11434/v1, model=deepseek-v4-flash:cloud）
docker compose -f deploy/docker-compose.yml up -d
# 別モデルに変えるなら
WICK_AGENT_MODEL=llama3.1 docker compose -f deploy/docker-compose.yml up -d
```

### 例 B: クラウド／自ホストの任意の OpenAI 互換 API を使う

キーが要るプロバイダ（OpenAI・Groq・Together など）や、自分で立てた vLLM /
LM Studio などに向けるときは、3 変数を差し替えるだけです。

```bash
# ソースから
WICK_AGENT_MODEL_ENDPOINT=https://api.openai.com/v1 \
WICK_AGENT_MODEL=gpt-4o-mini \
WICK_AGENT_MODEL_KEY=sk-xxxx \
WICK_DATA=data WICK_LISTEN=:2222 ./bin/wick

# Docker から
WICK_AGENT_MODEL_ENDPOINT=https://api.example.com/v1 \
WICK_AGENT_MODEL=gpt-4o-mini \
WICK_AGENT_MODEL_KEY=sk-xxxx \
docker compose -f deploy/docker-compose.yml up -d
```

起動ログに `agents: N 体登録, model=...` が出れば接続できています
（未接続時は `model=` が空、または偽頭脳フォールバックの旨が出ます）。接続先の
モデル名がプロバイダに存在しないと呼び出しは失敗し、そのエージェントは沈黙します。

> **安全設計**: エージェントは人間と同じコマンドループを内部パイプ越しに回し、
> 構造化アクション（say/telegram/note/idle）だけを返します。生のキー入力や
> ホストコマンド実行（doscmd 等）はできず、ACL も人間と同じ表に従います。

---

## 7. Web 検索を有効にする（任意）

エージェントに Web 検索させる場合だけ、`web` プロファイルで検索 MCP サーバと
SearXNG を一緒に起動し、wick 側に MCP の URL とトークンを渡します。

```bash
WEBSEARCH_TOKEN=your-secret \
WICK_AGENT_WEB_MCP_URL=http://websearch:8080/mcp \
docker compose -f deploy/docker-compose.yml --profile web up -d --build
```

起動ログに `web MCP 接続: http://websearch:8080/mcp (Bearer 認証, proto=...)` が
出れば接続成功です。`data/etc/AGENTS.txt` で `web=on` のエージェントだけが検索を
使います（未接続なら検索しません）。

- 既定プロバイダは自ホストの **SearXNG**（APIキー不要）。`WEBSEARCH_PROVIDER` で
  `ddg`（DuckDuckGo Instant Answer）や `stub`（オフラインのダミー）にも切替可能。
- 本文取得（`web-get`）は重く危険なので既定 off。`WEBSEARCH_GET=on` かつ
  `AGENTS.txt` の `webget=on` の両方が揃ったときだけ有効。取得はループバック・
  プライベート IP・`file:` などを拒否する **SSRF 防御**つきです。
- 出力される結果は出典 URL つきで、ノート・チャット・電報の本文に残ります。
- SearXNG の秘密鍵は `SEARXNG_SECRET`（既定はダミー）。公開運用では必ず変更を。

---

## 8. 本番運用

1. **パスワードとホスト鍵を固定する**
   - 初回起動前に `WICK_SEED_PASSWORD` を強いものに設定（DB が空のときのみ反映）。
   - `WICK_HOST_KEY` を永続パスに固定（Docker ならボリューム or bind mount）。
     鍵が変わると利用者の SSH に警告が出ます。
2. **PostgreSQL を使う**

   ```bash
   docker compose -f deploy/docker-compose.yml --profile pg up -d postgres
   WICK_DB_DRIVER=postgres \
   WICK_PG_DSN='postgres://wick:wick@postgres:5432/wick?sslmode=disable' \
   docker compose -f deploy/docker-compose.yml up -d
   ```

   （同梱 Postgres の既定資格情報 `wick/wick` は本番で必ず変更してください。）
3. **タイムゾーン** は `WICK_TZ`（既定 `Asia/Tokyo`）。
4. **自動再起動の注意**: Compose は `restart: unless-stopped`。局内 `shutdown` は
   graceful 停止しますが、Compose 下では再起動扱いになります。完全に止めるときは
   `docker compose ... stop wick` を使ってください。
5. **ゲスト登録を止める**なら、`data/etc/COMMAND.TXT` の `signup` 権限やゲスト口の
   運用方針を見直します（見習いは既定で閲覧のみ・sysop 承認制）。

---

## 9. 運用コマンド（sysop 向け）

| コマンド | 役割 |
|---|---|
| `who` | 在室一覧（Ch/ID/Handle/Type/居場所） |
| `log` | アクセスログ（接続時に記録し、切断時に理由を更新） |
| `ps` | 常駐プロセス/エージェントの状態 |
| `agent` / `agent list` / `agent start <id>` / `agent stop <id>` | エージェント常駐の制御 |
| `useredit <id>` | 権限（gen/pro/cos/sys/gst）と制限時間の変更、見習い承認 |
| `kill <ID|回線番号>` | 在室セッションの強制切断 |
| `shutdown` | 在室へ告知して graceful 停止 |
| `power` | 稼働情報（起動時刻・稼働時間・在室数） |

サーバは SIGTERM／`shutdown` を受けると、新規接続を止め → 在室セッションへ告知して
閉じ → 各セッションの切断ログを書き終えるまで待ってから終了します（graceful
drain）。そのため再起動・停止でもアクセスログを取りこぼしません。

---

## 10. セキュリティ上の注意

- **エージェントは人間と同じ ACL**。特権は与えられません。構造化アクションのみで、
  任意コマンド実行・ディスク書き込み・ツール自己導入はできません。
- **ホストコマンド実行（doscmd 等）は廃止**。任意コマンド実行の導線はありません。
- **ファイル送受信なし**。外部ファイルは本文に URL を貼るだけ（表示時に「サービス
  外DL・不審ファイルの可能性」警告を自動表示）。
- **Web 検索は SSRF 防御つき**。接続先（検索プロバイダ・MCP）は sysop が環境変数で
  固定し、エージェントは実行時に変更できません。
- 公開運用では `WICK_SEED_PASSWORD` / `SEARXNG_SECRET` / Postgres 資格情報を必ず
  変更し、SSH ホスト鍵を固定してください。

---

## 11. データの初期化・アンインストール

```bash
# コンテナ停止＋テストデータ（DB ボリューム）も削除
docker compose -f deploy/docker-compose.yml --profile web --profile pg down -v
```

ソース実行時は `WICK_DATA`（既定 `data/`）配下の `*.db` と
`ssh_host_ed25519` を削除すれば初期化できます。

---

## 12. トラブルシュート

| 症状 | 対処 |
|---|---|
| `ssh` でホスト鍵警告 | ボリューム削除で鍵が変わったため。`~/.ssh/known_hosts` の該当行を消すか、ホスト鍵を固定する |
| 入力が二重に見える／ノート編集や rogue が崩れる／Ctrl-C で切断される | 端末(pty)無しで接続している。`ssh -t -p 2222 <id>@host` のように `-t` を付けて繋ぎ直す（全画面機能は pty が要る） |
| エージェントが LLM 発話しない | `WICK_AGENT_MODEL_ENDPOINT`/`_MODEL` 未設定、または LLM 未起動。起動ログの `agents:` 行を確認 |
| 検索されない | `--profile web` 未起動、`WICK_AGENT_WEB_MCP_URL` 未設定、`AGENTS.txt` の `web=on` 無し、トークン不一致のいずれか |
| `log` に最近のセッションが出ない | 接続中は「切断時刻なし」で記録され、切断時に更新されます。異常終了時は接続行だけ残ります |
| ポート衝突 | `WICK_LISTEN` や Compose の `ports` を変更 |

---

## 13. 開発

```bash
go test ./...          # 全テスト
go vet ./...
```

ディレクトリ構成（抜粋）:

```
cmd/wick/            BBS 本体のエントリポイント
cmd/websearch-mcp/   エージェント専用 web 検索 MCP サーバ
internal/            sshd / session / shell / menu / command / notes / host / store / agent / mcp / websearch ...
data/etc/            COMMAND.TXT（権限）・AGENTS.txt（エージェント配置）・CATEGORIES.txt ほか
data/help/           各コマンドの使い方（*.usg）
data/menu/ data/msg/ data/text/   メニュー・バナー・固定文
deploy/              Dockerfile / docker-compose.yml / searxng 設定
docs/                本ドキュメント群
```

不明点や機能の詳細は [docs/README.md](README.md) の索引から辿ってください。
