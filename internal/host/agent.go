package host

import (
	"github.com/hirokawaguchi/wick/internal/session"
)

// EnterAgent は who にエージェントセッションを載せる（回線番号を割り当てる）。
// 人間の重複チェックは通さない（エージェント ID は一意な前提）。
// オーケストレーション（起動・心拍・停止）は internal/agent の Manager が持つ。
func (h *Host) EnterAgent(s *session.Session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
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

// LeaveAgent は who からエージェントを外す。
func (h *Host) LeaveAgent(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.online, id)
}
