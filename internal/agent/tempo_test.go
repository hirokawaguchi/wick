package agent

import (
	"testing"
	"time"
)

// TestTempoPaceAndStreak は floor/間隔・連続ターン上限・人の発言リセットを確かめる。
func TestTempoPaceAndStreak(t *testing.T) {
	agents := map[string]bool{"a": true, "b": true}
	tp := newTempo(func(id string) bool { return agents[id] })
	tp.pace = time.Second
	tp.maxStreak = 4
	tp.restFor = 10 * time.Second

	t0 := time.Now()

	// 最初の発言は通る。
	if !tp.tryGrant(1, "a", t0) {
		t.Fatal("1手目が通らない")
	}
	// 直後（pace 未満）は別エージェントでも不許可＝同時に喋らない。
	if tp.tryGrant(1, "b", t0.Add(200*time.Millisecond)) {
		t.Fatal("pace 未満で通ってしまった（floor が効いていない）")
	}
	// pace 経過後は通る。
	if !tp.tryGrant(1, "b", t0.Add(1200*time.Millisecond)) {
		t.Fatal("pace 経過後が通らない")
	}

	// ここまで streak=2。あと2手で maxStreak=4 に達し休止に入る。
	if !tp.tryGrant(1, "a", t0.Add(3*time.Second)) {
		t.Fatal("3手目が通らない")
	}
	if !tp.tryGrant(1, "b", t0.Add(5*time.Second)) {
		t.Fatal("4手目が通らない")
	}
	// 休止中は間隔を空けても不許可。
	if tp.tryGrant(1, "a", t0.Add(6*time.Second)) {
		t.Fatal("連続ターン上限の休止が効いていない")
	}

	// 人間の発言で休止・連続ターンが解ける（＝人に返せる）。
	tp.humanSpoke(1, t0.Add(7*time.Second))
	// 直近の AI 発言(t+5s)から pace は過ぎているので、休止が解けて返せる。
	if !tp.tryGrant(1, "a", t0.Add(7*time.Second)) {
		t.Fatal("人の発言後も休止が解けていない")
	}

	// 別部屋は独立。
	if !tp.tryGrant(2, "a", t0.Add(6*time.Second)) {
		t.Fatal("別部屋のテンポが混ざっている")
	}
}

// TestScreenHasHumanSpeech は画面から人間の発言だけを拾えることを確かめる。
func TestScreenHasHumanSpeech(t *testing.T) {
	isAgent := func(id string) bool { return id == "critic" || id == "scout" }
	// エージェントだけの発言は human ではない。
	if screenHasHumanSpeech("\r\ncritic> やあ\r\n", isAgent) {
		t.Fatal("エージェントの発言を人間と誤検出")
	}
	// 入退室行（"> " を含まない）も無視。
	if screenHasHumanSpeech("\r\n-- alice が入室 --\r\n", isAgent) {
		t.Fatal("入室行を発言と誤検出")
	}
	// 人間の発言は検出。
	if !screenHasHumanSpeech("\r\ncritic> やあ\r\nalice> こんにちは\r\n", isAgent) {
		t.Fatal("人間の発言を拾えていない")
	}
}
