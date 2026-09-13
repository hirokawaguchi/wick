package agent

import (
	"strings"
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	s, e, ok := parseWindow("21:00-21:30")
	if !ok || s != 21*60 || e != 21*60+30 {
		t.Fatalf("parseWindow 失敗: %d %d %v", s, e, ok)
	}
	if _, _, ok := parseWindow("bad"); ok {
		t.Fatal("不正な入力を通した")
	}
}

func TestParseRooms(t *testing.T) {
	got := parseRooms("1-3,5")
	want := []int{1, 2, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("parseRooms 長さ違い: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseRooms 中身違い: %v", got)
		}
	}
}

func TestInActiveWindow(t *testing.T) {
	b := &modelBrain{activeSet: true, activeStart: 21 * 60, activeEnd: 21*60 + 30}
	in := time.Date(2026, 1, 1, 21, 15, 0, 0, time.Local)
	out := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)
	if !b.inActiveWindow(in) {
		t.Fatal("窓の中なのに外と判定")
	}
	if b.inActiveWindow(out) {
		t.Fatal("窓の外なのに中と判定")
	}
	// activeSet=false は常に true。
	if !(&modelBrain{}).inActiveWindow(out) {
		t.Fatal("未設定は常時 active のはず")
	}
}

// TestModelBrainWindowGate は、窓の外では自分から話し始めず、
// 話しかけられれば返すことを確かめる。
func TestModelBrainWindowGate(t *testing.T) {
	srv := mockModel(t, `{"action":"say","text":"やあ"}`, nil)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.activeSet, b.activeStart, b.activeEnd = true, 21*60, 21*60+30
	b.silenceLimit = 1
	out := time.Date(2026, 1, 1, 10, 0, 0, 0, time.Local)

	// 窓の外＋誰も喋っていない → 自発しない（沈黙）。人は在室（Audience）。
	if s, _ := b.Next(Observation{Doing: "CHAT1", Now: out, Audience: true}); s != "" {
		t.Fatalf("窓の外で自発した: %q", s)
	}
	// 窓の外でも、話しかけられたら返す。
	s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> やあ\r\n", Now: out, Audience: true})
	if s != "やあ\n" {
		t.Fatalf("話しかけに応じない: %q", s)
	}
}

func TestSanitizeTalkLine(t *testing.T) {
	if got := sanitizeTalkLine("/kick foo"); got != "kick foo" {
		t.Fatalf("スラッシュ除去できていない: %q", got)
	}
	if got := sanitizeTalkLine(" \n本題です\n次行 "); got != "本題です" {
		t.Fatalf("先頭の非空行を取れていない: %q", got)
	}
	if got := sanitizeTalkLine("."); got != "" {
		t.Fatalf("退室キーを無害化していない: %q", got)
	}
}

func TestParseTalkRoom(t *testing.T) {
	if n, ok := parseTalkRoom("TALK7"); !ok || n != 7 {
		t.Fatalf("TALK7 を解釈できない: %d %v", n, ok)
	}
	if _, ok := parseTalkRoom("CHAT1"); ok {
		t.Fatal("CHAT を talk と誤認")
	}
}

func TestRecentTalk(t *testing.T) {
	screen := "-- 直近 --\n   1 sysop> こんにちは\n   2 kai> やあ\nゴミ行\n"
	got := recentTalk(screen)
	if !strings.Contains(got, "sysop> こんにちは") || !strings.Contains(got, "kai> やあ") {
		t.Fatalf("ログ抽出が違う: %q", got)
	}
	if strings.Contains(got, "ゴミ行") {
		t.Fatalf("ログでない行を拾った: %q", got)
	}
}

// TestTalkerRotation は、talker が「入室 → 1 行投稿して退室」の順に手を出すことを確かめる
// （model 未接続なので定型文で動く）。
func TestTalkerRotation(t *testing.T) {
	b := newTalkerBrain(ModelConfig{}, Spec{Rooms: []int{3}, Interval: time.Minute}, "kai", "Kai")
	base := time.Now()
	// 1 手目: MAIN から入室。
	if s, _ := b.Next(Observation{Doing: "MAIN", Now: base}); s != "talk 3\n" {
		t.Fatalf("入室しない: %q", s)
	}
	// 2 手目: 部屋の中で 1 行投稿してから . で退室。
	s, _ := b.Next(Observation{Doing: "TALK3", Screen: "-- 直近 --\n   1 sysop> やあ\n", Now: base})
	if !strings.HasSuffix(s, "\n.\n") || strings.HasPrefix(s, ".") {
		t.Fatalf("投稿→退室の形が違う: %q", s)
	}
	// 間隔中は動かない。
	if s, _ := b.Next(Observation{Doing: "MAIN", Now: base.Add(time.Second)}); s != "" {
		t.Fatalf("間隔中に動いた: %q", s)
	}
	// 間隔を過ぎたら次の巡回。
	if s, _ := b.Next(Observation{Doing: "MAIN", Now: base.Add(2 * time.Minute)}); s != "talk 3\n" {
		t.Fatalf("次の巡回が始まらない: %q", s)
	}
}
