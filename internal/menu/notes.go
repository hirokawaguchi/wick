package menu

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

// notesNameCols は日本語名の左詰め桁数（この右に（英字ID）を並べる）。
const notesNameCols = 20

type notesItem struct {
	cmd    string
	label  string
	header bool // 見出し行（番号を振らない・選べない）
}

func (e *Engine) notesItems(env *command.Env, name string) []notesItem {
	if env == nil || env.Store == nil {
		return nil
	}
	ctx := env.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	boards, err := env.Store.ListBoards(ctx)
	if err != nil {
		return nil
	}
	flags := uint32(0)
	if env.Sess != nil {
		flags = env.Sess.User.Flags
	}
	var readable []store.Board
	for _, b := range boards {
		if b.CanRead(flags) {
			readable = append(readable, b)
		}
	}

	lang := i18n.Lang("")
	if env.Sess != nil {
		lang = env.Sess.Lang
	}
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "notes" {
		return notesTopItems(readable, categoryNames(env), lang)
	}
	return notesCatItems(readable, n)
}

// categoryNames は data/etc/CATEGORIES.txt の「英字ID  日本語名」対応表を読む。
// 無ければ空。1 行 1 カテゴリ、# はコメント。名前は空白/タブ以降すべて。
func categoryNames(env *command.Env) map[string]string {
	m := map[string]string{}
	if env == nil || env.Assets.Root == "" {
		return m
	}
	text, err := env.Assets.Read("etc", "CATEGORIES.txt")
	if err != nil {
		return m
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		if i <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i:])
		if key != "" && val != "" {
			m[key] = val
		}
	}
	return m
}

// boardLine は「日本語名 <英字ID>」の 1 行。名前が無ければ ID を名前に使う。
// ID は <…>（コマンドの (…) と区別する）。
func boardLine(cmd, jpName, id string) notesItem {
	if jpName == "" {
		jpName = id
	}
	return notesItem{
		cmd:   cmd,
		label: fmt.Sprintf("%s<%s>", session.PadRight(jpName, notesNameCols), id),
	}
}

// cmdItem は「ラベル (コマンド)」の 1 行。コマンドは (…)（ID の <…> と区別する）。
func cmdItem(cmd, label string) notesItem {
	return notesItem{
		cmd:   cmd,
		label: fmt.Sprintf("%s(%s)", session.PadRight(label, notesNameCols), cmd),
	}
}

func notesTopItems(boards []store.Board, names map[string]string, lang i18n.Lang) []notesItem {
	cats := map[string]bool{}
	var loose []store.Board
	for _, b := range boards {
		if p := boardPrefix(b.Name); p != "" {
			cats[p] = true
		} else {
			loose = append(loose, b)
		}
	}
	catList := make([]string, 0, len(cats))
	for p := range cats {
		catList = append(catList, p)
	}
	sort.Strings(catList)

	out := make([]notesItem, 0, len(catList)+len(loose)+2)
	for _, p := range catList {
		jp := names[p]
		if jp == "" {
			jp = p
		}
		// カテゴリ（複数ボードのまとまり）は <id.*> で示す。
		out = append(out, notesItem{
			cmd:   p,
			label: fmt.Sprintf("%s<%s.*>", session.PadRight(jp, notesNameCols), p),
		})
	}
	for _, b := range loose {
		out = append(out, boardLine("open "+b.Name, b.Desc, b.Name))
	}
	out = append(out,
		cmdItem("new", i18n.T(lang, "notes.unread_scan")),
		cmdItem("bbslist", i18n.T(lang, "notes.board_list")),
	)
	return out
}

func notesCatItems(boards []store.Board, prefix string) []notesItem {
	if prefix == "" {
		return nil
	}
	var out []notesItem
	for _, b := range boards {
		if boardPrefix(b.Name) != prefix {
			continue
		}
		out = append(out, boardLine("open "+b.Name, b.Desc, b.Name))
	}
	return out
}

func boardPrefix(name string) string {
	i := strings.Index(name, ".")
	if i <= 0 {
		return ""
	}
	return strings.ToLower(name[:i])
}

func formatNotesMenu(items []notesItem) string {
	var b strings.Builder
	b.WriteByte('\n')
	num := 0
	for _, it := range items {
		if it.header {
			b.WriteString(it.label)
			b.WriteByte('\n')
			continue
		}
		num++
		b.WriteString(fmt.Sprintf("[%d] %s\n", num, it.label))
	}
	b.WriteByte('\n')
	return b.String()
}
