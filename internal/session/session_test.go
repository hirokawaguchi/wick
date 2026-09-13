package session

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
)

// TestEmitNoticeRedraw は、人間がプロンプト付きで入力中に着信すると、
// 入力行を消して通知を出し、プロンプト＋打ちかけを描き直すことを確かめる（A案）。
func TestEmitNoticeRedraw(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader(""), &out)
	buf := []rune("こん")
	s.inputActive = true
	s.inputPrompt = "sysop> "
	s.inputBuf = &buf

	s.emitNotice(Notice{Kind: NoticeChat, FromID: "poet", Body: "やあ"})
	got := out.String()
	if !strings.Contains(got, "\r\x1b[K") {
		t.Fatalf("入力行を消していない: %q", got)
	}
	if !strings.Contains(got, "poet> やあ") {
		t.Fatalf("通知が出ていない: %q", got)
	}
	if !strings.Contains(got, "sysop> こん") {
		t.Fatalf("プロンプト＋打ちかけを描き直していない: %q", got)
	}
}

// TestEmitNoticeAgentPlain は、エージェント（AgentIO）では再描画せず素通しで、
// 観測用の画面が従来どおり（先頭改行付きの id> 行）であることを確かめる。
func TestEmitNoticeAgentPlain(t *testing.T) {
	s, aio := NewPipe("agent", "poet", "Poet")
	s.inputActive = true // 入力中でも
	s.inputPrompt = "poet> "
	buf := []rune("")
	s.inputBuf = &buf
	s.emitNotice(Notice{Kind: NoticeChat, FromID: "alice", Body: "やあ"})
	got := aio.Drain()
	if strings.Contains(got, "\x1b[K") {
		t.Fatalf("エージェントに再描画制御が混ざった: %q", got)
	}
	if !strings.Contains(got, "alice> やあ") || strings.Contains(got, "poet> ") {
		t.Fatalf("素通しの通知になっていない（プロンプト再描画が混ざった）: %q", got)
	}
}

func TestReadLineCtrlCInterrupt(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("ab\x03rest\n"), &out)
	line, err := s.ReadLine(64)
	if !errors.Is(err, ErrInterrupt) {
		t.Fatalf("Ctrl-C で ErrInterrupt が返らない: line=%q err=%v", line, err)
	}
	if line != "" {
		t.Fatalf("中止時は空文字を返すべき: %q", line)
	}
}

func TestReadLineCRLF(t *testing.T) {
	s := New("t", strings.NewReader("hello\r\nnext\n"), io.Discard)
	line, err := s.ReadLine(64)
	if err != nil || line != "hello" {
		t.Fatalf("got %q %v", line, err)
	}
	line, err = s.ReadLine(64)
	if err != nil || line != "next" {
		t.Fatalf("got %q %v", line, err)
	}
}

func TestReadLineCROnly(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("who\roff\r"), &out)
	line, err := s.ReadLine(64)
	if err != nil || line != "who" {
		t.Fatalf("got %q %v", line, err)
	}
	line, err = s.ReadLine(64)
	if err != nil || line != "off" {
		t.Fatalf("got %q %v", line, err)
	}
	if !strings.Contains(out.String(), "who") {
		t.Fatalf("echo missing: %q", out.String())
	}
}

// TestReadLineNoPtyNoEcho は、pty 無しクライアント（cooked な行モード）では
// サーバがエコーしないことを確かめる（クライアントのローカルエコーとの二重化防止）。
func TestReadLineNoPtyNoEcho(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("hello\r"), &out)
	s.SetPTY(false)
	line, err := s.ReadLine(64)
	if err != nil || line != "hello" {
		t.Fatalf("got %q %v", line, err)
	}
	if strings.Contains(out.String(), "hello") {
		t.Fatalf("pty 無しなのにサーバがエコーした: %q", out.String())
	}
}

// TestEmitNoticeNoPtyPlain は、pty 無しでは ANSI 再描画をせず素通し表示に
// なることを確かめる（cooked 端末での画面崩れ防止）。
func TestEmitNoticeNoPtyPlain(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader(""), &out)
	s.SetPTY(false)
	buf := []rune("こん")
	s.inputActive = true
	s.inputPrompt = "alice> "
	s.inputBuf = &buf
	s.emitNotice(Notice{Kind: NoticeChat, FromID: "poet", Body: "やあ"})
	got := out.String()
	if strings.Contains(got, "\x1b[K") {
		t.Fatalf("pty 無しなのに再描画制御が出た: %q", got)
	}
	if !strings.Contains(got, "poet> やあ") {
		t.Fatalf("通知が出ていない: %q", got)
	}
}

func TestReadSecretNoEcho(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("secret\r"), &out)
	line, err := s.ReadSecret(64)
	if err != nil || line != "secret" {
		t.Fatalf("got %q %v", line, err)
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatalf("secret echoed: %q", out.String())
	}
}

func TestReadKeyCR(t *testing.T) {
	s := New("t", strings.NewReader("a\rq"), io.Discard)
	c, err := s.ReadKey()
	if err != nil || c != 'a' {
		t.Fatalf("got %q %v", c, err)
	}
	c, err = s.ReadKey()
	if err != nil || c != '\n' {
		t.Fatalf("cr got %q %v", c, err)
	}
}

func TestReadKeyUTF8(t *testing.T) {
	s := New("t", strings.NewReader(" \u3000ｑあ"), io.Discard)
	c, err := s.ReadKey()
	if err != nil || c != ' ' {
		t.Fatalf("ascii space %q %v", c, err)
	}
	c, err = s.ReadKey()
	if err != nil || c != ' ' {
		t.Fatalf("ideographic space %q %v", c, err)
	}
	c, err = s.ReadKey()
	if err != nil || c != 'q' {
		t.Fatalf("fullwidth q %q %v", c, err)
	}
	c, err = s.ReadKey()
	if err != nil || c != 0 {
		t.Fatalf("hiragana %q %v", c, err)
	}
}

func TestReadLineUTF8(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("こんにちは　世界\r"), &out)
	line, err := s.ReadLine(40)
	if err != nil || line != "こんにちは　世界" {
		t.Fatalf("got %q %v", line, err)
	}
	if !strings.Contains(out.String(), "こんにちは　世界") {
		t.Fatalf("echo %q", out.String())
	}
}

func TestReadCommandFoldsFullwidth(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader("ｏｐｅｎ　ｊｕｎｋ．ｔｅｓｔ\r"), &out)
	line, err := s.ReadCommand(128)
	if err != nil || line != "open junk.test" {
		t.Fatalf("got %q %v", line, err)
	}
	if !strings.Contains(out.String(), "ｏｐｅｎ　ｊｕｎｋ．ｔｅｓｔ") {
		t.Fatalf("echo should stay as typed: %q", out.String())
	}
}

func TestReadLineEOF(t *testing.T) {
	s := New("t", strings.NewReader(""), io.Discard)
	_, err := s.ReadLine(64)
	if err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestClipRunes(t *testing.T) {
	if ClipRunes("あいうえお", 3) != "あいう" {
		t.Fatal(ClipRunes("あいうえお", 3))
	}
	if ClipRunes("abc", 8) != "abc" {
		t.Fatal("short")
	}
}

func TestPrintCRLF(t *testing.T) {
	var out strings.Builder
	s := New("t", strings.NewReader(""), &out)
	s.Print("a\nb\n")
	if out.String() != "a\r\nb\r\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestNoticeTextLang(t *testing.T) {
	when := time.Date(2026, 9, 13, 12, 0, 0, 0, time.Local)
	n := Notice{Kind: NoticeTelegram, FromID: "alice", Handle: "Alice", Time: when, Body: "hi"}
	ja := New("t", strings.NewReader(""), io.Discard)
	if got := ja.noticeText(n); !strings.Contains(got, "電報 from alice") {
		t.Fatalf("ja telegram: %q", got)
	}
	en := New("t", strings.NewReader(""), io.Discard)
	en.Lang = i18n.EN
	if got := en.noticeText(n); !strings.Contains(got, "telegram from alice") {
		t.Fatalf("en telegram: %q", got)
	}
	join := Notice{Kind: NoticeChatJoin, FromID: "bob"}
	if got := en.noticeText(join); !strings.Contains(got, "bob entered") {
		t.Fatalf("en join: %q", got)
	}
}
