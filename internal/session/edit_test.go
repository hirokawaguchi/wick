package session

import (
	"io"
	"strings"
	"testing"
)

func TestEditorBufInsertNewlineBackspace(t *testing.T) {
	b := newEditorBuf("")
	for _, r := range "ab" {
		b.insertRune(r)
	}
	if b.text() != "ab" {
		t.Fatalf("insert: %q", b.text())
	}
	b.left()
	b.insertRune('X') // a X b
	if b.text() != "aXb" {
		t.Fatalf("mid insert: %q", b.text())
	}
	b.newline() // カーソルは col2。split: "aX" / "b"
	if b.text() != "aX\nb" {
		t.Fatalf("newline: %q", b.text())
	}
	// カーソルは 2 行目先頭。Backspace で行連結
	b.backspace()
	if b.text() != "aXb" {
		t.Fatalf("join: %q", b.text())
	}
}

func TestEditorBufVerticalMove(t *testing.T) {
	b := newEditorBuf("hello\nworld")
	if b.row != 1 || b.col != 5 {
		t.Fatalf("init pos row=%d col=%d", b.row, b.col)
	}
	b.up() // 桁 5 を保持（hello は 5 文字なので行末）
	if b.row != 0 || b.col != 5 {
		t.Fatalf("up: row=%d col=%d", b.row, b.col)
	}
	b.col = 2
	b.down() // world は 5 文字。桁 2 は保持
	if b.row != 1 || b.col != 2 {
		t.Fatalf("down: row=%d col=%d", b.row, b.col)
	}
}

func TestEditorBufIsDotLine(t *testing.T) {
	b := newEditorBuf("hi\n.")
	if !b.isDotLine() {
		t.Fatal("最終行 . を検出できていない")
	}
	b2 := newEditorBuf(".\nhi")
	if b2.isDotLine() {
		t.Fatal("最終行でない . を送信扱いにしている")
	}
}

func TestEditTextSubmit(t *testing.T) {
	// hello Enter . Enter → 送信、本文は hello
	s := New("t", strings.NewReader("hello\r.\r"), io.Discard)
	text, ok, err := s.EditText("", 500)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("送信されていない")
	}
	if text != "hello" {
		t.Fatalf("本文 %q", text)
	}
}

func TestEditTextCancel(t *testing.T) {
	// Ctrl-C (0x03) で破棄
	s := New("t", strings.NewReader("abc\x03"), io.Discard)
	_, ok, err := s.EditText("", 500)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Ctrl-C が破棄になっていない")
	}
}

func TestEditTextArrowEdit(t *testing.T) {
	// ab, ←, X → aXb を末尾へ移動(Ctrl-E)後 改行して . で送信
	// 入力: a b ESC[D X  Ctrl-E \r . \r
	in := "ab\x1b[DX\x05\r.\r"
	s := New("t", strings.NewReader(in), io.Discard)
	text, ok, err := s.EditText("", 500)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("送信されていない")
	}
	if text != "aXb" {
		t.Fatalf("本文 %q", text)
	}
}
