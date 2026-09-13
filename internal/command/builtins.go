package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const Version = "0.9.0"

func (r *Registry) registerBuiltins() {
	r.Register("off", cmdOff)
	r.Register("who", cmdWho)
	r.Register("version", cmdVersion)
	r.Register("echo", cmdEcho)
	r.Register("expert", cmdExpert)
	r.Register("handle", cmdHandle)
	r.Register("password", cmdPassword)
	r.Register("terminal", cmdTerminal)
	r.Register("option", cmdOption)
	r.Register("userlist", cmdUserlist)
	r.Register("log", cmdLog)
	r.Register("ps", cmdWho)
	r.Register("whoami", cmdWhoami)
	r.Register("time", cmdTime)
	r.Register("shutdown", cmdShutdown)
	r.Register("kill", cmdKill)
	r.Register("power", cmdPower)
	r.Register("agent", cmdAgent)
	r.Register("lang", cmdLang)
}

// cmdPower は局の稼働情報（起動時刻・稼働時間・在室数）を出す。ACL で sys/cos に制限。
// 原典 power（処理速度測定）はコンテナ上で意味がないため uptime 系に寄せた（正本参照）。
func cmdPower(e *Env) error {
	e.Sess.Printf("Wick %s\n", Version)
	if e.Host != nil {
		e.Sess.Printf("起動時刻 : %s\n", e.Host.Started().Format("2006-01-02 15:04:05"))
		e.Sess.Printf("稼働時間 : %s\n", formatUptime(e.Host.Uptime()))
		e.Sess.Printf("在室     : %d / %d\n", e.Host.OnlineCount(), e.Host.Max())
	}
	return nil
}

func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	days := total / 86400
	h := (total % 86400) / 3600
	m := (total % 3600) / 60
	s := total % 60
	if days > 0 {
		return fmt.Sprintf("%d日 %02d:%02d:%02d", days, h, m, s)
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// cmdKill は sysop 用。回線番号か ID の在室を強制切断する。ACL で sys/cos に制限。
func cmdKill(e *Env) error {
	sel := strings.TrimSpace(e.Args)
	if sel == "" {
		e.Sess.Print("使い方: kill <回線番号|ID>（回線番号は who / ps で確認）\n")
		return nil
	}
	e.Sess.Printf("%s を切断します。よろしいですか? (y/N) : ", sel)
	ans, err := e.Sess.ReadCommand(4)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(ans), "y") {
		e.Sess.Print("-- 中止しました --\n")
		return nil
	}
	id, err := e.Host.Kill(sel, e.Sess)
	if err != nil {
		e.Sess.Printf("切断できません: %v\n", err)
		return nil
	}
	e.Sess.Printf("-- %s を切断しました --\n", id)
	return nil
}

// cmdShutdown は sysop 用の graceful 停止。在室へ一斉告知してから停止を要求する。
// ACL（COMMAND.TXT）で sys/cos のみに制限している。
func cmdShutdown(e *Env) error {
	e.Sess.Print("システムを停止します。よろしいですか? (y/N) : ")
	ans, err := e.Sess.ReadCommand(4)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(ans), "y") {
		e.Sess.Print("-- 中止しました --\n")
		return nil
	}
	msg := strings.TrimSpace(e.Args)
	if msg == "" {
		msg = "まもなくシステムを停止します。"
	}
	if e.Host != nil {
		e.Host.Broadcast(session.Notice{
			Kind:   session.NoticeSystem,
			FromID: e.Sess.User.ID,
			Handle: e.Sess.User.Handle,
			Time:   time.Now(),
			Body:   msg,
		})
		e.Host.RequestShutdown()
	}
	e.Sess.Print("-- 停止を要求しました --\n")
	return ErrLogOff
}

func cmdOff(*Env) error { return ErrLogOff }

func cmdVersion(e *Env) error {
	e.Sess.Printf("Wick %s\n", Version)
	return nil
}

func cmdWhoami(e *Env) error {
	text, err := e.Assets.ReadLang(string(e.Sess.Lang), "text", "whoami.txt")
	if err != nil {
		e.Sess.Printf("You are logged in as %s (%s).\n", e.Sess.User.ID, e.Sess.User.Handle)
		return nil
	}
	e.Sess.Print(text)
	if !strings.HasSuffix(text, "\n") {
		e.Sess.Print("\n")
	}
	return nil
}

func cmdTime(e *Env) error {
	e.Sess.Printf("%s\n", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}

func cmdEcho(e *Env) error {
	e.Sess.Print(e.Args + "\n")
	return nil
}

func cmdWho(e *Env) error {
	// 桁を揃える: Ch=右3、ID=8、Handle=MaxHandle、Type=6。ID は 8 桁に収める。
	e.Sess.Printf("%3s %s %s %s %s\n", "Ch",
		session.PadRight("ID", 8),
		session.PadRight("Handle", store.MaxHandle),
		session.PadRight("Type", 6),
		"Menu")
	for _, o := range e.Host.Who() {
		kind := "人"
		if o.Agent {
			kind = "AI"
		}
		e.Sess.Printf("%3d %s %s %s %s\n", o.Chan,
			session.PadRight(session.ClipWidth(o.ID, 8), 8),
			session.PadRight(o.Handle, store.MaxHandle),
			session.PadRight(kind, 6),
			o.Doing)
	}
	return nil
}

func cmdExpert(e *Env) error {
	e.Sess.Printf("expert now = %d (0=beginner, 1=normal, 2=expert)\n", e.Sess.User.Expert)
	line := strings.TrimSpace(e.Args)
	if line == "" {
		e.Sess.Print("new value: ")
		got, err := e.Sess.ReadCommand(8)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(got)
		if line == "" {
			return nil
		}
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 0 || n > 2 {
		e.Sess.Print("invalid\n")
		return nil
	}
	if err := e.Store.UpdateExpert(e.Ctx, e.Sess.User.ID, n); err != nil {
		return err
	}
	e.Sess.User.Expert = n
	e.Sess.Printf("expert = %d\n", n)
	return nil
}

func cmdHandle(e *Env) error {
	e.Sess.Printf("handle now = %s\n", e.Sess.User.Handle)
	h := strings.TrimSpace(e.Args)
	if h == "" {
		e.Sess.Print("new handle: ")
		line, err := e.Sess.ReadLine(16)
		if err != nil {
			return err
		}
		h = strings.TrimSpace(line)
		if h == "" {
			return nil
		}
	}
	if session.DisplayWidth(h) > store.MaxHandle {
		e.Sess.Printf("** 長すぎます（全角%d文字/半角%d文字まで） **\n", store.MaxHandle/2, store.MaxHandle)
		return nil
	}
	if err := e.Store.UpdateHandle(e.Ctx, e.Sess.User.ID, h); err != nil {
		return err
	}
	e.Sess.User.Handle = h
	e.Sess.Printf("handle = %s\n", h)
	return nil
}

func cmdPassword(e *Env) error {
	e.Sess.Print("old password: ")
	old, err := e.Sess.ReadSecret(32)
	if err != nil {
		return err
	}
	e.Sess.Print("\n")
	if bcrypt.CompareHashAndPassword([]byte(e.Sess.User.PasswordHash), []byte(old)) != nil {
		e.Sess.Print("mismatch\n")
		return nil
	}
	e.Sess.Print("new password: ")
	n1, err := e.Sess.ReadSecret(16)
	if err != nil {
		return err
	}
	e.Sess.Print("\n")
	e.Sess.Print("retype: ")
	n2, err := e.Sess.ReadSecret(16)
	if err != nil {
		return err
	}
	e.Sess.Print("\n")
	if n1 == "" || n1 != n2 {
		e.Sess.Print("not match\n")
		return nil
	}
	if err := e.Store.UpdatePassword(e.Ctx, e.Sess.User.ID, n1); err != nil {
		return err
	}
	u, err := e.Store.GetUser(e.Ctx, e.Sess.User.ID)
	if err == nil {
		e.Sess.User.PasswordHash = u.PasswordHash
	}
	e.Sess.Print("password updated\n")
	return nil
}

func cmdTerminal(e *Env) error {
	u := &e.Sess.User
	e.Sess.Printf("width=%d height=%d color=%v prompt=%q\n", u.TermWidth, u.TermHeight, u.Esc&1 != 0, u.Prompt)
	e.Sess.Print("width [enter=keep]: ")
	if v, ok := readInt(e, u.TermWidth); ok {
		u.TermWidth = v
	}
	e.Sess.Print("height [enter=keep]: ")
	if v, ok := readInt(e, u.TermHeight); ok {
		u.TermHeight = v
	}
	e.Sess.Print("color 0/1 [enter=keep]: ")
	if v, ok := readInt(e, u.Esc&1); ok {
		if v != 0 {
			u.Esc |= 1
		} else {
			u.Esc &^= 1
		}
	}
	e.Sess.Print("prompt [enter=keep]: ")
	if line, err := e.Sess.ReadLine(20); err == nil {
		if p := strings.TrimRight(line, "\n"); p != "" {
			u.Prompt = p
		}
	}
	_ = e.Store.UpdateTerminal(e.Ctx, u.ID, u.TermWidth, u.TermHeight, u.Esc, u.Prompt)
	e.Sess.Print("saved\n")
	return nil
}

func cmdOption(e *Env) error {
	for _, o := range e.ACL.Options {
		on := "off"
		if acl.Allowed(e.Sess.User.Flags, o.Allow) {
			on = "on"
		}
		e.Sess.Printf("%-8s %s\n", o.Name, on)
	}
	return nil
}

func cmdUserlist(e *Env) error {
	users, err := e.Store.ListUsers(e.Ctx)
	if err != nil {
		return err
	}
	e.Sess.Printf("%-8s %s %s\n", "ID", session.PadRight("Handle", store.MaxHandle), "Access")
	for _, u := range users {
		e.Sess.Printf("%-8s %s %d\n", u.ID, session.PadRight(u.Handle, store.MaxHandle), u.Access)
	}
	return nil
}

func cmdLog(e *Env) error {
	logs, err := e.Store.ListLogs(e.Ctx, 15)
	if err != nil {
		return err
	}
	for _, l := range logs {
		disc := "-"
		if l.DisconnectTime != nil {
			disc = l.DisconnectTime.Format("15:04:05")
		}
		e.Sess.Printf("%s %s %s reason=%d\n", l.UserID, l.ConnectTime.Format("15:04:05"), disc, l.Reason)
	}
	return nil
}

func readInt(e *Env, def int) (int, bool) {
	line, err := e.Sess.ReadCommand(8)
	if err != nil {
		return def, false
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, false
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return def, false
	}
	return n, true
}

// PromptOf はメニュー（機能選択）用のプロンプト。既定は「機能名:」で終える。
// 末尾 : がメニュー、末尾 > が機能内の処理、という統一ルール（正本参照）。
// terminal で自分の prompt を変えている人はその値を尊重する（$ が機能名に化ける）。
func PromptOf(u store.User, menu string) string {
	p := u.Prompt
	if p == "" {
		p = "$: "
	}
	return strings.ReplaceAll(p, "$", menu)
}

func Usage(e *Env, name string) {
	text, err := e.Assets.Read("help", name+".usg")
	if err != nil {
		e.Sess.Printf("%s: no usage\n", name)
		return
	}
	e.Sess.Print(text)
}

// UsageSummary は help/<name>.usg の 1 行目「name - 説明」から説明部分を返す。
// usg が無い／書式が違うときは空文字。簡易ヘルプ（?）の一覧に添える。
func UsageSummary(e *Env, name string) string {
	text, err := e.Assets.Read("help", name+".usg")
	if err != nil {
		return ""
	}
	head := text
	if i := strings.IndexByte(head, '\n'); i >= 0 {
		head = head[:i]
	}
	head = strings.TrimRight(head, "\r")
	if i := strings.Index(head, " - "); i >= 0 {
		return strings.TrimSpace(head[i+3:])
	}
	return ""
}

func Later(e *Env, name string) error {
	e.Sess.Printf("%s: まだ実装されていません\n", name)
	return nil
}

func Denied(e *Env, name string) error {
	e.Sess.Printf("%s: permission denied\n", name)
	return ErrDenied
}
