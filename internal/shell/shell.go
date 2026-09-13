package shell

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/menu"
	"github.com/hirokawaguchi/wick/internal/reason"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

const Version = command.Version

type Runner struct {
	Host   *host.Host
	Store  store.Store
	ACL    *acl.Table
	Assets assets.Dir
	Agents command.AgentControl // エージェント常駐の制御（agent コマンド用。nil 可）
}

func (r *Runner) Run(ctx context.Context, s *session.Session) int {
	s.Login = time.Now()
	if s.User.LastMsgRead != nil {
		s.Sequencer = *s.User.LastMsgRead
	}
	s.Print(candleBanner(r.Assets, string(s.Lang), Version))
	s.Printf("Welcome, %s.\n", s.User.Handle)
	if s.User.PWErr > 0 {
		s.Printf("Password Error : %d\n", s.User.PWErr)
	}
	if !s.User.Unlimited() {
		s.Printf("Time limit: %d min\n", s.User.TLimit)
	}
	if text, err := r.Assets.ReadLang(string(s.Lang), "msg", "login.msg"); err == nil {
		s.Print(expandLogin(text, s.User))
	}
	if r.Store != nil {
		if n, err := r.Store.CountUnreadMail(ctx, s.User.ID); err == nil && n > 0 {
			s.Print(s.T("login.unread_mail", n) + "\n")
		}
		if n, err := r.Store.CountUnreadNews(ctx, s.User.ID); err == nil && n > 0 {
			s.Print(s.T("login.unread_news", n) + "\n")
		}
	}
	s.Print("\n")

	sctx := ctx
	cancel := func() {}
	if !s.User.Unlimited() && s.User.TLimit > 0 {
		sctx, cancel = context.WithTimeout(ctx, time.Duration(s.User.TLimit)*time.Minute)
	}
	defer cancel()

	env := &command.Env{
		Ctx:    sctx,
		Sess:   s,
		Host:   r.Host,
		Store:  r.Store,
		ACL:    r.ACL,
		Assets: r.Assets,
		Agents: r.Agents,
	}
	if isGuest(s.User) {
		return r.runGuest(sctx, env, s)
	}

	err := (&menu.Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	switch {
	case errors.Is(err, command.ErrLogOff), err == nil:
		s.Print("Goodbye.\n")
		return reason.LogOff
	case errors.Is(err, io.EOF):
		return reason.CarrierDown
	case errors.Is(err, context.DeadlineExceeded):
		s.Print("\n## time out ##\n")
		return reason.TimeOut
	default:
		if sctx.Err() != nil && errors.Is(sctx.Err(), context.DeadlineExceeded) {
			s.Print("\n## time out ##\n")
			return reason.TimeOut
		}
		if sctx.Err() != nil {
			return reason.CarrierDown
		}
		return reason.SystemError
	}
}

// isGuest はサインアップ専用の共有ゲスト口（gst のみ）かどうか。
// 会員ビット（gen/sys/cos/pro）を一切持たないときだけ真。
func isGuest(u store.User) bool {
	return u.Flags&acl.FlagGst != 0 &&
		u.Flags&(acl.FlagGen|acl.FlagSys|acl.FlagCos|acl.FlagPro) == 0
}

// runGuest はゲスト口を signup と off だけの最小ループに閉じ込める。
// MAIN も他メニューも見せない（不正アクセス面を絞る）。
func (r *Runner) runGuest(ctx context.Context, env *command.Env, s *session.Session) int {
	reg := command.NewRegistry()
	for {
		s.Print("\n" + s.T("guest.menu"))
		s.Print("guest> ")
		line, err := readGuestLine(ctx, s)
		if errors.Is(err, session.ErrInterrupt) {
			continue // Ctrl-C は無視（ゲスト口では中止対象が無い）
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				s.Print("\n## time out ##\n")
				return reason.TimeOut
			}
			return reason.CarrierDown
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "1", "signup", "s":
			env.Args = ""
			env.Sess.SetDoing("SIGNUP")
			if e := reg.Dispatch(env, "signup"); errors.Is(e, io.EOF) {
				return reason.CarrierDown
			}
		case "2", "off", "o", ".", "/", "q", "quit", "bye":
			s.Print("Goodbye.\n")
			return reason.LogOff
		case "version", "v":
			s.Printf("Wick %s\n", Version)
		case "":
			continue
		default:
			s.Print(s.T("guest.only") + "\n")
		}
	}
}

func readGuestLine(ctx context.Context, s *session.Session) (string, error) {
	type rec struct {
		s string
		e error
	}
	ch := make(chan rec, 1)
	go func() {
		line, err := s.ReadCommand(16)
		ch <- rec{line, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		return r.s, r.e
	}
}

func expandLogin(text string, u store.User) string {
	remain := "unlimited"
	if !u.Unlimited() {
		remain = strconv.Itoa(u.TLimit)
	}
	last := "-"
	if u.LastLogout != nil {
		last = u.LastLogout.Format("2006-01-02 15:04")
	}
	now := time.Now()
	return strings.NewReplacer(
		`\ID`, u.ID,
		`\HANDLE`, u.Handle,
		`\ACCESS`, strconv.Itoa(u.Access),
		`\COMER`, strconv.Itoa(u.Access),
		`\LASTLOGOUT`, last,
		`\DATE`, now.Format("2006-01-02"),
		`\TIME`, now.Format("15:04:05"),
		`\REMAIN`, remain,
	).Replace(text)
}

// candleBanner はログイン時のオープニングヘッダ。局名 Wick（＝ろうそくの芯）に
// ちなみ、火のともったろうそくのドットアートを出す。
//
// 見た目は外部アセット data/<lang>/msg/banner.txt で差し替えられる（カスタマイズ点）。
// ファイル中では色トークンと \VERSION を使える:
//
//	{Y}=明るい黄  {O}=橙  {W}=白  {D}=暗い灰  {X}/{R}=リセット   \VERSION=版番号
//
// アセットが無いときは素朴な "Wick <版>" にフォールバックする。
func candleBanner(as assets.Dir, lang, version string) string {
	text, err := as.ReadLang(lang, "msg", "banner.txt")
	if err != nil || strings.TrimSpace(text) == "" {
		return "\nWick " + version + "\n"
	}
	rep := strings.NewReplacer(
		"{Y}", "\x1b[93m", // 明るい黄（炎）
		"{O}", "\x1b[33m", // 橙（芯）
		"{W}", "\x1b[97m", // 白（ろう）
		"{D}", "\x1b[90m", // 暗い灰（影・台座）
		"{X}", "\x1b[0m",
		"{R}", "\x1b[0m",
		`\VERSION`, version,
	)
	return "\n" + rep.Replace(text) + "\n"
}
