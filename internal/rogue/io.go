// Package rogue は Wick の内部ゲーム「rogue（ローグ・クローン）」の実装。
//
// 仕様の正は Rogue_Archive.Official の Rogue2.Official（データ分離・UTF-8 の
// 日本語ローグ・クローン最終版）。ゲーム内キーは太田純氏ガイドどおり
// （hjkl yubn、大文字走り、q 飲む / Q 終了、. 休憩 など）。
//
// C ソースや mesg（太田氏の著作物）は取り込まない。ロジックは Go で書き直し、
// 画面は Wick のセッション（ANSI / 1 キー読み）に載せる。文言は Wick 所有の
// ja/en を新しく書き、意味だけ原典に合わせる。
package rogue

import "strings"

// 画面レイアウト（原典どおり 80x24 前提）。
const (
	Cols      = 80
	Rows      = 24
	MsgRow    = 0  // 最上行＝メッセージ
	MapTop    = 1  // マップ開始行（画面行）
	MapH      = 22 // マップの高さ
	MapW      = 80 // マップの幅
	StatusRow = 23
)

// IO はゲームの入出力口。Wick のセッションを包む薄いアダプタが実装する。
// テストは擬似実装でキー列を流し、画面出力を検証できる。
type IO interface {
	// ReadKey はコマンド 1 キーを返す。CR は '\n'。切断時は非 nil の error。
	ReadKey() (byte, error)
	// Print は生の出力（ANSI 制御を含む）。改行は呼び側で \n を使えばよい
	// （セッション側が \n→\r\n に変換する）。
	Print(s string)
	// Pending は全画面中に保留された通知（電報など）の 1 行要約を返す。
	// 無ければ nil。ゲームはこれをメッセージ行に短く出す。
	Pending() []string
	// Size は端末の桁・行。0 のときはゲーム側が既定（80x24）に倒す。
	Size() (w, h int)
}

// ANSI 制御。全画面はフレームごとに全再描画する（24x80 と小さい）。
const (
	ansiClear    = "\x1b[2J"
	ansiHome     = "\x1b[H"
	ansiHideCur  = "\x1b[?25l"
	ansiShowCur  = "\x1b[?25h"
	ansiClearEOL = "\x1b[K"
	ansiReset    = "\x1b[0m"
)

// gotoRC は 1 始まりの行・桁へカーソルを移す ANSI を返す。
func gotoRC(row, col int) string {
	return "\x1b[" + itoa(row+1) + ";" + itoa(col+1) + "H"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// readLine は 1 行を ReadKey で読む（別画面プロンプト用。名前・回数・確認など）。
// Backspace で 1 文字消す。ESC で中止（ok=false）。エコーは呼び側の Print で行う。
func readLine(io IO, max int) (string, bool, error) {
	var sb strings.Builder
	n := 0
	for {
		c, err := io.ReadKey()
		if err != nil {
			return "", false, err
		}
		switch c {
		case '\n':
			return sb.String(), true, nil
		case 0x1b: // ESC 中止
			return "", false, nil
		case 0x7f, '\b':
			if n > 0 {
				s := sb.String()
				s = s[:len(s)-1]
				sb.Reset()
				sb.WriteString(s)
				n--
				io.Print("\b \b")
			}
		default:
			if c >= 32 && c < 127 && n < max {
				sb.WriteByte(c)
				n++
				io.Print(string([]byte{c}))
			}
		}
	}
}
