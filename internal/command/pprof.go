package command

import (
	"strings"

	"github.com/hirokawaguchi/wick/internal/store"
)

func cmdRegprof(e *Env) error {
	cur := strings.TrimRight(e.Sess.User.Profile, "\n")
	if cur == "" {
		e.Sess.Print("現在の公開プロフィール : (なし)\n")
	} else {
		e.Sess.Print("現在の公開プロフィール :\n")
		e.Sess.Print(cur + "\n")
	}
	e.Sess.Print("公開文を編集 (矢印で移動。送信は単独行の . / 中止は Ctrl-C / 空のまま . で削除):\n")
	text, ok, err := editField(e, cur, store.MaxProfLines, 78)
	if err != nil {
		return err
	}
	if !ok {
		e.Sess.Print("-- 変更しません --\n")
		return nil
	}
	if err := e.Store.UpdateProfile(e.Ctx, e.Sess.User.ID, text); err != nil {
		return err
	}
	e.Sess.User.Profile = text
	if text == "" {
		e.Sess.Print("-- プロフィールを消しました --\n")
		return nil
	}
	e.Sess.Print("-- 保存しました --\n")
	return nil
}

func cmdProfile(e *Env) error {
	id := strings.ToLower(strings.TrimSpace(e.Args))
	if id == "" {
		e.Sess.Print("ID : ")
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
		e.Sess.Print("** そのユーザーはいません **\n")
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
		e.Sess.Print("(公開プロフィールはありません)\n")
	}
	return nil
}

func cmdSearchprof(e *Env) error {
	kw := strings.TrimSpace(e.Args)
	if kw == "" {
		e.Sess.Print("キーワード : ")
		line, err := e.Sess.ReadLine(40)
		if err != nil {
			return err
		}
		kw = strings.TrimSpace(line)
	}
	if kw == "" {
		e.Sess.Print("usage: searchprof [語]\n")
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
		e.Sess.Print("ヒットなし\n")
	}
	return nil
}

func printProfile(e *Env, u store.User) {
	e.Sess.Printf("\n-- %s (%s) --\n", u.ID, u.Handle)
	text := strings.TrimRight(u.Profile, "\n")
	if text == "" {
		e.Sess.Print("(なし)\n")
		return
	}
	e.Sess.Print(text + "\n")
}
