package command

import (
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/notes"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func (r *Registry) registerNotes() {
	r.Register("open", cmdOpen)
	r.Register("bbslist", cmdBBSList)
	r.Register("board", cmdBoard)
	r.Register("setseque", cmdSetSeque)
	r.Register("private", cmdPrivate)
	r.Register("scanlist", cmdScanlist)
	r.Register("regsign", cmdRegsign)
}

func cmdOpen(e *Env) error {
	return notes.Open(&notes.Env{
		Ctx: e.Ctx, Sess: e.Sess, Host: e.Host, Store: e.Store, Assets: e.Assets,
	}, e.Args)
}

func cmdBBSList(e *Env) error {
	boards, err := e.Store.ListBoards(e.Ctx)
	if err != nil {
		return err
	}
	n := 0
	for _, b := range boards {
		if !b.CanRead(e.Sess.User.Flags) {
			continue
		}
		if n == 0 {
			// 記号の意味を先頭に1行だけ添える（R=読む W=書く B=新規話題）。
			e.Sess.Print(e.Sess.T("notes.bbslist_head") + "\n")
		}
		n++
		w, bn := ' ', ' '
		if b.CanWrite(e.Sess.User.Flags) {
			w = 'W'
		}
		if b.CanBasenote(e.Sess.User.Flags) {
			bn = 'B'
		}
		// 日本語名を主に、識別用の英字ID（ボード名）を <…> で添える。
		name := b.Desc
		if name == "" {
			name = b.Name
		}
		e.Sess.Printf(" (R%c%c) %s<%s>\n", w, bn, session.PadRight(name, 30), b.Name)
	}
	if n == 0 {
		e.Sess.Print(e.Sess.T("notes.no_readable") + "\n")
	}
	return nil
}

func cmdSetSeque(e *Env) error {
	cur := e.Sess.Sequencer
	e.Sess.Print(e.Sess.T("notes.seq_cur_val", formatSeq(cur)) + "\n")
	e.Sess.Print(e.Sess.T("notes.seq_input_hint") + "\n")
	e.Sess.Print(e.Sess.T("notes.seq_input"))
	line, err := e.Sess.ReadCommand(32)
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	var t time.Time
	if line == "" {
		if e.Sess.User.LastMsgRead != nil {
			t = *e.Sess.User.LastMsgRead
		}
	} else {
		t, err = parseSeq(line)
		if err != nil {
			e.Sess.Print(e.Sess.T("invalid") + "\n")
			return nil
		}
	}
	e.Sess.Sequencer = t
	e.Sess.Print(e.Sess.T("notes.seq_changed", formatSeq(t)) + "\n")
	return nil
}

func formatSeq(t time.Time) string {
	if t.IsZero() {
		return "(none)"
	}
	return t.Format("2006/01/02 15:04:05")
}

func parseSeq(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006/01/02 15:04:05",
		"2006/1/2 15:04:05",
		"06/01/02 15:04:05",
		"2006-01-02 15:04:05",
		"2006/01/02",
		"06/01/02",
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}

func cmdBoard(e *Env) error {
	name := strings.TrimSpace(e.Args)
	if name == "" {
		e.Sess.Print("boardname : ")
		line, err := e.Sess.ReadCommand(32)
		if err != nil {
			return err
		}
		name = strings.TrimSpace(line)
	}
	if name == "" {
		return nil
	}
	b, err := e.Store.GetBoard(e.Ctx, name)
	isNew := false
	if err == store.ErrNotFound {
		e.Sess.Print("-- new board --\n")
		b = store.Board{
			Name: name, Slug: name,
			Read: 0xffffffff, Write: 0xffffffff, Basenote: 0xffffffff,
		}
		isNew = true
	} else if err != nil {
		return err
	}
	for {
		e.Sess.Printf("\n    last update : %s\n", formatSeq(b.LastUpdate))
		e.Sess.Printf("    messages    : %d\n", b.MsgCount)
		e.Sess.Printf("[1] boardname   : %s\n", b.Name)
		e.Sess.Printf("[2] description : %s\n", b.Desc)
		e.Sess.Printf("[3] slug        : %s\n", b.Slug)
		e.Sess.Printf("[4] read        : %s\n", fmtMask(b.Read))
		e.Sess.Printf("[5] write       : %s\n", fmtMask(b.Write))
		e.Sess.Printf("[6] basenote    : %s\n", fmtMask(b.Basenote))
		e.Sess.Printf("[7] sign        : %s\n", signPreview(b.Sign))
		e.Sess.Print(e.Sess.T("notes.board_item"))
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
			e.Sess.Print("boardname : ")
			if v, err := e.Sess.ReadCommand(32); err == nil && strings.TrimSpace(v) != "" {
				b.Name = strings.TrimSpace(v)
			}
		case "2":
			e.Sess.Print("description : ")
			if v, err := e.Sess.ReadLine(64); err == nil {
				b.Desc = strings.TrimSpace(v)
			}
		case "3":
			e.Sess.Print("slug : ")
			if v, err := e.Sess.ReadCommand(16); err == nil && strings.TrimSpace(v) != "" {
				b.Slug = strings.TrimSpace(v)
			}
		case "4":
			b.Read = readMask(e, b.Read)
		case "5":
			b.Write = readMask(e, b.Write)
		case "6":
			b.Basenote = readMask(e, b.Basenote)
		case "7":
			b.Sign = readSign(e, b.Sign)
		}
	}
	e.Sess.Print(e.Sess.T("notes.board_save_q"))
	ans, err := e.Sess.ReadCommand(8)
	if err != nil {
		return err
	}
	ans = strings.TrimSpace(strings.ToLower(ans))
	if ans == "n" || ans == "no" {
		e.Sess.Print(e.Sess.T("notes.discarded") + "\n")
		return nil
	}
	if isNew {
		if _, err := e.Store.CreateBoard(e.Ctx, b); err != nil {
			e.Sess.Print(e.Sess.T("notes.save_failed") + "\n")
			return nil
		}
	} else {
		if err := e.Store.UpdateBoard(e.Ctx, b); err != nil {
			e.Sess.Print(e.Sess.T("notes.save_failed") + "\n")
			return nil
		}
	}
	e.Sess.Print(e.Sess.T("saved") + "\n")
	return nil
}

func signPreview(sign string) string {
	sign = strings.TrimRight(sign, "\n")
	if sign == "" {
		return "(none)"
	}
	first := sign
	if i := strings.IndexByte(sign, '\n'); i >= 0 {
		first = sign[:i] + " …"
	}
	return session.ClipRunes(first, 40)
}

func readSign(e *Env, cur string) string {
	e.Sess.Print(e.Sess.T("notes.sign_edit") + "\n")
	text, submitted, err := e.Sess.EditText(strings.TrimRight(cur, "\n"), 200)
	if err != nil || !submitted {
		return cur // 中止・エラーは現状維持
	}
	if strings.TrimRight(text, "\n") == "" {
		return ""
	}
	return text + "\n"
}

func fmtMask(v uint32) string {
	var b strings.Builder
	for i := 0; i < 32; i++ {
		if i > 0 && i%8 == 0 {
			b.WriteByte(':')
		}
		if v&(1<<uint(31-i)) != 0 {
			b.WriteByte('o')
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func readMask(e *Env, cur uint32) uint32 {
	e.Sess.Printf("mask [%s]: ", fmtMask(cur))
	line, err := e.Sess.ReadCommand(40)
	if err != nil || strings.TrimSpace(line) == "" {
		return cur
	}
	return acl.ParseMask(line)
}
