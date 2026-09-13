package session

import (
	"strings"
	"testing"
)

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"":    0,
		"abc": 3,
		"あいう": 6,
		"aあb": 4,
		"ＡＢ":  4, // 全角英字
	}
	for in, want := range cases {
		if got := DisplayWidth(in); got != want {
			t.Errorf("DisplayWidth(%q)=%d want %d", in, got, want)
		}
	}
}

func TestClipWidth(t *testing.T) {
	if got := ClipWidth("あいうえお", 5); got != "あい" { // 4 cells; 5 目に全角は入らない
		t.Errorf("ClipWidth zenkaku odd = %q", got)
	}
	if got := ClipWidth("あいうえお", 6); got != "あいう" {
		t.Errorf("ClipWidth zenkaku even = %q", got)
	}
	if got := ClipWidth("abc", 8); got != "abc" {
		t.Errorf("ClipWidth ascii = %q", got)
	}
	if got := ClipWidth("aあb", 3); got != "aあ" {
		t.Errorf("ClipWidth mixed = %q", got)
	}
}

func TestPadRight(t *testing.T) {
	// 全角3文字=6セル → 8桁に揃えると空白2つ
	if got := PadRight("あいう", 8); got != "あいう  " {
		t.Errorf("PadRight zenkaku = %q (w=%d)", got, DisplayWidth(got))
	}
	// 半角3文字 → 8桁で空白5つ（従来の %-8s と同じ）
	if got := PadRight("abc", 8); got != "abc     " {
		t.Errorf("PadRight ascii = %q", got)
	}
	// はみ出しはクリップして幅ちょうど
	if got := PadRight("あいうえお", 6); got != "あいう" {
		t.Errorf("PadRight clip = %q", got)
	}
	// 全角と半角混在でも表示幅が一致する
	a := PadRight("あいう", 16)
	b := PadRight("abcdef", 16)
	if DisplayWidth(a) != DisplayWidth(b) {
		t.Errorf("PadRight widths differ: %d vs %d", DisplayWidth(a), DisplayWidth(b))
	}
}

func TestWrapWidth(t *testing.T) {
	// 全角6文字(=12桁)を6桁で折ると2行、全文が残る（切り捨てない）。
	got := WrapWidth("あいうえおか", 6)
	if len(got) != 2 || got[0] != "あいう" || got[1] != "えおか" {
		t.Fatalf("WrapWidth zenkaku = %#v", got)
	}
	if strings.Join(got, "") != "あいうえおか" {
		t.Fatalf("WrapWidth lost text: %#v", got)
	}
	// 既存の改行は段落境界として尊重。
	if got := WrapWidth("ab\ncd", 10); len(got) != 2 || got[0] != "ab" || got[1] != "cd" {
		t.Fatalf("WrapWidth newline = %#v", got)
	}
}

func TestFoldLine(t *testing.T) {
	s := New("t", strings.NewReader(""), &strings.Builder{})
	s.User.TermWidth = 12 // prefix "p> "(3桁) → 本文は 9桁ずつ
	out := s.FoldLine("p> ", "あいうえおか")
	// 1行目: "p> あいうえ"(3+8=11) 2行目字下げ3 + "おか"
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("FoldLine lines = %#v", lines)
	}
	if lines[0] != "p> あいうえ" {
		t.Fatalf("FoldLine head = %q", lines[0])
	}
	if lines[1] != "   おか" {
		t.Fatalf("FoldLine cont = %q", lines[1])
	}
}
