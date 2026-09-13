package agent

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/i18n"
)

// TestConversantCapsAgentReacts は、conversant がエージェント相手には数回で相づちを
// 打ち止め、人間の発言で反応を再開することを確かめる（AI 同士の無限往復の防止）。
func TestConversantCapsAgentReacts(t *testing.T) {
	b := &conversantBrain{
		selfID:         "muse",
		room:           1,
		lang:           i18n.JA,
		silenceLimit:   3,
		maxFillers:     3,
		maxAgentReacts: 2,
		isAgent:        func(id string) bool { return id == "poet" || id == "muse" },
	}
	// 入室（最初の 1 手）。
	if s, _ := b.Next(Observation{}); !strings.HasPrefix(s, "chat ") {
		t.Fatalf("最初は入室のはず: %q", s)
	}

	// 相手（エージェント poet）が喋り続けても、相づちは maxAgentReacts 回まで。
	reacts := 0
	for i := 0; i < 12; i++ {
		s, _ := b.Next(Observation{Screen: "poet> こんにちは " + strconv.Itoa(i) + "\n"})
		if strings.Contains(s, "了解です") {
			reacts++
		}
	}
	if reacts != 2 {
		t.Fatalf("エージェント相手の相づち回数 = %d, want 2", reacts)
	}

	// 人間 alice が発言したら反応を再開する。
	s, _ := b.Next(Observation{Screen: "alice> やっほー\n"})
	if !strings.Contains(s, "了解です") {
		t.Fatalf("人間には反応するはず: %q", s)
	}
}

// TestReactLineNoNestedQuotes は、相手の相づちを引用しても鉤括弧が入れ子に
// 肥大しないことを確かめる。
func TestReactLineNoNestedQuotes(t *testing.T) {
	prev := "poet さん、「muse さん、「やあ」了解です。」了解です。"
	got := reactLine("poet", prev, i18n.JA)
	// 出力の鉤括弧は、テンプレートの 1 対（「…」）だけであるべき。
	if n := strings.Count(got, "「"); n != 1 {
		t.Fatalf("鉤括弧が入れ子になっている: %q (count=%d)", got, n)
	}
}
