package command

import (
	"errors"
	"strconv"
	"strings"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/warn"
)

func (r *Registry) registerMail() {
	r.Register("postmail", cmdPostmail)
	r.Register("readmail", cmdReadmail)
	r.Register("deletema", cmdDeletema)
	r.Register("killmail", cmdKillmail)
	r.Register("lookreco", cmdLookreco)
	r.Register("multipos", cmdMultipos)
	r.Register("regmbox", cmdRegmbox)
	r.Register("reggroup", cmdReggroup)
	r.Register("regprof", cmdRegprof)
	r.Register("searchprof", cmdSearchprof)
	r.Register("profile", cmdProfile)
	r.Register("readprof", cmdReadprof)
}

func cmdPostmail(e *Env) error {
	arg := strings.TrimSpace(e.Args)
	to, err := mailDest(e, arg)
	if err != nil || to == "" {
		return err
	}
	if _, err := e.Store.GetUser(e.Ctx, to); err != nil {
		e.Sess.Print("** そのユーザーはいません **\n")
		return nil
	}
	subj, body, ok, err := mailCompose(e)
	if err != nil || !ok {
		return err
	}
	return mailSendOne(e, to, subj, body)
}

func cmdMultipos(e *Env) error {
	arg := strings.TrimSpace(e.Args)
	if arg == "" {
		e.Sess.Print("宛先 (空白区切り / @グループ) : ")
		line, err := e.Sess.ReadCommand(80)
		if err != nil {
			return err
		}
		arg = strings.TrimSpace(line)
	}
	ids, err := expandMailDests(e, arg)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		if strings.Contains(arg, "@") {
			return nil
		}
		e.Sess.Print("usage: multipos <id ... | @group>\n")
		return nil
	}
	subj, body, ok, err := mailCompose(e)
	if err != nil || !ok {
		return err
	}
	for _, id := range ids {
		if err := mailSendOne(e, id, subj, body); err != nil {
			return err
		}
	}
	return nil
}

func cmdReadmail(e *Env) error {
	return mailBox(e, "受信箱", func(e *Env) ([]store.Mail, error) {
		return e.Store.ListInbox(e.Ctx, e.Sess.User.ID)
	}, mailShowInbox, true)
}

func cmdLookreco(e *Env) error {
	return mailBox(e, "送信控え", func(e *Env) ([]store.Mail, error) {
		return e.Store.ListSent(e.Ctx, e.Sess.User.ID)
	}, mailShowSent, false)
}

func cmdDeletema(e *Env) error {
	return mailPick(e, "削除", func(e *Env) ([]store.Mail, error) {
		return e.Store.ListInbox(e.Ctx, e.Sess.User.ID)
	}, mailListInbox, func(e *Env, m store.Mail) error {
		if err := e.Store.DeleteInboxMail(e.Ctx, m.ID, e.Sess.User.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				e.Sess.Print("no such mail\n")
				return nil
			}
			return err
		}
		e.Sess.Print("-- 削除しました --\n")
		return nil
	})
}

func cmdKillmail(e *Env) error {
	return mailPick(e, "撤回", func(e *Env) ([]store.Mail, error) {
		return e.Store.ListWithdrawable(e.Ctx, e.Sess.User.ID)
	}, mailListSent, func(e *Env, m store.Mail) error {
		if err := e.Store.KillMail(e.Ctx, m.ID, e.Sess.User.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				e.Sess.Print("** 既読のため撤回できません **\n")
				return nil
			}
			return err
		}
		e.Sess.Print("-- 撤回しました --\n")
		return nil
	})
}

func cmdRegmbox(e *Env) error {
	cur := "off"
	if e.Sess.User.MailSave {
		cur = "on"
	}
	e.Sess.Printf("送信控え now = %s\n", cur)
	line := strings.TrimSpace(session.FoldCommand(e.Args))
	if line == "" {
		e.Sess.Print("on / off (Enter=そのまま): ")
		got, err := e.Sess.ReadCommand(8)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(session.FoldCommand(got))
		if line == "" {
			return nil
		}
	}
	var save bool
	switch line {
	case "on", "1", "yes", "y":
		save = true
	case "off", "0", "no", "n":
		save = false
	default:
		e.Sess.Print("invalid\n")
		return nil
	}
	if err := e.Store.UpdateMailSave(e.Ctx, e.Sess.User.ID, save); err != nil {
		return err
	}
	e.Sess.User.MailSave = save
	if save {
		e.Sess.Print("送信控え = on\n")
	} else {
		e.Sess.Print("送信控え = off\n")
	}
	return nil
}

func cmdReggroup(e *Env) error {
	for {
		groups, err := e.Store.ListMailGroups(e.Ctx, e.Sess.User.ID)
		if err != nil {
			return err
		}
		e.Sess.Print("\n同報グループ\n")
		if len(groups) == 0 {
			e.Sess.Print("  (なし)\n")
		}
		for i, g := range groups {
			e.Sess.Printf("[%d] @%s  %s\n", i+1, g.Name, g.Members)
		}
		e.Sess.Print("名前 (Enter=終了  -番号|-名前=削除): ")
		line, err := e.Sess.ReadCommand(32)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(session.FoldCommand(line))
		if line == "" {
			return nil
		}
		if strings.HasPrefix(line, "-") {
			spec := strings.TrimPrefix(line, "-")
			name := spec
			if n, conv := strconv.Atoi(spec); conv == nil {
				if n < 1 || n > len(groups) {
					e.Sess.Print("invalid\n")
					continue
				}
				name = groups[n-1].Name
			}
			name = strings.TrimPrefix(name, "@")
			if err := e.Store.DeleteMailGroup(e.Ctx, e.Sess.User.ID, name); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					e.Sess.Print("no such group\n")
					continue
				}
				return err
			}
			e.Sess.Print("-- 削除しました --\n")
			continue
		}
		name := strings.TrimPrefix(line, "@")
		if name == "" {
			e.Sess.Print("invalid\n")
			continue
		}
		cur := ""
		if g, err := e.Store.GetMailGroup(e.Ctx, e.Sess.User.ID, name); err == nil {
			cur = g.Members
			e.Sess.Printf("現在: %s\n", cur)
		}
		e.Sess.Print("メンバー (空白区切り): ")
		mem, err := e.Sess.ReadCommand(80)
		if err != nil {
			return err
		}
		members := store.JoinMailMembers(store.ParseMailMembers(mem))
		if members == "" {
			members = cur
		}
		if err := e.Store.SaveMailGroup(e.Ctx, store.MailGroup{
			Owner: e.Sess.User.ID, Name: name, Members: members,
		}); err != nil {
			if errors.Is(err, store.ErrTooManyGroups) {
				e.Sess.Print("** グループは 8 までです **\n")
				continue
			}
			return err
		}
		e.Sess.Print("-- 保存しました --\n")
	}
}

func mailDest(e *Env, arg string) (string, error) {
	to := strings.ToLower(strings.TrimSpace(arg))
	if to == "" {
		e.Sess.Print("宛先 : ")
		line, err := e.Sess.ReadCommand(16)
		if err != nil {
			return "", err
		}
		to = strings.ToLower(strings.TrimSpace(line))
	}
	if to == "" {
		e.Sess.Print("usage: postmail [id]\n")
		return "", nil
	}
	return to, nil
}

func mailCompose(e *Env) (subj, body string, ok bool, err error) {
	e.Sess.Print("題 (Ctrl-C 中止) : ")
	line, err := e.Sess.ReadLine(store.MaxMailSubject)
	if err != nil {
		return "", "", false, err
	}
	subj = strings.TrimSpace(line)
	if subj == "" {
		subj = "(無題)"
	}
	subj = session.ClipRunes(subj, store.MaxMailSubject)
	raw, ok, err := composeBody(e)
	if err != nil || !ok {
		return "", "", false, err
	}
	body = store.ApplyAutosign(raw, e.Sess.User.Autosign)
	return subj, body, true, nil
}

func mailSendOne(e *Env, to, subj, body string) error {
	if _, err := e.Store.GetUser(e.Ctx, to); err != nil {
		e.Sess.Printf("** %s はいません **\n", to)
		return nil
	}
	_, err := e.Store.SendMail(e.Ctx, store.Mail{
		FromID:     e.Sess.User.ID,
		FromHandle: e.Sess.User.Handle,
		ToID:       to,
		Subject:    subj,
		Body:       body,
		Saved:      e.Sess.User.MailSave,
	})
	if errors.Is(err, store.ErrTooManyMails) {
		e.Sess.Printf("** %s の受信箱がいっぱいです **\n", to)
		return nil
	}
	if err != nil {
		return err
	}
	e.Sess.Printf("-- %s へ送信しました --\n", to)
	return nil
}

func expandMailDests(e *Env, spec string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	for _, tok := range strings.Fields(spec) {
		if strings.HasPrefix(tok, "@") {
			name := strings.TrimPrefix(tok, "@")
			g, err := e.Store.GetMailGroup(e.Ctx, e.Sess.User.ID, name)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					e.Sess.Printf("** グループ @%s はありません **\n", name)
					return nil, nil
				}
				return nil, err
			}
			for _, id := range store.ParseMailMembers(g.Members) {
				add(id)
			}
			continue
		}
		add(tok)
	}
	return ids, nil
}

type mailListFn func(*Env) ([]store.Mail, error)

func mailBox(e *Env, title string, list mailListFn, show func(*Env, store.Mail) error, markRead bool) error {
	// who に居場所を出す（読み終えたら元へ戻す）。
	prevDoing := e.Sess.GetDoing()
	e.Sess.SetDoing("MAIL " + title)
	defer e.Sess.SetDoing(prevDoing)
	// 操作の統一ガイド（入場時に一度）。三種の場で共通: i=一覧 / q=抜ける / ?=ヘルプ。
	e.Sess.Print("[ 番号=読む  i=一覧  q=抜ける  ?=ヘルプ ]\n")
	for {
		mails, err := list(e)
		if err != nil {
			return err
		}
		printMailList(e, title, mails, title == "受信箱")
		e.Sess.Printf("%s MAIL> ", title)
		line, err := e.Sess.ReadCommand(16)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(session.FoldCommand(line))
		switch {
		case line == "" || line == "dir" || line == "i":
			continue
		case line == "." || line == "q":
			return nil
		case line == "?" || line == "-?":
			Usage(e, "readmail")
			continue
		}
		n, conv := strconv.Atoi(line)
		if conv != nil || n < 1 || n > len(mails) {
			e.Sess.Print("no such mail\n[ i=一覧  q=抜ける  ?=ヘルプ ]\n")
			continue
		}
		m := mails[n-1]
		if err := show(e, m); err != nil {
			return err
		}
		if markRead {
			if err := e.Store.MarkMailRead(e.Ctx, m.ID); err != nil {
				return err
			}
		}
	}
}

func mailPick(e *Env, verb string, list mailListFn, printer func(*Env, []store.Mail), act func(*Env, store.Mail) error) error {
	for {
		mails, err := list(e)
		if err != nil {
			return err
		}
		printer(e, mails)
		e.Sess.Printf("番号 (%s)  [ i=一覧  q=終了 ]: ", verb)
		line, err := e.Sess.ReadCommand(16)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(session.FoldCommand(line))
		switch {
		case line == "" || line == "dir" || line == "i":
			continue
		case line == "." || line == "q":
			return nil
		}
		n, conv := strconv.Atoi(line)
		if conv != nil || n < 1 || n > len(mails) {
			e.Sess.Print("no such mail\n")
			continue
		}
		if err := act(e, mails[n-1]); err != nil {
			return err
		}
	}
}

func printMailList(e *Env, title string, mails []store.Mail, inbox bool) {
	e.Sess.Printf("\n[ %s ]\n", title)
	if inbox {
		e.Sess.Printf("  %2s  %-8s %-24s %s\n", "No", "From", "Subject", "Date")
	} else {
		e.Sess.Printf("  %2s  %-8s %-24s %s\n", "No", "To", "Subject", "Date")
	}
	if len(mails) == 0 {
		e.Sess.Print("  (empty)\n")
	}
	for i, m := range mails {
		who := m.ToID
		if inbox {
			who = m.FromID
		}
		mark := " "
		if inbox && m.ReadAt == nil {
			mark = "*"
		} else if !inbox && m.ReadAt != nil {
			mark = "R"
		}
		e.Sess.Printf("  %2d  %-8s %s %s %s\n", i+1, who, session.PadRight(clipMailSubj(m.Subject), 24), m.SentAt.Format("2006-01-02 15:04"), mark)
	}
	e.Sess.Print("番号で読む。dir 再表示。q で抜ける。\n")
}

func mailListInbox(e *Env, mails []store.Mail) { printMailList(e, "受信箱", mails, true) }
func mailListSent(e *Env, mails []store.Mail)  { printMailList(e, "送信控え", mails, false) }

func mailShowInbox(e *Env, m store.Mail) error { return printMail(e, m) }
func mailShowSent(e *Env, m store.Mail) error  { return printMail(e, m) }

func printMail(e *Env, m store.Mail) error {
	e.Sess.Printf("From: %s (%s)\n", m.FromID, m.FromHandle)
	e.Sess.Printf("To:   %s\n", m.ToID)
	e.Sess.Printf("Date: %s\n", m.SentAt.Format("2006-01-02 15:04:05"))
	e.Sess.Printf("Subj: %s\n\n", m.Subject)
	e.Sess.Print(m.Body)
	if !strings.HasSuffix(m.Body, "\n") {
		e.Sess.Print("\n")
	}
	if warn.HasLink(m.Body) {
		e.Sess.Print(warn.External + "\n")
	}
	return nil
}

func clipMailSubj(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return session.ClipWidth(s, 24)
}
