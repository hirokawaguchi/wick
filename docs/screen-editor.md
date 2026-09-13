# スクリーンエディタ（複数行本文エディタ） 設計メモ

状態: **検討中（未実装）**。`edit` / `textedit` の実体をこれにする案。行エディタ（`l`/`a`/`i`/`d`/`w`）は採らない。

## 1. ねらい

今の本文入力は「1 行ずつ打ち、`.` 単独行で確定」。改行した後は前の行に戻れない（打ち直しになる）。
これを、**カーソルを前の行へ戻して直せる**複数行エディタにする。使い勝手は現状のまま（`.` 終端は据え置き）で、行間・行内移動と挿入/削除を足すだけ、を目標にする。

- 対象は「本文（複数行）」の入力すべて：ノート基文/レス、メール、news、プロフィール、autosign、talk など今の `.` ループ。
- 端末は SSH pty。サーバ側で 1 文字ずつ受け、エコーとカーソル制御を自前で行う（`readRune` が既に 1 文字単位なので土台はある）。

## 2. キー（仮。実装時に確定して「ずらしたもの」へ）

| キー | 動作 |
|---|---|
| 文字 | カーソル位置に挿入 |
| Enter / Shift+Enter | 常に改行（カーソル位置で行分割／末尾なら新規行）。**送信はしない** |
| `.` 単独行 | **送信（確定）。唯一の送信手段** |
| ↑ / ↓ | 前後の行へ（桁は保持、行が短ければ行末へ） |
| ← / → | 行内を左右へ。行頭で←＝前行末、行末で→＝次行頭 |
| Backspace(0x7f/0x08) | カーソル左を削除。行頭なら前行と連結 |
| Ctrl-A / Ctrl-E | 行頭 / 行末（案） |
| Ctrl-C | 破棄して中止（案） |

- **送信は `.` 単独行だけ**。Enter は誤爆防止のため一切送信に使わない（改行専用）。
- 端末は Enter と Shift+Enter を区別できない（どちらも CR/LF が届く）ので、すべての Enter 系＝改行、で自然に要件を満たす。特別扱い不要。
- Ctrl-D も送信には割り当てない（`.` に一本化）。中止だけ Ctrl-C（案）。
- 方向キーは端末から `ESC [ A/B/C/D`。Home/End は `ESC [ H/F` や `ESC [ 1~ / 4~` など端末差があるため、まずは矢印＋Ctrl-A/E に絞る。

## 3. 端末制御で使う ANSI（最小）

- カーソル移動: `ESC[<n>A` 上, `ESC[<n>B` 下, `ESC[<n>C` 右, `ESC[<n>D` 左, `ESC[<col>G` 桁指定
- 行クリア: `ESC[K`（カーソルから行末まで）
- 桁数は `session.User.TermWidth`（terminal コマンドで設定済み）。0 のときは 80。
- 行数（高さ）は現状未取得。まずは「編集領域は入力開始行から下へ伸びる」前提で、画面高さに依存しない再描画（相対移動）にする。スクロールが要る長文は将来対応。

## 4. 入力読みの拡張

`readRune` はエスケープを素通しし、`ReadKey` は `ESC[` 系を読み捨てている。エディタ用に**キーコードを返す読み**を足す。

- 新設 `session.ReadEditKey() (Key, error)`：通常ルーン / Enter / Backspace / 矢印 / Ctrl-x を区別して返す。
- 既存の `readLine` / `ReadKey` はそのまま（メニュー・確認・パスワードは現状維持）。

## 5. 公開 API（案）

```
// EditText は複数行本文をスクリーン編集で読む。init は既存本文（新規は空）。
// 戻り値 ok=false は破棄（Ctrl-C）。確定時は "行\n..." を返す。
func (s *Session) EditText(init string, maxLineLen int) (text string, ok bool, err error)
```

- 既存の `.` ループ呼び出し（notes/mail/news/pprof/setup/talk）を段階的に `EditText` に置換。
- expert 設定やユーザーの端末能力で「簡易（従来 `.` ループ）／スクリーン」を切替える余地を残す（VT 非対応端末の保険）。

## 6. 段階実装

1. ✅ `ReadEditKey` とキー型を追加（`internal/session/edit.go`。ESC シーケンス分解＝矢印・Home/End）。
2. ✅ `EditText` を実装（バッファ＝`[][]rune`、カーソル=行/桁、`ESC[J` で相対再描画）。テストは `strings.Reader` にバイト列を流して buffer 結果を検証（`edit_test.go`）。
3. ✅ **ノート基文/レス（`internal/notes/open.go editBody`）** を差し替え済み。
4. ✅ 横展開済み。本文 compose は共通 `command.composeBody`（mail 作成・mail 返信・news 投稿）に集約。プロフィール/署名/看板は現在値を初期表示して編集できる `command.editField`（`cmdRegprof`・`cmdRegsign`・`readSign`）へ。
   - talk のライン発言・電報・チャットは**単行入力**なので対象外（エディタ化しない）。
5. ✅ 単独の `edit`/`textedit` コマンドは**廃止**（COMMAND.TXT から削除）。編集は各 compose（ノート・mail・news・プロフィール等）の中でエディタが開くので、独立した入口は設けない。

## 7. 非目標（今回入れない）

- 全画面スクロール・ページング（長文の画面追従）。
- 検索置換、矩形選択、複数バッファ。
- Home/End/PageUp 等の端末差の大きいキー（矢印＋Ctrl-A/E で足りる範囲に絞る）。

## 8. 表示幅（桁）— 一覧の整列と行長

昔の BBS は「等幅・固定桁」を前提に画面を組んでいた。日本語端末では全角＝2 セル・
半角＝1 セルの二倍幅等幅なので、桁を揃えるには **文字数(rune) ではなく表示セル数**で
数える必要がある（等幅フォントを使うだけでは、サーバが rune 数で詰めると全角列がずれる）。

- ヘルパー: `internal/session/width.go`
  - `DisplayWidth(s)` … 表示桁数（全角=2, 半角=1）
  - `ClipWidth(s, cols)` … cols 桁を超えないよう末尾を切る（全角の途中で割らない）
  - `PadRight(s, cols)` … 左寄せで cols 桁に空白詰め（`fmt` の `%-Ns` の全角対応版）
  - 全角判定は `wideRune`（`session.go`）を共有。
- 整列に適用済み（全角を含み後続に列がある箇所）:
  - `who` の Handle/種別（`command/builtins.go`）、`agent` 一覧の Handle/状態（`command/agent.go`）、
    ユーザ一覧の Handle（`cmdUserlist`）、mail 一覧の Subject（`command/mail.go`）、
    ノート見出し一覧のタイトル（`notes/open.go listTitles`）。
  - 末尾列が全角（news の Subject、talk/chat のルーム題、INDEX 本体のタイトル）は後続が無いので整列対象外。
- 行長（桁）クリップも rune → セルに統一: 電報/チャット本文（`host/social.go` `TelegramMax=78`）、
  talk ライン（`command/talk.go` `TalkLineMax=80`）、エディタ各行（`command/compose.go editField`）。
- 基準桁は将来 `users.term_width`（既定 80）に寄せられるが、現状は各列の固定桁を維持。
- **入力・保存の上限も桁（セル）で切る**（「上限＝列幅」を揃える。定数は `store`）:
  - ハンドル `MaxHandle=16`（who/agent/ユーザ一覧の Handle 列。全角8/半角16まで）。
    signup と `handle` コマンドの両方でセル数え。
  - タイトル `MaxTitle=40`（ノート基文/レス題・talk/chat のルーム題）。
  - これらは `ClipWidth`（保存側）と `PadRight`（表示側）で同じ桁を共有するので、
    全角を入れても列からはみ出さず、rune 数と桁数のズレも起きない。
- **会話（チャット/talk/電報）の発言は切らずに折り返す**。本文は `ClipRunes`（入力
  自体が `TalkLineMax=80` / `TelegramMax=78` の rune 数で制限される）で安全に丸めるだけにし、
  表示時に `session.FoldLine`（`WrapWidth` ＋発言者名幅の字下げ）で端末幅（`term_width`、
  既定80）に折り返す。桁で機械的に切ると全角の会話文が途中で切れてしまうため、
  「切らずにそのままつなげて全文表示」を採る（`notice.go` の NoticeChat/NoticeTalk/NoticeTelegram、
  自分の発言エコー、talk 履歴の再生も同じ折り返しを使う）。

## 9. リスク

- 端末差（矢印・Backspace のバイト列、幅計算＝全角）。全角は既存 `wideRune` を再利用。
- VT 非対応・生 TTY でない接続。→ 簡易モードへフォールバックできる設計にする。
- 多数の `.` ループを一気に替えると事故る。→ ノートから 1 か所ずつ。
