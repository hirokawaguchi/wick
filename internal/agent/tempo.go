package agent

import (
	"sync"
	"time"
)

// tempo は部屋ごとの発話テンポを監督する（複数エージェントで共有）。
//   - floor + pace : 同時に一人ずつ。直近発言から pace 未満は不許可
//   - 連続ターン上限: エージェントの連続発言が maxStreak に達したら restFor 休止
//   - 人が来たら譲る: 人間の発言で streak と休止をリセット（AI は素直に応答してよい）
//
// 「全レス即反応」を防ぎ、AI 同士が部屋を埋め尽くさないための仕組み。
type tempo struct {
	mu        sync.Mutex
	pace      time.Duration
	maxStreak int
	restFor   time.Duration
	isAgent   func(id string) bool
	rooms     map[int]*roomTempo
}

type roomTempo struct {
	lastSpeak time.Time
	streak    int
	restUntil time.Time
}

func newTempo(isAgent func(id string) bool) *tempo {
	return &tempo{
		// AI 同士がよく喋る展開を狙って、連続ターンを長め・休止を短めにする。
		// 部屋に人がいるときだけ会話するので（無人時は静観）、活発でも問題ない。
		// 人が発言すれば humanSpoke で streak/休止がリセットされ、人に譲れる。
		pace:      1000 * time.Millisecond,
		maxStreak: 10,
		restFor:   3 * time.Second,
		isAgent:   isAgent,
		rooms:     map[int]*roomTempo{},
	}
}

func (t *tempo) roomLocked(n int) *roomTempo {
	rs := t.rooms[n]
	if rs == nil {
		rs = &roomTempo{}
		t.rooms[n] = rs
	}
	return rs
}

// tryGrant は now 時点で agentID が room で発言してよいかを判定し、
// 許可時は floor（直近発言時刻）と連続ターンを進める。
func (t *tempo) tryGrant(room int, agentID string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	rs := t.roomLocked(room)
	if now.Before(rs.restUntil) {
		return false // 連続ターン上限に達して休止中
	}
	if !rs.lastSpeak.IsZero() && now.Sub(rs.lastSpeak) < t.pace {
		return false // 直前の発言から間隔が空いていない（floor/cooldown）
	}
	rs.lastSpeak = now
	rs.streak++
	if t.maxStreak > 0 && rs.streak >= t.maxStreak {
		rs.restUntil = now.Add(t.restFor)
		rs.streak = 0
	}
	return true
}

// humanSpoke は人間の発言を観測したときに呼ぶ。連続ターンと休止を解く。
// pace（floor）はそのまま効かせるが、人に宛てられた返答は妨げない
// （lastSpeak は触らないので、直近に AI が喋っていなければ即座に返せる）。
func (t *tempo) humanSpoke(room int, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rs := t.roomLocked(room)
	rs.streak = 0
	rs.restUntil = time.Time{}
}
