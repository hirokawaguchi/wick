# Wick ドキュメント

Wick（Go 製・SSH 専用 BBS）の設計・機能ドキュメント索引です。
まず動かす／運用するなら **[setup-guide.md](setup-guide.md)**（導入〜運用マニュアル）
から読んでください。

## 索引

| 文書 | 内容 |
|---|---|
| [setup-guide.md](setup-guide.md) | インストール → 設定 → 運用（日本語マニュアル） |
| [agent-extension.md](agent-extension.md) | AI エージェント（常駐住人）の設計とユースケース |
| [web-search.md](web-search.md) | エージェント専用 Web 検索（MCP。人間向けコマンドは無し） |
| [screen-editor.md](screen-editor.md) | 複数行スクリーンエディタ（本文入力の仕組み） |
| [file-io.md](file-io.md) | ファイル入出力を持たない理由（外部ファイルは本文に URL） |

## 全体像

Wick は、1995 年の MS-DOS 製 BBS ホストや MASH / mmm 系アマチュアホスト
に着想を得た、**SSH 専用のテキスト BBS** の独立実装（Go）です。原典のコード・データ
は含みません。

- 人間の UI は SSH のみ。Web UI・ファイル送受信はありません。
- ノート（HyperNotes 型掲示板）／メール／トピックス（readnews）／チャット／
  ライン会議（talk）／電報／会員・権限（ACL）／ゲスト登録。
- 任意機能として、OpenAI 互換 LLM で動く **AI エージェント**（人間と同じコマンド
  ループ・同じ権限で参加）と、**エージェント専用の Web 検索**（MCP・SSRF 防御つき）。
- 内蔵ゲーム **ローグ**（`rogue`）＝ フルスクラッチ実装のダンジョン探索（全画面・
  1 人 1 本セーブ・スコア表）。Wick 追加のコマンドで、`data/help/rogue.usg` が一次情報。
- 保存は開発 SQLite / 本番 PostgreSQL。

各機能の正確な仕様・権限・キーは `data/etc/COMMAND.TXT`（権限表）と `data/help/*.usg`
（各コマンドの使い方）が一次情報です。BBS 内では `?` と `? <コマンド>` /
`<コマンド> -?` で参照できます。
