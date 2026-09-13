package command

import (
	"errors"
	"strconv"
	"strings"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func (r *Registry) registerSignup() {
	r.Register("signup", cmdSignup)
	r.Register("useredit", cmdUseredit)
}

const newMemberTLimit = 30

// 会員IDの採番方式（昔のBBS流：ログインIDはユーザが選べず、登録順の連番）。
// 8桁固定 = 3文字プレフィックス + 5桁ゼロ詰め連番。例: prd00001。小文字。
const (
	memberIDPrefix = "prd"
	memberIDDigits = 5
)

// cmdSignup はゲスト口からの新規登録。見習い会員（pro）を作る。
// ID はユーザが選べず、登録順に自動採番する（prd00001…）。人に見える名前は
// ハンドル（自由入力）。ゲストのセッションは昇格しない。作成後は新 ID で繋ぎ直してもらう。
func cmdSignup(e *Env) error {
	e.Sess.Print("\n== 新規登録 ==\n")
	e.Sess.Print("ログイン ID は登録順に自動発行します（変更・指定はできません）。\n")
	e.Sess.Print("画面に出る名前は「ハンドル」で自由に決められます。\n")

	e.Sess.Print("ハンドル : ")
	hline, err := e.Sess.ReadLine(16)
	if err != nil {
		return err
	}
	handle := session.ClipWidth(strings.TrimSpace(hline), store.MaxHandle)
	if handle == "" {
		e.Sess.Print("-- 中止 --\n")
		return nil
	}

	pw, ok, err := signupPassword(e)
	if err != nil || !ok {
		return err
	}

	// ID を採番して作成。競合（同時登録）時は採番からやり直す。
	var id string
	for tries := 0; tries < 5; tries++ {
		id, err = e.Store.AllocMemberID(e.Ctx, memberIDPrefix, memberIDDigits)
		if err != nil {
			return err
		}
		cerr := e.Store.CreateUser(e.Ctx, store.User{
			ID: id, Handle: handle, Flags: acl.FlagPro, TLimit: newMemberTLimit,
		}, pw)
		if cerr == nil {
			break
		}
		if errors.Is(cerr, store.ErrAlreadyExists) {
			id = ""
			continue
		}
		return cerr
	}
	if id == "" {
		e.Sess.Print("** 登録が混み合っています。少し待って再度お試しください **\n")
		return nil
	}

	e.Sess.Printf("\n-- 登録しました --\n")
	e.Sess.Printf("あなたのログイン ID : %s   （ハンドル: %s）\n", id, handle)
	e.Sess.Print("次回からはこの ID でログインしてください（メモをお願いします）。\n")
	e.Sess.Print("いまはまだ見習い会員です。sysop が承認すると書き込めます。\n")
	e.Sess.Print("いったん切断し、新しい ID で繋ぎ直してください。\n")
	return nil
}

func signupPassword(e *Env) (string, bool, error) {
	e.Sess.Print("パスワード (4〜16 文字) : ")
	p1, err := e.Sess.ReadSecret(16)
	if err != nil {
		return "", false, err
	}
	e.Sess.Print("\nもう一度 : ")
	p2, err := e.Sess.ReadSecret(16)
	if err != nil {
		return "", false, err
	}
	e.Sess.Print("\n")
	if len([]rune(p1)) < 4 {
		e.Sess.Print("** 短すぎます **\n")
		return "", false, nil
	}
	if p1 != p2 {
		e.Sess.Print("** 一致しません **\n")
		return "", false, nil
	}
	return p1, true, nil
}

// cmdUseredit は sysop/co-sysop 用。会員のレベルと制限時間を変える。
// 見習い(pro) を gen へ承認するのが主用途。
func cmdUseredit(e *Env) error {
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
		return nil
	}
	u, err := e.Store.GetUser(e.Ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			e.Sess.Print("** そのユーザーはいません **\n")
			return nil
		}
		return err
	}
	for {
		e.Sess.Printf("\n    ID          : %s\n", u.ID)
		e.Sess.Printf("    handle      : %s\n", u.Handle)
		e.Sess.Printf("[1] level       : %s\n", levelName(u.Flags))
		e.Sess.Printf("[2] time limit  : %d\n", u.TLimit)
		e.Sess.Print("変更する項目番号 (Enter=保存して終了) : ")
		line, err := e.Sess.ReadCommand(4)
		if err != nil {
			return err
		}
		switch strings.TrimSpace(line) {
		case "":
			if err := e.Store.UpdateFlags(e.Ctx, u.ID, u.Flags); err != nil {
				return err
			}
			if err := e.Store.UpdateTLimit(e.Ctx, u.ID, u.TLimit); err != nil {
				return err
			}
			e.Sess.Print("-- 保存しました --\n")
			return nil
		case "1":
			e.Sess.Print("level (gen/pro/cos/sys/gst) : ")
			lv, err := e.Sess.ReadCommand(8)
			if err != nil {
				return err
			}
			if f, ok := levelFlag(strings.TrimSpace(lv)); ok {
				u.Flags = f
			} else {
				e.Sess.Print("invalid\n")
			}
		case "2":
			e.Sess.Print("time limit (分, 65535=無制限) : ")
			tl, err := e.Sess.ReadCommand(8)
			if err != nil {
				return err
			}
			if n, cerr := strconv.Atoi(strings.TrimSpace(tl)); cerr == nil && n >= 0 {
				u.TLimit = n
			} else {
				e.Sess.Print("invalid\n")
			}
		}
	}
}

func levelName(flags uint32) string {
	switch {
	case flags&acl.FlagSys != 0:
		return "sysop"
	case flags&acl.FlagCos != 0:
		return "co-sysop"
	case flags&acl.FlagGen != 0:
		return "一般"
	case flags&acl.FlagPro != 0:
		return "見習い"
	case flags&acl.FlagGst != 0:
		return "ゲスト"
	default:
		return "?"
	}
}

func levelFlag(name string) (uint32, bool) {
	switch strings.ToLower(name) {
	case "gen":
		return acl.FlagGen, true
	case "pro":
		return acl.FlagPro, true
	case "cos":
		return acl.FlagCos, true
	case "sys":
		return acl.FlagSys, true
	case "gst":
		return acl.FlagGst, true
	default:
		return 0, false
	}
}
