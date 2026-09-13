package agent

import "testing"

type stubBrain struct{ script string }

func (s *stubBrain) Next(Observation) (string, bool) { return s.script, false }

// TestWanderVisitsNotesWhenIdle は、inner が idle かつ MAIN にいるとき、
// ノートを開いて数心拍滞在してから q で戻ることを確かめる。
func TestWanderVisitsNotesWhenIdle(t *testing.T) {
	inner := &stubBrain{script: ""} // 常に idle
	w := newWanderBrain(inner, func() []string { return []string{"junk.test"} }, "columnist")
	w.stay = 2
	w.gap = 10

	// MAIN で idle → ノートを開きに行く。
	if s, _ := w.Next(Observation{Tick: 1, Doing: "MAIN"}); s != "open junk.test\n" {
		t.Fatalf("idle 時にノートを開かない: %q", s)
	}
	// 閲覧中は滞在（何もしない）。
	if s, _ := w.Next(Observation{Tick: 2, Doing: "NOTE junk.test"}); s != "" {
		t.Fatalf("滞在中は黙るはず: %q", s)
	}
	// 滞在心拍を過ぎたら q で戻る。
	if s, _ := w.Next(Observation{Tick: 3, Doing: "NOTE junk.test"}); s != "q\n" {
		t.Fatalf("滞在後は q で戻るはず: %q", s)
	}
}

// TestWanderYieldsToInner は、inner が実際の手を返すときはそれを優先し、
// 既に MAIN 以外にいるときは出歩かないことを確かめる。
func TestWanderYieldsToInner(t *testing.T) {
	inner := &stubBrain{script: "say hello\n"}
	w := newWanderBrain(inner, func() []string { return []string{"junk.test"} }, "x")
	if s, _ := w.Next(Observation{Tick: 1, Doing: "CHAT1"}); s != "say hello\n" {
		t.Fatalf("inner の手を通していない: %q", s)
	}

	inner2 := &stubBrain{script: ""}
	w2 := newWanderBrain(inner2, func() []string { return []string{"junk.test"} }, "x")
	// MAIN 以外にいるなら出歩かない。
	if s, _ := w2.Next(Observation{Tick: 1, Doing: "TALK3"}); s != "" {
		t.Fatalf("MAIN 以外では出歩かないはず: %q", s)
	}
}
