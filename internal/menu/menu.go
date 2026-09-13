package menu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/session"
)

const MaxLevel = 8

var (
	ErrBack = errors.New("back")
	ErrTop  = errors.New("top")
)

type Engine struct {
	Reg *command.Registry
}

func (e *Engine) Enter(env *command.Env, name, param string, level int) error {
	if level > MaxLevel {
		env.Sess.Print(env.Sess.T("menu.too_deep") + "\n")
		return nil
	}
	name = strings.ToUpper(name)
	show := level > 1 || env.Sess.User.Expert == 0
	for {
		env.Sess.SetDoing(name)
		if show {
			e.dispMenu(env, name)
		}
		show = false
		env.Sess.Print(command.PromptOf(env.Sess.User, name))
		line, err := readLineCtx(env)
		if err == io.EOF {
			return err
		}
		if errors.Is(err, session.ErrInterrupt) {
			// メニューでは中止対象が無い。プロンプトを出し直すだけ。
			continue
		}
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			show = true
			continue
		}
		if line == "." || line == "/" {
			if level == 1 {
				env.Sess.Print(env.Sess.T("menu.at_top") + "\n")
				continue
			}
			if line == "/" {
				return ErrTop
			}
			return nil
		}
		err = e.multi(env, name, param, line, level)
		if errors.Is(err, command.ErrLogOff) || errors.Is(err, io.EOF) {
			return err
		}
		if errors.Is(err, ErrTop) && level != 1 {
			return ErrTop
		}
	}
}

func (e *Engine) multi(env *command.Env, menu, param, buf string, level int) error {
	for _, part := range splitSemi(buf) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		word, rest := splitWS(part)
		if isNum(word) {
			n, _ := strconv.Atoi(word)
			line, ok := e.menuLine(env, menu, n)
			if !ok {
				env.Sess.Print(env.Sess.T("menu.no_item") + "\n")
				continue
			}
			if err := e.multi(env, menu, param, expand(line, rest), level); err != nil {
				return err
			}
			continue
		}
		args := expand(rest, param)
		if e.hasMenu(env, word) {
			err := e.Enter(env, word, args, level+1)
			if errors.Is(err, ErrTop) && level != 1 {
				return ErrTop
			}
			if err != nil && !errors.Is(err, ErrBack) {
				return err
			}
			continue
		}
		if err := e.runCommand(env, word, args); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) runCommand(env *command.Env, name, args string) error {
	if strings.HasSuffix(name, "?") {
		pref := strings.TrimSuffix(name, "?")
		// `? コマンド名` はそのコマンドの詳しい使い方（cmd -? と同じ）。
		if pref == "" {
			if arg := strings.TrimSpace(args); arg != "" {
				target := strings.Fields(arg)[0]
				if cmd, ok := env.ACL.LookupCommand(target); ok {
					command.Usage(env, cmd.Name)
				} else {
					command.Usage(env, strings.ToLower(target))
				}
				return nil
			}
		}
		// それ以外は説明つきの一覧（簡易ヘルプ）。
		e.helpList(env, pref)
		return nil
	}
	if strings.Contains(args, "-?") {
		cmd, ok := env.ACL.LookupCommand(name)
		if ok {
			command.Usage(env, cmd.Name)
		} else {
			command.Usage(env, strings.ToLower(name))
		}
		return nil
	}
	cmd, ok := env.ACL.LookupCommand(name)
	if !ok {
		env.Sess.Print(env.Sess.T("menu.unknown_cmd", name) + "\n")
		return nil
	}
	if !acl.Allowed(env.Sess.User.Flags, cmd.Allow) {
		return command.Denied(env, cmd.Name)
	}
	if cmd.Alias != "" {
		return e.multi(env, env.Sess.GetDoing(), args, expand(cmd.Alias, args), 1)
	}
	env.Sess.SetDoing(strings.ToUpper(cmd.Name))
	env.Args = args
	err := e.Reg.Dispatch(env, cmd.Name)
	if errors.Is(err, command.ErrLater) {
		return command.Later(env, cmd.Name)
	}
	if errors.Is(err, session.ErrInterrupt) {
		// コマンド実行中の Ctrl-C。今の処理を中止してメニューへ戻る。
		env.Sess.Print(env.Sess.T("menu.aborted") + "\n")
		return nil
	}
	return err
}

// helpList は説明つきのコマンド一覧を出す（簡易ヘルプ）。pref が空なら全件、
// 非空ならその前方一致だけ。各行は「name  説明」。詳しくは `? 名前` か `名前 -?`。
func (e *Engine) helpList(env *command.Env, pref string) {
	cmds := env.ACL.PrefixCommands(pref, env.Sess.User.Flags)
	if len(cmds) == 0 {
		env.Sess.Print(env.Sess.T("help.none", pref) + "\n")
		return
	}
	if pref == "" {
		env.Sess.Print(env.Sess.T("help.header") + "\n")
	}
	for _, c := range cmds {
		desc := command.UsageSummary(env, c.Name)
		if desc == "" && c.Alias != "" {
			// 別名は展開先の説明を借りて「<本体> の別名」と示す。
			target := strings.Fields(c.Alias)[0]
			if s := command.UsageSummary(env, target); s != "" {
				desc = fmt.Sprintf("%s（%s の別名）", s, target)
			} else {
				desc = fmt.Sprintf("%s の別名", target)
			}
		}
		if desc == "" {
			env.Sess.Printf("  %-12s\n", c.Name)
		} else {
			env.Sess.Printf("  %-12s %s\n", c.Name, desc)
		}
	}
}

func (e *Engine) hasMenu(env *command.Env, name string) bool {
	n := strings.ToUpper(name)
	if env.Assets.Exists("menu", n+".mnu") || env.Assets.Exists("menu", strings.ToLower(n)+".mnu") {
		return true
	}
	return len(e.notesItems(env, name)) > 0
}

func (e *Engine) menuFile(env *command.Env, name, ext string) (string, bool) {
	n := strings.ToUpper(name)
	lang := ""
	if env.Sess != nil {
		lang = string(env.Sess.Lang)
	}
	// 言語別メニュー（data/<lang>/menu/...）を優先し、無ければ基準(ja)へ落ちる。
	for _, cand := range []string{n + ext, strings.ToLower(n) + ext} {
		if env.Assets.ExistsLang(lang, "menu", cand) {
			text, err := env.Assets.ReadLang(lang, "menu", cand)
			if err == nil {
				return text, true
			}
		}
	}
	return "", false
}

func (e *Engine) dispMenu(env *command.Env, name string) {
	if items := e.notesItems(env, name); len(items) > 0 {
		env.Sess.Print(formatNotesMenu(items))
		return
	}
	text, ok := e.menuFile(env, name, ".txt")
	if ok {
		env.Sess.Print(text)
	}
}

func (e *Engine) menuLine(env *command.Env, name string, num int) (string, bool) {
	if num <= 0 {
		return "", false
	}
	if items := e.notesItems(env, name); len(items) > 0 {
		seen := 0
		for _, it := range items {
			if it.header {
				continue
			}
			seen++
			if seen == num {
				return it.cmd, true
			}
		}
		return "", false
	}
	text, ok := e.menuFile(env, name, ".mnu")
	if !ok {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// 1行目はプロンプト、2行目が [1]
	idx := num
	if idx < 0 || idx >= len(lines) {
		return "", false
	}
	line := strings.TrimSpace(lines[idx])
	return line, line != ""
}

func (e *Engine) PromptLine(env *command.Env, name string) string {
	text, ok := e.menuFile(env, name, ".mnu")
	if !ok {
		return name
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return name
	}
	p := strings.TrimSpace(lines[0])
	if p == "" {
		return name
	}
	return p
}

func splitSemi(s string) []string {
	return strings.Split(s, ";")
}

func splitWS(s string) (string, string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	i := 0
	for i < len(s) && !unicode.IsSpace(rune(s[i])) {
		i++
	}
	if i >= len(s) {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}

func isNum(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func expand(s, param string) string {
	if !strings.Contains(s, "%") {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.ReplaceAll(s, "%", param))
}

func readLineCtx(env *command.Env) (string, error) {
	type rec struct {
		s string
		e error
	}
	ch := make(chan rec, 1)
	go func() {
		s, e := env.Sess.ReadCommand(128)
		ch <- rec{s, e}
	}()
	select {
	case <-env.Ctx.Done():
		if errors.Is(env.Ctx.Err(), context.DeadlineExceeded) {
			return "", env.Ctx.Err()
		}
		return "", env.Ctx.Err()
	case r := <-ch:
		return r.s, r.e
	}
}
