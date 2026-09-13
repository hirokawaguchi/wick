package host

import (
	"sort"
	"sync"
	"time"

	"github.com/hirokawaguchi/wick/internal/session"
)

type Host struct {
	mu       sync.Mutex
	online   map[string]*session.Session
	max      int
	chkDup   bool
	seedPass string
	boardMu  map[int64]*sync.Mutex
	rooms    []*chatRoom
	talk     map[int]*talkRoomLive
	shutdown chan struct{}
	shutOnce sync.Once
	started  time.Time
}

func New(max int) *Host {
	if max <= 0 {
		max = 500
	}
	h := &Host{
		online:   make(map[string]*session.Session),
		max:      max,
		chkDup:   true,
		boardMu:  make(map[int64]*sync.Mutex),
		shutdown: make(chan struct{}),
		started:  time.Now(),
	}
	h.initChat(DefaultChatRooms)
	h.initTalk()
	return h
}

// Broadcast は在室の全セッションへ通知を送る（送信元も含む）。
func (h *Host) Broadcast(n session.Notice) {
	h.mu.Lock()
	targets := make([]*session.Session, 0, len(h.online))
	for _, s := range h.online {
		targets = append(targets, s)
	}
	h.mu.Unlock()
	for _, s := range targets {
		s.Notify(n)
	}
}

// RequestShutdown は graceful 停止を一度だけ要求する。main が ShutdownC を待つ。
func (h *Host) RequestShutdown() {
	h.shutOnce.Do(func() { close(h.shutdown) })
}

// ShutdownC は停止要求が来たら閉じられるチャネル。
func (h *Host) ShutdownC() <-chan struct{} { return h.shutdown }

func (h *Host) LockBoard(id int64) func() {
	h.mu.Lock()
	m, ok := h.boardMu[id]
	if !ok {
		m = &sync.Mutex{}
		h.boardMu[id] = m
	}
	h.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (h *Host) Max() int { return h.max }

// Started は局（プロセス）の起動時刻。
func (h *Host) Started() time.Time { return h.started }

// Uptime は起動からの経過時間。
func (h *Host) Uptime() time.Duration { return time.Since(h.started) }

func (h *Host) TryEnter(s *session.Session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.chkDup {
		if _, ok := h.online[s.User.ID]; ok {
			return false
		}
	}
	if len(h.online) >= h.max {
		return false
	}
	ch := h.allocChanLocked()
	if ch == 0 {
		return false
	}
	s.Chan = ch
	h.online[s.User.ID] = s
	return true
}

func (h *Host) allocChanLocked() int {
	used := make(map[int]bool, len(h.online))
	for _, s := range h.online {
		if s.Chan > 0 {
			used[s.Chan] = true
		}
	}
	for i := 1; i <= h.max; i++ {
		if !used[i] {
			return i
		}
	}
	return 0
}

func (h *Host) Leave(id string) {
	h.LeaveTalk(id)
	h.LeaveChat(id)
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.online, id)
}

// Presence は who / ! 用の在室スナップショット。
// session.Session（Mutex を持つ）を値コピーしないため必要な項目だけ写す。
type Presence struct {
	Chan   int
	ID     string
	Handle string
	Doing  string
	Agent  bool // エージェント（AgentIO）なら true
}

func (h *Host) Who() []Presence {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Presence, 0, len(h.online))
	for _, s := range h.online {
		out = append(out, Presence{
			Chan:   s.Chan,
			ID:     s.User.ID,
			Handle: s.User.Handle,
			Doing:  s.GetDoing(),
			Agent:  s.IsAgent(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Chan < out[j].Chan })
	return out
}

func (h *Host) OnlineCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.online)
}

// CloseHumans は在室の人間セッションへ告知して接続を閉じる（graceful 停止用）。
// エージェント（AgentIO）は Manager が別途止めるので対象外。コマンドループは
// 接続クローズで EOF 終了し、ハンドラが切断ログを書いて抜ける。
func (h *Host) CloseHumans(msg string) {
	h.mu.Lock()
	targets := make([]*session.Session, 0, len(h.online))
	for _, s := range h.online {
		if s.IsAgent() {
			continue
		}
		targets = append(targets, s)
	}
	h.mu.Unlock()
	for _, s := range targets {
		if msg != "" {
			s.Notify(session.Notice{Kind: session.NoticeSystem, Time: time.Now(), Body: msg})
		}
		s.Close()
	}
}
