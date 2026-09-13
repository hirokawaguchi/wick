package host

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

var (
	ErrOffline   = errors.New("not online")
	ErrNoChannel = errors.New("no channel")
)

const DefaultChatRooms = 8

type ChatRoomInfo struct {
	Num     int
	Title   string
	Members []string
}

type chatRoom struct {
	Num     int
	Title   string
	Members map[string]*session.Session
}

func (h *Host) initChat(n int) {
	if n <= 0 {
		n = DefaultChatRooms
	}
	if n > 32 {
		n = 32
	}
	h.rooms = make([]*chatRoom, n)
	for i := 0; i < n; i++ {
		h.rooms[i] = &chatRoom{
			Num:     i + 1,
			Title:   "Room " + strconv.Itoa(i+1),
			Members: map[string]*session.Session{},
		}
	}
}

func (h *Host) Session(id string) *session.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessionLocked(id)
}

func (h *Host) sessionLocked(id string) *session.Session {
	if s, ok := h.online[id]; ok {
		return s
	}
	for k, s := range h.online {
		if strings.EqualFold(k, id) {
			return s
		}
	}
	return nil
}

func (h *Host) sessionByChanLocked(n int) *session.Session {
	for _, s := range h.online {
		if s.Chan == n {
			return s
		}
	}
	return nil
}

func (h *Host) lookupDestLocked(to string) (*session.Session, error) {
	if n, err := strconv.Atoi(to); err == nil && n > 0 && to == strconv.Itoa(n) {
		if s := h.sessionByChanLocked(n); s != nil {
			return s, nil
		}
	}
	if s := h.sessionLocked(to); s != nil {
		return s, nil
	}
	if n, err := strconv.Atoi(to); err == nil && n > 0 {
		return nil, ErrNoChannel
	}
	return nil, ErrOffline
}

func (h *Host) SendTelegram(to string, from *session.Session, body string) error {
	body = session.ClipRunes(strings.TrimSpace(body), session.TelegramMax)
	if body == "" {
		return errors.New("empty")
	}
	h.mu.Lock()
	dst, err := h.lookupDestLocked(to)
	h.mu.Unlock()
	if err != nil {
		return err
	}
	n := session.Notice{
		Kind:   session.NoticeTelegram,
		FromID: from.User.ID,
		Handle: from.User.Handle,
		Time:   time.Now(),
		Body:   body,
	}
	dst.Notify(n)
	return nil
}

// Kill は sysop 用。回線番号か ID で在室を探し、切断通知を送ってから強制切断する。
// 戻り値は切断した相手の ID。
func (h *Host) Kill(sel string, from *session.Session) (string, error) {
	h.mu.Lock()
	dst, err := h.lookupDestLocked(sel)
	h.mu.Unlock()
	if err != nil {
		return "", err
	}
	fromID := ""
	if from != nil {
		fromID = from.User.ID
	}
	dst.Notify(session.Notice{
		Kind:   session.NoticeSystem,
		FromID: fromID,
		Time:   time.Now(),
		Body:   "sysop により切断されました。",
	})
	id := dst.User.ID
	dst.Close()
	return id, nil
}

func (h *Host) ListRooms() []ChatRoomInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ChatRoomInfo, 0, len(h.rooms))
	for _, r := range h.rooms {
		info := ChatRoomInfo{Num: r.Num, Title: r.Title}
		for id := range r.Members {
			info.Members = append(info.Members, id)
		}
		out = append(out, info)
	}
	return out
}

func (h *Host) JoinChat(num int, s *session.Session) (ChatRoomInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(num)
	if err != nil {
		return ChatRoomInfo{}, err
	}
	if s.TalkRoom != 0 {
		h.leaveTalkLocked(s.User.ID)
	}
	if s.ChatRoom != 0 && s.ChatRoom != num {
		h.leaveChatLocked(s.User.ID)
		room, err = h.roomLocked(num)
		if err != nil {
			return ChatRoomInfo{}, err
		}
	}
	if _, ok := room.Members[s.User.ID]; !ok {
		room.Members[s.User.ID] = s
		s.ChatRoom = num
		h.broadcastLocked(room, session.Notice{
			Kind:   session.NoticeChatJoin,
			FromID: s.User.ID,
			Handle: s.User.Handle,
			Room:   num,
			Title:  room.Title,
		}, s.User.ID)
	}
	return ChatRoomInfo{Num: room.Num, Title: room.Title, Members: memberIDs(room)}, nil
}

func (h *Host) LeaveChat(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveChatLocked(id)
}

func (h *Host) leaveChatLocked(id string) {
	s := h.sessionLocked(id)
	handle := ""
	if s != nil {
		handle = s.User.Handle
		s.ChatRoom = 0
	}
	for _, room := range h.rooms {
		if _, ok := room.Members[id]; !ok {
			continue
		}
		delete(room.Members, id)
		h.broadcastLocked(room, session.Notice{
			Kind:   session.NoticeChatLeave,
			FromID: id,
			Handle: handle,
			Room:   room.Num,
		}, "")
	}
}

func (h *Host) SayChat(num int, from *session.Session, body string) error {
	body = session.ClipRunes(strings.TrimSpace(body), session.ChatMax)
	if body == "" {
		return errors.New("empty")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(num)
	if err != nil {
		return err
	}
	if _, ok := room.Members[from.User.ID]; !ok {
		return errors.New("not in room")
	}
	h.broadcastLocked(room, session.Notice{
		Kind:   session.NoticeChat,
		FromID: from.User.ID,
		Handle: from.User.Handle,
		Time:   time.Now(),
		Body:   body,
		Room:   num,
	}, from.User.ID)
	return nil
}

func (h *Host) SetChatTitle(num int, title string) error {
	title = session.ClipWidth(strings.TrimSpace(title), store.MaxTitle)
	if title == "" {
		return errors.New("empty")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(num)
	if err != nil {
		return err
	}
	room.Title = title
	return nil
}

// HumanInRoom は部屋 num に人間（非エージェント）の在室者がいるかを返す。
// AI 同士の会話は「人が見ているとき（金魚鉢の観客がいるとき）」だけ活発化させ、
// 誰も見ておらず記録も残らない無人の部屋では静観させるために使う。
func (h *Host) HumanInRoom(num int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	room, err := h.roomLocked(num)
	if err != nil {
		return false
	}
	for _, s := range room.Members {
		if s != nil && !s.IsAgent() {
			return true
		}
	}
	return false
}

func (h *Host) roomLocked(num int) (*chatRoom, error) {
	if num < 1 || num > len(h.rooms) {
		return nil, errors.New("no such room")
	}
	return h.rooms[num-1], nil
}

func (h *Host) broadcastLocked(room *chatRoom, n session.Notice, skip string) {
	for id, s := range room.Members {
		if skip != "" && strings.EqualFold(id, skip) {
			continue
		}
		s.Notify(n)
	}
}

func memberIDs(room *chatRoom) []string {
	out := make([]string, 0, len(room.Members))
	for id := range room.Members {
		out = append(out, id)
	}
	return out
}
