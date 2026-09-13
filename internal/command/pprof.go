package command

import (
	"strings"

	"github.com/hirokawaguchi/wick/internal/store"
)

func cmdRegprof(e *Env) error {
	cur := strings.TrimRight(e.Sess.User.Profile, "\n")
	if cur == "" {
		e.Sess.Print(e.Sess.T("pprof.cur") + " " + e.Sess.T("none_paren") + "\n")
	} else {
		e.Sess.Print(e.Sess.T("pprof.cur") + "\n")
		e.Sess.Print(cur + "\n")
	}
	e.Sess.Print(e.Sess.T("pprof.edit") + "\n")
	text, ok, err := editField(e, cur, store.MaxProfLines, 78)
	if err != nil {
		return err
	}
	if !ok {
		e.Sess.Print(e.Sess.T("unchanged") + "\n")
		return nil
	}
	if err := e.Store.UpdateProfile(e.Ctx, e.Sess.User.ID, text); err != nil {
		return err
	}
	e.Sess.User.Profile = text
	if text == "" {
		e.Sess.Print(e.Sess.T("pprof.deleted") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("saved") + "\n")
	return nil
}

func cmdProfile(e *Env) error {
	id := strings.ToLower(strings.TrimSpace(e.Args))
	if id == "" {
		e.Sess.Print(e.Sess.T("signup.id_prompt"))
		line, err := e.Sess.ReadCommand(16)
		if err != nil {
			return err
		}
		id = strings.ToLower(strings.TrimSpace(line))
	}
	if id == "" {
		e.Sess.Print("usage: profile [id]\n")
		return nil
	}
	u, err := e.Store.GetUser(e.Ctx, id)
	if err != nil {
		e.Sess.Print(e.Sess.T("mail.no_user") + "\n")
		return nil
	}
	printProfile(e, u)
	return nil
}

func cmdReadprof(e *Env) error {
	users, err := e.Store.ListUsers(e.Ctx)
	if err != nil {
		return err
	}
	n := 0
	for _, u := range users {
		if strings.TrimSpace(u.Profile) == "" {
			continue
		}
		printProfile(e, u)
		n++
	}
	if n == 0 {
		e.Sess.Print(e.Sess.T("pprof.none") + "\n")
	}
	return nil
}

func cmdSearchprof(e *Env) error {
	kw := strings.TrimSpace(e.Args)
	if kw == "" {
		e.Sess.Print(e.Sess.T("pprof.keyword"))
		line, err := e.Sess.ReadLine(40)
		if err != nil {
			return err
		}
		kw = strings.TrimSpace(line)
	}
	if kw == "" {
		e.Sess.Print(e.Sess.T("pprof.search_usage") + "\n")
		return nil
	}
	users, err := e.Store.ListUsers(e.Ctx)
	if err != nil {
		return err
	}
	fold := strings.ToLower(kw)
	n := 0
	for _, u := range users {
		if !strings.Contains(strings.ToLower(u.Profile), fold) {
			continue
		}
		printProfile(e, u)
		n++
	}
	if n == 0 {
		e.Sess.Print(e.Sess.T("pprof.no_hit") + "\n")
	}
	return nil
}

func printProfile(e *Env, u store.User) {
	e.Sess.Printf("\n-- %s (%s) --\n", u.ID, u.Handle)
	text := strings.TrimRight(u.Profile, "\n")
	if text == "" {
		e.Sess.Print(e.Sess.T("none_paren") + "\n")
		return
	}
	e.Sess.Print(text + "\n")
}
