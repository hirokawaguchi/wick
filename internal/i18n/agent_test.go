package i18n

import "testing"

func TestAgentCatalog(t *testing.T) {
	if got := T(JA, "agent.react", "kai", "hi"); got != "kai さん、「hi」了解です。" {
		t.Fatalf("react ja = %q", got)
	}
	if got := T(EN, "agent.react", "kai", "hi"); got != "kai: understood — \"hi\"." {
		t.Fatalf("react en = %q", got)
	}
	if got := T(JA, "seed.board.sandbox"); got != "もう一つのジャンク" {
		t.Fatalf("sandbox ja = %q", got)
	}
	if got := T(EN, "seed.board.sandbox"); got != "another junk" {
		t.Fatalf("sandbox en = %q", got)
	}
	if got := T(EN, "agent.llm.chat_sys", "Kai", "kai"); !containsASCII(got) {
		t.Fatalf("chat_sys en looks Japanese: %q", clip(got, 80))
	}
}

func containsASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
