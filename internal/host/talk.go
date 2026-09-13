package host

import (
	"errors"
	"sort"
	"strings"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

const (
	TalkVisitor = 0
	TalkSeat    = 1
	TalkKnock   = 2
)

type TalkPresence struct {
	Num    int
	Role   int
	Seats  []string
	Knocks []string
}

type talkMember struct {
	sess *session.Session
	role int
}

type talkRoomLive struct {
	members map[string]*talkMember
}

func (h *Host) initTalk() {
	h.talk = map[int]*talkRoomLive{}
}

func (h *Host) talkLocked(num int) *talkRoomLive {
	r, ok := h.talk[num]
	if !ok {
		r = &talkRoomLive{members: map[string]*talkMember{}}
		h.talk[num] = r
	}
	return r
}

func (h *Host) JoinTalk(num, status int, s *session.Session) (TalkPresence, error) {
	if num < 1 || num > store.MaxTalkRooms {
		return TalkPresence{}, errors.New("no such room")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.ChatRoom != 0 {
		h.leaveChatLocked(s.User.ID)
	}
	if s.TalkRoom != 0 && s.TalkRoom != num {
		h.leaveTalkLocked(s.User.ID)
	}
	room := h.talkLocked(num)
	role := TalkSeat
	switch status {
	case store.TalkClosed:
		role = TalkVisitor
	case store.TalkLocked:
		role = TalkKnock
	}
	if m, ok := room.members[s.User.ID]; ok {
		s.TalkRoom = num
		return h.presenceLocked(num, m.role), nil
	}
	room.members[s.User.ID] = &talkMember{sess: s, role: role}
	s.TalkRoom = num
	kind := session.NoticeTalkJoin
	if role == TalkKnock {
		kind = session.NoticeTalkKnock
	}
	h.broadcastTalkLocked(num, session.Notice{
		Kind:   kind,
		FromID: s.User.ID,
		Handle: s.User.Handle,
		Room:   num,
	}, s.User.ID)
	return h.presenceLocked(num, role), nil
}

func (h *Host) LeaveTalk(id string) (int, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.leaveTalkLocked(id)
}

func (h *Host) leaveTalkLocked(id string) (int, []string) {
	s := h.sessionLocked(id)
	handle := ""
	num := 0
	if s != nil {
		handle = s.User.Handle
		num = s.TalkRoom
		s.TalkRoom = 0
	}
	if num == 0 {
		for n, room := range h.talk {
			if _, ok := room.members[id]; ok {
				num = n
				break
			}
		}
	}
	if num == 0 {
		return 0, nil
	}
	room := h.talkLocked(num)
	if _, ok := room.members[id]; !ok {
		return num, seatIDs(room)
	}
	delete(room.members, id)
	h.broadcastTalkLocked(num, session.Notice{
		Kind:   session.NoticeTalkLeave,
		FromID: id,
		Handle: handle,
		Room:   num,
	}, "")
	return num, seatIDs(room)
}

func (h *Host) TalkRole(num int, id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.talk[num]
	if room == nil {
		return -1
	}
	m, ok := room.members[id]
	if !ok {
		return -1
	}
	return m.role
}

func (h *Host) ListTalkPresence(num int) TalkPresence {
	h.mu.Lock()
	defer h.mu.Unlock()
	role := -1
	return h.presenceLocked(num, role)
}

func (h *Host) KnockTalk(num int, s *session.Session) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.talk[num]
	if room == nil {
		return errors.New("not in room")
	}
	m, ok := room.members[s.User.ID]
	if !ok {
		return errors.New("not in room")
	}
	if m.role == TalkSeat {
		return errors.New("already seated")
	}
	m.role = TalkKnock
	h.broadcastTalkLocked(num, session.Notice{
		Kind:   session.NoticeTalkKnock,
		FromID: s.User.ID,
		Handle: s.User.Handle,
		Room:   num,
	}, s.User.ID)
	return nil
}

func (h *Host) AdmitTalk(num int, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.talk[num]
	if room == nil {
		return errors.New("not in room")
	}
	m := memberLocked(room, id)
	if m == nil {
		return errors.New("not in room")
	}
	m.role = TalkSeat
	h.broadcastTalkLocked(num, session.Notice{
		Kind:   session.NoticeTalkAdmit,
		FromID: m.sess.User.ID,
		Handle: m.sess.User.Handle,
		Room:   num,
	}, "")
	return nil
}

func (h *Host) KickTalk(num int, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.talk[num]
	if room == nil {
		return errors.New("not in room")
	}
	if memberLocked(room, id) == nil {
		return errors.New("not in room")
	}
	_, _ = h.leaveTalkLocked(id)
	return nil
}

func (h *Host) SayTalk(num int, from *session.Session, ln store.TalkLine) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.talk[num]
	if room == nil {
		return errors.New("not in room")
	}
	m, ok := room.members[from.User.ID]
	if !ok || m.role != TalkSeat {
		return errors.New("no seat")
	}
	h.broadcastTalkLocked(num, session.Notice{
		Kind:   session.NoticeTalk,
		FromID: ln.Author,
		Handle: ln.Handle,
		Time:   ln.PostTime,
		Body:   ln.Body,
		Room:   num,
		Line:   ln.Num,
	}, from.User.ID)
	return nil
}

func (h *Host) presenceLocked(num, role int) TalkPresence {
	p := TalkPresence{Num: num, Role: role}
	room := h.talk[num]
	if room == nil {
		return p
	}
	for id, m := range room.members {
		switch m.role {
		case TalkSeat:
			p.Seats = append(p.Seats, id)
		case TalkKnock:
			p.Knocks = append(p.Knocks, id)
		}
	}
	sort.Strings(p.Seats)
	sort.Strings(p.Knocks)
	return p
}

func (h *Host) broadcastTalkLocked(num int, n session.Notice, skip string) {
	room := h.talk[num]
	if room == nil {
		return
	}
	for id, m := range room.members {
		if skip != "" && strings.EqualFold(id, skip) {
			continue
		}
		m.sess.Notify(n)
	}
}

func memberLocked(room *talkRoomLive, id string) *talkMember {
	if m, ok := room.members[id]; ok {
		return m
	}
	for k, m := range room.members {
		if strings.EqualFold(k, id) {
			return m
		}
	}
	return nil
}

func seatIDs(room *talkRoomLive) []string {
	out := make([]string, 0, len(room.members))
	for id, m := range room.members {
		if m.role == TalkSeat {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
