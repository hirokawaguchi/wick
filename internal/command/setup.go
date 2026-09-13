package command

import (
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/store"
)

// cmdLang は表示言語（ja / en）を切り替える。SETUP 配下（Nouvelle 追加。正本の
// 「ずらしたもの」参照）。選んだ言語は DB に保存し、以降このセッションへ即反映する。
// UI 文字列の翻訳は段階移行中で、未翻訳キーは自動で日本語へフォールバックする。
func cmdLang(e *Env) error {
	cur := i18n.Normalize(e.Sess.User.Lang)
	e.Sess.Printf(e.Sess.T("lang.current")+"\n", i18n.Label(cur))
	e.Sess.Print(e.Sess.T("lang.opt.ja") + "\n")
	e.Sess.Print(e.Sess.T("lang.opt.en") + "\n")

	arg := strings.TrimSpace(e.Args)
	if arg == "" {
		e.Sess.Print(e.Sess.T("lang.prompt"))
		got, err := e.Sess.ReadCommand(8)
		if err != nil {
			return err
		}
		arg = strings.TrimSpace(got)
	}
	if arg == "" {
		e.Sess.Print(e.Sess.T("unchanged") + "\n")
		return nil
	}
	var chosen i18n.Lang
	switch strings.ToLower(arg) {
	case "1", "ja", "jp", "日本語":
		chosen = i18n.JA
	case "2", "en", "english":
		chosen = i18n.EN
	default:
		e.Sess.Print(e.Sess.T("invalid") + "\n")
		return nil
	}
	if err := e.Store.UpdateLang(e.Ctx, e.Sess.User.ID, string(chosen)); err != nil {
		return err
	}
	e.Sess.User.Lang = string(chosen)
	e.Sess.Lang = chosen // 以降の出力は新しい言語で
	e.Sess.Printf(e.Sess.T("lang.saved")+"\n", i18n.Label(chosen))
	return nil
}

func cmdPrivate(e *Env) error {
	u := &e.Sess.User
	for {
		e.Sess.Print("\n  会員票（他の利用者には見えません）\n")
		e.Sess.Printf("[1] 本名     : %s\n", dash(u.RealName))
		e.Sess.Printf("[2] 生年月日 : %s\n", dash(u.Birthday))
		e.Sess.Printf("[3] 住所     : %s\n", dash(u.Address))
		e.Sess.Printf("[4] 電話     : %s\n", dash(u.Phone))
		e.Sess.Printf("[5] 備考     : %s\n", dash(u.Comment))
		e.Sess.Print("変更する項目番号 (Enter=保存): ")
		line, err := e.Sess.ReadCommand(4)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		switch line {
		case "1":
			e.Sess.Print("本名 : ")
			if v, err := e.Sess.ReadLine(32); err == nil {
				u.RealName = strings.TrimSpace(v)
			}
		case "2":
			e.Sess.Print("生年月日 (YYYY/MM/DD) : ")
			if v, err := e.Sess.ReadCommand(16); err == nil {
				v = strings.TrimSpace(v)
				if v != "" && !validBirthday(v) {
					e.Sess.Print("invalid\n")
					continue
				}
				u.Birthday = v
			}
		case "3":
			e.Sess.Print("住所 : ")
			if v, err := e.Sess.ReadLine(64); err == nil {
				u.Address = strings.TrimSpace(v)
			}
		case "4":
			e.Sess.Print("電話 : ")
			if v, err := e.Sess.ReadLine(20); err == nil {
				u.Phone = strings.TrimSpace(v)
			}
		case "5":
			e.Sess.Print("備考 : ")
			if v, err := e.Sess.ReadLine(64); err == nil {
				u.Comment = strings.TrimSpace(v)
			}
		default:
			e.Sess.Print("invalid\n")
		}
	}
	if err := e.Store.UpdatePrivate(e.Ctx, u.ID, u.RealName, u.Birthday, u.Address, u.Phone, u.Comment); err != nil {
		return err
	}
	e.Sess.Print("-- 保存しました --\n")
	return nil
}

func cmdScanlist(e *Env) error {
	items := store.ParseScanList(e.Sess.User.ScanList)
	for {
		e.Sess.Print("\n巡回するボード (空なら全ボード)\n")
		if len(items) == 0 {
			e.Sess.Print("  (なし = new は全ボード)\n")
		}
		for i, name := range items {
			e.Sess.Printf("[%d] %s\n", i+1, name)
		}
		e.Sess.Print("追加する名前 (Enter=保存  -番号=削除): ")
		line, err := e.Sess.ReadCommand(32)
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "-") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "-"))
			if err != nil || n < 1 || n > len(items) {
				e.Sess.Print("invalid\n")
				continue
			}
			items = append(items[:n-1], items[n:]...)
			continue
		}
		if len(items) >= store.MaxScanList {
			e.Sess.Print("** これ以上追加できません **\n")
			continue
		}
		items = store.ParseScanList(strings.Join(append(items, line), "\n"))
	}
	list := store.JoinScanList(items)
	if err := e.Store.UpdateScanList(e.Ctx, e.Sess.User.ID, list); err != nil {
		return err
	}
	e.Sess.User.ScanList = list
	e.Sess.Print("-- 保存しました --\n")
	return nil
}

func cmdRegsign(e *Env) error {
	cur := strings.TrimRight(e.Sess.User.Autosign, "\n")
	if cur == "" {
		e.Sess.Print("現在の署名 : (なし)\n")
	} else {
		e.Sess.Print("現在の署名 :\n")
		e.Sess.Print(cur + "\n")
	}
	e.Sess.Print("署名を編集 (矢印で移動。送信は単独行の . / 中止は Ctrl-C / 空のまま . で削除):\n")
	text, ok, err := editField(e, cur, 8, 78)
	if err != nil {
		return err
	}
	if !ok {
		e.Sess.Print("-- 変更しません --\n")
		return nil
	}
	if err := e.Store.UpdateAutosign(e.Ctx, e.Sess.User.ID, text); err != nil {
		return err
	}
	e.Sess.User.Autosign = text
	if text == "" {
		e.Sess.Print("-- 署名を消しました --\n")
		return nil
	}
	e.Sess.Print("-- 保存しました --\n")
	return nil
}

func dash(s string) string {
	if s == "" {
		return "(なし)"
	}
	return s
}

func validBirthday(s string) bool {
	for _, layout := range []string{"2006/01/02", "2006/1/2", "06/01/02"} {
		if _, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return true
		}
	}
	return false
}
