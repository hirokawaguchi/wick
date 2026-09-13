package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/store"
)

// TestResponderRespondsToTopic は、既設の話題ベースノートに「レス」する（新規
// ベースノートを作らない）ことを確かめる。
func TestResponderRespondsToTopic(t *testing.T) {
	feed := func() []NoteInfo { return []NoteInfo{{Num: 2, Title: "雑談", RespCount: 3}} }
	rb := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed)
	base := time.Now()
	if s, _ := rb.Next(Observation{Now: base}); s != "" {
		t.Fatalf("初回は基準時刻だけのはず: %q", s)
	}
	if s, _ := rb.Next(Observation{Now: base.Add(time.Second)}); s != "" {
		t.Fatalf("間隔中は動かないはず: %q", s)
	}
	s, _ := rb.Next(Observation{Now: base.Add(2 * time.Minute)})
	if !strings.HasPrefix(s, "open junk.test\n2\nw\n") {
		t.Fatalf("既設話題へのレスになっていない: %q", s)
	}
	if !strings.HasSuffix(s, "\n.\nq") {
		t.Fatalf("レス投稿の締めが違う: %q", s)
	}
}

// TestResponderContinuationWhenFull は、対象話題がレス上限に達したら「継続」
// ベースノートを 1 本だけ立てることを確かめる（慣習の例外規定）。
func TestResponderContinuationWhenFull(t *testing.T) {
	feed := func() []NoteInfo {
		return []NoteInfo{{Num: 2, Title: "雑談", RespCount: store.MaxResponses}}
	}
	rb := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed)
	rb.lastPost = time.Now().Add(-2 * time.Minute) // 間隔は満たす
	s, _ := rb.Next(Observation{Now: time.Now()})
	if !strings.HasPrefix(s, "open junk.test\nw続き: ") {
		t.Fatalf("継続ベースノートになっていない: %q", s)
	}
	if !strings.Contains(s, "からの継続です") {
		t.Fatalf("継続の説明が入っていない: %q", s)
	}
}

// TestResponderGenUsed は、gen があればモデル生成のレス本文を使うことを確かめる。
func TestResponderGenUsed(t *testing.T) {
	feed := func() []NoteInfo { return []NoteInfo{{Num: 5, Title: "本と音楽", RespCount: 1}} }
	rb := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed)
	rb.gen = func(title, intro string, recent []string, quoteHandle, quoteBody, webCtx string) (string, error) {
		return "生成したレス本文\n", nil
	}
	rb.lastPost = time.Now().Add(-2 * time.Minute)
	s, _ := rb.Next(Observation{Now: time.Now()})
	if !strings.Contains(s, "生成したレス本文") {
		t.Fatalf("gen の本文を使っていない: %q", s)
	}
	if !strings.HasPrefix(s, "open junk.test\n5\nw\n") {
		t.Fatalf("番号5へのレスになっていない: %q", s)
	}
}

// TestResponderRepliesToOther は、他者（人間）の未応答レスがあれば、その話題へ
// 引用返信すること（人間優先・> 引用つき・quote 情報が gen に渡る）を確かめる。
func TestResponderRepliesToOther(t *testing.T) {
	feed := func() []NoteInfo {
		return []NoteInfo{
			{Num: 3, Title: "雑談", RespCount: 1, Recent: []NoteResp{
				{Author: "poet", Handle: "Poet", Body: "AIのレス", Agent: true},
			}},
			{Num: 7, Title: "日々の発見", RespCount: 2, Recent: []NoteResp{
				{Author: "alice", Handle: "Alice", Body: "今日は良い天気でした\n散歩した", Agent: false},
			}},
		}
	}
	rb := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed)
	rb.lastPost = time.Now().Add(-2 * time.Minute)
	// フォールバック（gen なし）での引用書式を確認。
	s, _ := rb.Next(Observation{Now: time.Now()})
	if !strings.HasPrefix(s, "open junk.test\n7\nw\n") && !strings.HasPrefix(s, "open junk.test\n3\nw\n") {
		t.Fatalf("他者レスのある話題へのレスになっていない: %q", s)
	}
	if !strings.Contains(s, "\n> ") {
		t.Fatalf("> 引用が入っていない: %q", s)
	}

	// gen があるとき、人間(Alice)を優先して quote 情報が渡ることを確認。
	feed2 := func() []NoteInfo {
		return []NoteInfo{
			{Num: 7, Title: "日々の発見", RespCount: 2, Recent: []NoteResp{
				{Author: "poet", Handle: "Poet", Body: "AIのレス", Agent: true},
				{Author: "alice", Handle: "Alice", Body: "人間のレス", Agent: false},
			}},
		}
	}
	rb2 := newResponderBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist", feed2)
	var gotQH, gotQB string
	rb2.gen = func(title, intro string, recent []string, quoteHandle, quoteBody, webCtx string) (string, error) {
		gotQH, gotQB = quoteHandle, quoteBody
		return "> 人間のレス\nなるほど。\n", nil
	}
	rb2.lastPost = time.Now().Add(-2 * time.Minute)
	if _, _ = rb2.Next(Observation{Now: time.Now()}); gotQH != "Alice" || gotQB != "人間のレス" {
		t.Fatalf("人間を優先して quote が渡っていない: qh=%q qb=%q", gotQH, gotQB)
	}
}
