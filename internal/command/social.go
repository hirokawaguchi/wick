package command

import (
	"errors"
	"strconv"
	"strings"

	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func (r *Registry) registerSocial() {
	r.Register("!", cmdTelegram)
	r.Register("chat", cmdChat)
	r.Register("talk", cmdTalk)
	r.Register("roomlist", cmdRoomlist)
}

func cmdTelegram(e *Env) error {
	to, msg := splitHead(e.Args)
	if to == "" {
		e.Sess.Print(e.Sess.T("mail.dest"))
		line, err := e.Sess.ReadCommand(16)
		if err != nil {
			return err
		}
		to = strings.TrimSpace(line)
	}
	if to == "" {
		e.Sess.Print(e.Sess.T("social.tg_usage") + "\n")
		return nil
	}
	if msg == "" {
		e.Sess.Print(e.Sess.T("social.body_prompt"))
		line, err := e.Sess.ReadLine(session.TelegramMax)
		if err != nil {
			return err
		}
		msg = line
	}
	if err := e.Host.SendTelegram(to, e.Sess, msg); err != nil {
		if errors.Is(err, host.ErrNoChannel) {
			e.Sess.Print(e.Sess.T("social.no_channel") + "\n")
			return nil
		}
		if errors.Is(err, host.ErrOffline) {
			e.Sess.Print(e.Sess.T("social.offline") + "\n")
			return nil
		}
		e.Sess.Print(e.Sess.T("talk.send_fail") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("social.sent") + "\n")
	return nil
}

func cmdChat(e *Env) error {
	arg := strings.TrimSpace(e.Args)
	num := 0
	if arg != "" {
		n, err := strconv.Atoi(session.FoldCommand(arg))
		if err != nil || n < 1 {
			e.Sess.Print(e.Sess.T("social.chat_usage") + "\n")
			return nil
		}
		num = n
	} else {
		n, ok, err := pickRoomNum(e, func(e *Env) error {
			printRooms(e)
			return nil
		})
		if err != nil || !ok {
			return err
		}
		num = n
	}
	info, err := e.Host.JoinChat(num, e.Sess)
	if err != nil {
		e.Sess.Print(e.Sess.T("talk.no_room") + "\n")
		return nil
	}
	prev := e.Sess.GetDoing()
	e.Sess.SetDoing("CHAT" + strconv.Itoa(num))
	defer func() {
		e.Host.LeaveChat(e.Sess.User.ID)
		e.Sess.SetDoing(prev)
	}()
	e.Sess.Print(e.Sess.T("social.chat_enter", info.Num, info.Title) + "\n")
	if len(info.Members) > 0 {
		e.Sess.Printf("online: %s\n", strings.Join(info.Members, " "))
	}
	// 部屋内は自分の ID をプロンプトにする（例 "sysop> "）。発話行と同じ形で
	// 1 回だけ出るので、echoback（自分の発言の反射）は既定 OFF。/e で切替。
	prompt := e.Sess.User.ID + "> "
	echo := false
	for {
		line, err := e.Sess.ReadLinePrompt(prompt, session.ChatMax)
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			printRooms(e)
			continue
		}
		if line == "." || line == "q" || line == "/q" {
			e.Sess.Print(e.Sess.T("talk.leave") + "\n")
			return nil
		}
		if line == "/i" {
			// 一覧: 部屋一覧の再表示。三種の場で一覧に揃える（発言の場は /i）。
			printRooms(e)
			continue
		}
		if line == "/?" || line == "/help" {
			Usage(e, "chat")
			continue
		}
		if line == "/e" {
			echo = !echo
			if echo {
				e.Sess.Print("-- echoback on --\n")
			} else {
				e.Sess.Print("-- echoback off --\n")
			}
			continue
		}
		if strings.HasPrefix(line, "/!") {
			// 部屋を出ずに個人電報を送る（部屋内では素の "!" は発言になるため）。
			to, msg := splitHead(strings.TrimSpace(line[2:]))
			if to == "" || msg == "" {
				e.Sess.Print(e.Sess.T("social.tg_room_usage") + "\n")
				continue
			}
			if err := e.Host.SendTelegram(to, e.Sess, msg); err != nil {
				switch {
				case errors.Is(err, host.ErrNoChannel):
					e.Sess.Print(e.Sess.T("social.no_channel") + "\n")
				case errors.Is(err, host.ErrOffline):
					e.Sess.Print(e.Sess.T("social.offline") + "\n")
				default:
					e.Sess.Print(e.Sess.T("talk.send_fail") + "\n")
				}
				continue
			}
			e.Sess.Print(e.Sess.T("social.sent") + "\n")
			continue
		}
		if line == "/who" || strings.HasPrefix(line, "/who ") {
			rooms := e.Host.ListRooms()
			for _, r := range rooms {
				if r.Num == num {
					e.Sess.Printf("online: %s\n", strings.Join(r.Members, " "))
					break
				}
			}
			continue
		}
		if strings.HasPrefix(line, "/title") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "/title"))
			if title == "" {
				e.Sess.Print(e.Sess.T("social.title_usage") + "\n")
				continue
			}
			if err := e.Host.SetChatTitle(num, title); err != nil {
				e.Sess.Print(e.Sess.T("social.cant_change") + "\n")
				continue
			}
			e.Sess.Print(e.Sess.T("talk.title_set", session.ClipWidth(title, store.MaxTitle)) + "\n")
			continue
		}
		if err := e.Host.SayChat(num, e.Sess, line); err != nil {
			e.Sess.Print(e.Sess.T("talk.send_fail") + "\n")
			return nil
		}
		if echo {
			e.Sess.Print(e.Sess.FoldLine(e.Sess.User.ID+"> ",
				session.ClipRunes(strings.TrimSpace(line), session.ChatMax)))
		}
	}
}

func pickRoomNum(e *Env, list func(*Env) error) (int, bool, error) {
	for {
		if err := list(e); err != nil {
			return 0, false, err
		}
		e.Sess.Print(e.Sess.T("social.room_prompt"))
		line, err := e.Sess.ReadCommand(8)
		if err != nil {
			return 0, false, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "." || line == "q" {
			return 0, false, nil
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 {
			e.Sess.Print(e.Sess.T("invalid") + "\n")
			continue
		}
		return n, true, nil
	}
}

func printRooms(e *Env) {
	e.Sess.Print(" No  N  Title\n")
	for _, r := range e.Host.ListRooms() {
		e.Sess.Printf("[%2d] %2d  %s\n", r.Num, len(r.Members), r.Title)
	}
}

func splitHead(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	i := strings.IndexFunc(s, func(r rune) bool { return r == ' ' || r == '\t' })
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i+1:])
}
