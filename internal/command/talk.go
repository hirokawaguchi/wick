package command

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func cmdTalk(e *Env) error {
	scan, num, ok := parseTalkArgs(e.Args)
	if !ok {
		e.Sess.Print(e.Sess.T("talk.usage") + "\n")
		return nil
	}
	if err := e.Store.SeedTalkRooms(e.Ctx); err != nil {
		return err
	}
	if scan {
		return talkScan(e, num)
	}
	if num == 0 {
		n, ok, err := pickRoomNum(e, printTalkRooms)
		if err != nil || !ok {
			return err
		}
		num = n
	}
	return enterTalkRoom(e, num)
}

func cmdRoomlist(e *Env) error {
	if err := e.Store.SeedTalkRooms(e.Ctx); err != nil {
		return err
	}
	return printTalkRooms(e)
}

func parseTalkArgs(args string) (scan bool, num int, ok bool) {
	ok = true
	for _, f := range strings.Fields(session.FoldCommand(args)) {
		if f == "-n" || f == "-N" {
			scan = true
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 {
			return false, 0, false
		}
		num = n
	}
	return scan, num, true
}

func talkScan(e *Env, only int) error {
	rooms, err := e.Store.ListTalkRooms(e.Ctx)
	if err != nil {
		return err
	}
	shown := 0
	for _, r := range rooms {
		if only > 0 && r.Num != only {
			continue
		}
		if !r.LastUpdate.After(e.Sess.Sequencer) {
			continue
		}
		lines, err := e.Store.ListTalkLinesSince(e.Ctx, r.Num, e.Sess.Sequencer)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			continue
		}
		e.Sess.Printf("-- talk %d: %s [%s] (%d new) --\n", r.Num, r.Title, store.TalkStatusName(r.Status), len(lines))
		for _, ln := range lines {
			printTalkLine(e.Sess, ln)
			e.Sess.Seen(ln.PostTime)
		}
		shown++
	}
	if shown == 0 {
		e.Sess.Print(e.Sess.T("talk.no_unread") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("talk.scan_end") + "\n")
	return nil
}

func enterTalkRoom(e *Env, num int) error {
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			e.Sess.Print(e.Sess.T("talk.no_room") + "\n")
			return nil
		}
		return err
	}
	info, err := e.Host.JoinTalk(num, room.Status, e.Sess)
	if err != nil {
		e.Sess.Print(e.Sess.T("talk.no_room") + "\n")
		return nil
	}
	if info.Role == host.TalkSeat {
		if err := ensureTalkLeader(e, &room, e.Sess.User.ID); err != nil {
			return err
		}
	}
	prev := e.Sess.GetDoing()
	e.Sess.SetDoing("TALK" + strconv.Itoa(num))
	defer func() {
		left, seats := e.Host.LeaveTalk(e.Sess.User.ID)
		e.Sess.SetDoing(prev)
		if left == num {
			_ = transferTalkLeader(e, num, e.Sess.User.ID, seats)
		}
	}()

	e.Sess.Print(e.Sess.T("talk.enter", room.Num, room.Title, store.TalkStatusName(room.Status), talkRoleName(e.Sess, info.Role)) + "\n")
	// 操作の統一ガイド（入室時に一度）。三種の場で共通: /i=一覧 / /q=退出 / /?=ヘルプ。
	e.Sess.Print(e.Sess.T("talk.guide") + "\n")
	printTalkWho(e.Sess, info)
	if err := printRecentTalk(e, room); err != nil {
		return err
	}
	for {
		e.Sess.Printf("%d TALK> ", num)
		line, err := e.Sess.ReadLine(store.TalkLineMax)
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err := printTalkRooms(e); err != nil {
				return err
			}
			continue
		}
		if line == "." || line == "q" || line == "/q" {
			e.Sess.Print(e.Sess.T("talk.leave") + "\n")
			return nil
		}
		if strings.HasPrefix(line, "/") {
			if err := talkSlash(e, num, line); err != nil {
				return err
			}
			continue
		}
		if err := postTalkLine(e, num, line); err != nil {
			return err
		}
	}
}

func talkSlash(e *Env, num int, line string) error {
	cmd, arg := splitHead(line)
	switch strings.ToLower(cmd) {
	case "/?", "/help":
		Usage(e, "talk")
		return nil
	case "/who":
		printTalkWho(e.Sess, e.Host.ListTalkPresence(num))
		return nil
	case "/title":
		return talkSetTitle(e, num, arg)
	case "/open":
		return talkSetStatus(e, num, store.TalkOpen)
	case "/close":
		return talkSetStatus(e, num, store.TalkClosed)
	case "/lock":
		return talkSetStatus(e, num, store.TalkLocked)
	case "/knock":
		return talkKnock(e, num)
	case "/seat":
		return talkAdmit(e, num, arg)
	case "/kick":
		return talkKick(e, num, arg)
	case "/i":
		// 一覧: 部屋一覧の再表示。三種の場で一覧に揃える（発言の場は /i）。
		return printTalkRooms(e)
	case "/ra":
		return talkReadAll(e, num)
	default:
		e.Sess.Print(e.Sess.T("talk.unknown") + "\n")
		return nil
	}
}

func postTalkLine(e *Env, num int, body string) error {
	body = session.ClipRunes(strings.TrimSpace(body), store.TalkLineMax)
	if body == "" {
		return nil
	}
	if e.Host.TalkRole(num, e.Sess.User.ID) != host.TalkSeat {
		e.Sess.Print(e.Sess.T("talk.no_seat") + "\n")
		return nil
	}
	ln, err := e.Store.CreateTalkLine(e.Ctx, store.TalkLine{
		Room:   num,
		Author: e.Sess.User.ID,
		Handle: e.Sess.User.Handle,
		Body:   body,
	})
	if err != nil {
		if errors.Is(err, store.ErrTooManyTalkLines) {
			e.Sess.Print(e.Sess.T("talk.too_many") + "\n")
			return nil
		}
		return err
	}
	e.Sess.Seen(ln.PostTime)
	if err := e.Host.SayTalk(num, e.Sess, ln); err != nil {
		e.Sess.Print(e.Sess.T("talk.send_fail") + "\n")
	}
	return nil
}

func talkKnock(e *Env, num int) error {
	if e.Host.TalkRole(num, e.Sess.User.ID) == host.TalkSeat {
		e.Sess.Print(e.Sess.T("talk.already_seated") + "\n")
		return nil
	}
	if err := e.Host.KnockTalk(num, e.Sess); err != nil {
		e.Sess.Print(e.Sess.T("talk.knock_fail") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("talk.knocked") + "\n")
	return nil
}

func talkAdmit(e *Env, num int, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		e.Sess.Print("usage: /seat <id>\n")
		return nil
	}
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if !talkLeader(e, room) {
		e.Sess.Print(e.Sess.T("talk.not_leader") + "\n")
		return nil
	}
	if err := e.Host.AdmitTalk(num, id); err != nil {
		e.Sess.Print(e.Sess.T("talk.no_person") + "\n")
		return nil
	}
	if err := ensureTalkLeader(e, &room, id); err != nil {
		return err
	}
	e.Sess.Print(e.Sess.T("talk.seated", id) + "\n")
	return nil
}

func talkKick(e *Env, num int, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		e.Sess.Print("usage: /kick <id>\n")
		return nil
	}
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if !talkLeader(e, room) {
		e.Sess.Print(e.Sess.T("talk.not_leader") + "\n")
		return nil
	}
	if strings.EqualFold(id, e.Sess.User.ID) {
		e.Sess.Print(e.Sess.T("talk.no_self_kick") + "\n")
		return nil
	}
	if err := e.Host.KickTalk(num, id); err != nil {
		e.Sess.Print(e.Sess.T("talk.no_person") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("talk.kicked", id) + "\n")
	return nil
}

func talkSetTitle(e *Env, num int, title string) error {
	title = session.ClipWidth(strings.TrimSpace(title), store.MaxTitle)
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if title == "" {
		e.Sess.Printf("title: %s\n", room.Title)
		return nil
	}
	if !talkLeader(e, room) {
		e.Sess.Print(e.Sess.T("talk.not_leader") + "\n")
		return nil
	}
	room.Title = title
	if err := e.Store.UpdateTalkRoom(e.Ctx, room); err != nil {
		return err
	}
	e.Sess.Print(e.Sess.T("talk.title_set", title) + "\n")
	return nil
}

func talkSetStatus(e *Env, num, status int) error {
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if !talkLeader(e, room) {
		e.Sess.Print(e.Sess.T("talk.not_leader") + "\n")
		return nil
	}
	room.Status = status
	if err := e.Store.UpdateTalkRoom(e.Ctx, room); err != nil {
		return err
	}
	e.Sess.Print(e.Sess.T("talk.status_set", store.TalkStatusName(status)) + "\n")
	return nil
}

func talkReadAll(e *Env, num int) error {
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if room.LineCount > 40 {
		e.Sess.Print(e.Sess.T("talk.ra_warn", room.LineCount) + "\n")
		e.Sess.Print(e.Sess.T("talk.continue_q"))
		line, err := e.Sess.ReadCommand(8)
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			return nil
		}
	}
	lines, err := e.Store.ListTalkLines(e.Ctx, num, 1)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		e.Sess.Print(e.Sess.T("talk.no_log") + "\n")
		return nil
	}
	for _, ln := range lines {
		printTalkLine(e.Sess, ln)
		e.Sess.Seen(ln.PostTime)
	}
	return nil
}

func printRecentTalk(e *Env, room store.TalkRoom) error {
	from := room.LineCount - 19
	if from < 1 {
		from = 1
	}
	lines, err := e.Store.ListTalkLines(e.Ctx, room.Num, from)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	e.Sess.Print(e.Sess.T("talk.recent") + "\n")
	for _, ln := range lines {
		printTalkLine(e.Sess, ln)
		e.Sess.Seen(ln.PostTime)
	}
	return nil
}

func printTalkRooms(e *Env) error {
	rooms, err := e.Store.ListTalkRooms(e.Ctx)
	if err != nil {
		return err
	}
	e.Sess.Print(" No  St  Line  N  Title\n")
	for _, r := range rooms {
		p := e.Host.ListTalkPresence(r.Num)
		e.Sess.Printf("[%2d] %s %5d %2d  %s\n", r.Num, store.TalkStatusShort(r.Status), r.LineCount, len(p.Seats), r.Title)
	}
	return nil
}

func printTalkWho(s *session.Session, p host.TalkPresence) {
	if len(p.Seats) > 0 {
		s.Printf("seats: %s\n", strings.Join(p.Seats, " "))
	} else {
		s.Print("seats: (none)\n")
	}
	if len(p.Knocks) > 0 {
		s.Printf("knock: %s\n", strings.Join(p.Knocks, " "))
	}
}

func printTalkLine(s *session.Session, ln store.TalkLine) {
	s.Print(s.FoldLine(fmt.Sprintf("%4d %s> ", ln.Num, ln.Author), ln.Body))
}

func talkLeader(e *Env, room store.TalkRoom) bool {
	if e.Sess.User.Flags&(acl.FlagSys|acl.FlagCos) != 0 {
		return true
	}
	return strings.EqualFold(room.Leader, e.Sess.User.ID)
}

func ensureTalkLeader(e *Env, room *store.TalkRoom, id string) error {
	if room.Leader != "" {
		return nil
	}
	pick := strings.ToLower(id)
	p := e.Host.ListTalkPresence(room.Num)
	if len(p.Seats) > 0 {
		pick = strings.ToLower(p.Seats[0])
	}
	room.Leader = pick
	return e.Store.UpdateTalkRoom(e.Ctx, *room)
}

func transferTalkLeader(e *Env, num int, left string, seats []string) error {
	room, err := e.Store.GetTalkRoom(e.Ctx, num)
	if err != nil {
		return err
	}
	if !strings.EqualFold(room.Leader, left) {
		return nil
	}
	if len(seats) == 0 {
		return nil
	}
	sort.Strings(seats)
	room.Leader = seats[0]
	return e.Store.UpdateTalkRoom(e.Ctx, room)
}

func talkRoleName(s *session.Session, role int) string {
	switch role {
	case host.TalkSeat:
		return s.T("talk.role_seat")
	case host.TalkKnock:
		return s.T("talk.role_knock")
	default:
		return s.T("talk.role_watch")
	}
}
