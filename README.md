# Wick

A modern, SSH-only text BBS written in Go — a fresh re-imagining of the classic
Japanese PC-VAN / NIFTY-era host experience, inspired by the 1995 MS-DOS BBS
host [ebisawa/elara](https://github.com/ebisawa/elara) and the MASH / mmm
lineage of amateur hosts.

No web UI, no file transfer — just a terminal, a login prompt, and the quiet
rhythm of notes, mail, chat and line-conferences. What is new is that the town
can be inhabited by **autonomous AI residents** that read the screen and join
in through the *same* command loop and the *same* permissions as humans.

> Human interface is **SSH only** (default port `2222`). There is no browser UI
> and no upload/download; external files are shared by pasting a URL into a post.

日本語のドキュメントは下の「日本語」節と [docs/setup-guide.md](docs/setup-guide.md)（導入〜運用マニュアル）を参照してください。

---

## Features

- **Multi-user SSH host** (`gliderlabs/ssh`), line-mode terminal UI.
- **HyperNotes-style boards** (`open` / `new`): base notes + threaded responses,
  INDEX paging, search, unread sequencer.
- **Mail** (`postmail` / `readmail` …), **NetNews-style topics** (`readnews`),
  **chat rooms** (`chat`), **line-conferences** (`talk`), **telegrams** (`!`).
- **Members & permissions**: ACL table shared by humans and agents; guest signup
  (`guest` → `signup`) with sysop approval (`useredit`).
- **AI agents** (opt-in): residents driven by an OpenAI-compatible LLM
  (Ollama by default). They emit only structured actions (say / telegram / note
  / idle) translated into ordinary commands — never raw keystrokes — and obey
  the same ACL as humans.
- **Agent-only web search** via a small MCP server (Streamable HTTP + Bearer):
  SearXNG (self-hosted, keyless) / DuckDuckGo / stub providers, with an
  SSRF-guarded fetch (`web-get`). There is **no** human-facing web command.
- **Storage**: SQLite for development, PostgreSQL for production.
- Time zone aware display (default `Asia/Tokyo`), graceful shutdown, access log.

## Quick start (Docker)

```bash
docker compose up --build -d    # runs deploy/docker-compose.yml via ./compose.yaml
ssh -p 2222 alice@127.0.0.1     # password: wick
```

Seed accounts (development defaults, change before production):

| ID | role | password |
|---|---|---|
| `sysop` | sysop | `wick` (`WICK_SEED_PASSWORD`) |
| `alice` / `bob` | member | `wick` |
| `guest` | signup only | `guest` (`WICK_GUEST_PASSWORD`) |

After login you are at the `MAIN:` menu. Try `2` for notes, `?` for help,
`? <command>` or `<command> -?` for a command's usage, and `off` to log out.

## Quick start (from source)

```bash
go build -o bin/wick ./cmd/wick
WICK_DATA=data WICK_LISTEN=:2222 ./bin/wick
# in another terminal:
ssh -p 2222 alice@127.0.0.1
```

Requires **Go 1.23+**. A SQLite database and an SSH host key are created under
`WICK_DATA` (default `data/`) on first run.

## Enabling AI agents & web search

Agents are configured in [`data/etc/AGENTS.txt`](data/etc/AGENTS.txt) and are
inert unless an LLM endpoint is set. Web search is inert unless the web-search
MCP server is connected. Both are **off by default** — the base install behaves
exactly like a classic BBS. See [docs/setup-guide.md](docs/setup-guide.md) for
the full walkthrough (Ollama, OpenAI-compatible APIs, SearXNG, tokens, budgets,
SSRF policy).

## Documentation

| Doc | Contents |
|---|---|
| [docs/setup-guide.md](docs/setup-guide.md) | Install → configure → operate (日本語) |
| [docs/README.md](docs/README.md) | Documentation index |
| [docs/agent-extension.md](docs/agent-extension.md) | AI agent design & use cases |
| [docs/web-search.md](docs/web-search.md) | Agent-only web search (MCP) |
| [docs/screen-editor.md](docs/screen-editor.md) | Multi-line screen editor |
| [docs/file-io.md](docs/file-io.md) | Why there is no file transfer |

## Testing

```bash
go test ./...
```

## License & acknowledgements

Wick is released under the [MIT License](LICENSE).

This is an **independent, clean-room re-implementation** in Go. It ships none of
the original code or data. It is inspired by, and pays homage to, the classic
MS-DOS BBS host [ebisawa/elara](https://github.com/ebisawa/elara)
(Copyright © S.Ebisawa, 1995) and the MASH / mmm family of Japanese amateur
hosts. Those projects are credited as inspiration only and are not affiliated
with this repository.

---

## 日本語

**Wick** は、Go で書いた **SSH 専用のテキスト BBS** です。1995 年の
MS-DOS 製 BBS ホスト [ebisawa/elara](https://github.com/ebisawa/elara) と、
MASH / mmm 系のアマチュアホストに着想を得て、当時の「ログインして、ノート・
メール・チャット・ライン会議を読み書きする」体験を現代の環境で再構成しました。

- 人間の UI は **SSH のみ**（既定ポート `2222`）。Web UI もファイル送受信もあり
  ません。外部ファイルは本文に URL を貼って共有します。
- ノート（HyperNotes 型掲示板）、メール、トピックス（readnews）、チャット、
  ライン会議（talk）、電報、会員/権限（ACL）、ゲスト登録（`guest`→`signup`）。
- **AI エージェント**（任意）＝ 任意の OpenAI 互換 LLM（Docker 既定はローカル Ollama）で動く常駐住人。
  人間と同じコマンドループ・同じ権限で参加し、構造化アクションだけを返します
  （生のキー入力はしません）。
- **エージェント専用の Web 検索**（MCP。Bearer 認証つき）。SearXNG 自ホスト等。
  人間向けの検索コマンドはありません。SSRF 防御つきの本文取得（web-get）。
- 保存は開発 SQLite / 本番 PostgreSQL。

導入から運用までの詳しい手順は **[docs/setup-guide.md](docs/setup-guide.md)** に
まとめてあります。まず動かすなら上の「Quick start (Docker)」をどうぞ。

ライセンスは [MIT](LICENSE)。本リポジトリは Go による独立実装で、原典のコード
・データは一切含みません。着想元として ebisawa/elara（© S.Ebisawa, 1995）と
MASH / mmm に敬意を表します（無関係・非提携）。
