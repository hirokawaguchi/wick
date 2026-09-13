package notes

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/warn"
)

type Env struct {
	Ctx    context.Context
	Sess   *session.Session
	Host   *host.Host
	Store  store.Store
	Assets assets.Dir
}

const (
	modeIndex = 0
	modeOpen  = 1

	actDisp      = 0
	actSilent    = 1
	actNextBoard = 2
	actAbort     = 3

	oflagA  = 0x0001
	oflagS  = 0x0002
	oflagX  = 0x0004
	oflagN  = 0x0008
	oflagAt = 0x0010

	nflagA = 0x0001
	nflagN = 0x0002

	indexWindow = 20
)

type runner struct {
	env      *Env
	board    store.Board
	notes    []store.Note
	res      []store.Response
	status   int
	note     int
	resn     int
	oflag    int
	nflag    int
	boardSeq time.Time
	noteSeq  time.Time
	idxOff   int
}

func Open(env *Env, args string) error {
	oflag, pat := parseOpts(args)
	if pat == "" {
		env.Sess.Print("usage: open [-sxna@] <wildcard>\n")
		return nil
	}
	boards, err := env.Store.ListBoards(env.Ctx)
	if err != nil {
		return err
	}
	// who に居場所を出す（読み終えたら元へ戻す）。人間・エージェント共通。
	prevDoing := env.Sess.GetDoing()
	defer env.Sess.SetDoing(prevDoing)
	pats := scanPatterns(pat, env.Sess.User.ScanList, oflag&oflagS != 0)
	opened := false
	guided := false
	seen := map[int64]bool{}
	for _, p := range pats {
		for _, b := range boards {
			if seen[b.ID] || !wild(b.Name, p) {
				continue
			}
			if !b.CanRead(env.Sess.User.Flags) {
				continue
			}
			if oflag&oflagS != 0 && oflag&oflagA == 0 && !b.LastUpdate.After(env.Sess.Sequencer) {
				continue
			}
			seen[b.ID] = true
			if !guided {
				// 操作の統一ガイド（実際に開くとき一度だけ）。i=一覧 / q=抜ける / ?=ヘルプ。
				env.Sess.Print("[ i=一覧  q=抜ける  ?=ヘルプ ]\n")
				guided = true
			}
			env.Sess.SetDoing("NOTE " + b.Name)
			env.Sess.Printf("<%s>\n", b.Name)
			opened = true
			r := &runner{
				env: env, board: b, status: modeIndex,
				oflag: oflag, boardSeq: env.Sess.Sequencer, noteSeq: env.Sess.Sequencer,
			}
			code, err := r.enter()
			if err != nil {
				return err
			}
			if code == actAbort {
				if oflag&oflagAt != 0 {
					env.Sess.Print("-- SCAN ABORTED --\n")
				}
				return nil
			}
		}
	}
	if oflag&oflagAt != 0 {
		env.Sess.Print("-- SCAN COMPLETED --\n")
	} else if !opened && oflag&oflagS == 0 {
		env.Sess.Print("no such board\n")
	}
	return nil
}

func scanPatterns(pat, list string, scanning bool) []string {
	if scanning && pat == "*" {
		if items := store.ParseScanList(list); len(items) > 0 {
			return items
		}
	}
	return []string{pat}
}

func parseOpts(args string) (int, string) {
	fields := strings.Fields(args)
	var flag int
	pat := ""
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			for _, c := range strings.ToLower(f[1:]) {
				switch c {
				case 's':
					flag |= oflagS
				case 'x':
					flag |= oflagX
				case 'n':
					flag |= oflagN
				case 'a':
					flag |= oflagA
				case '@':
					flag |= oflagAt
				}
			}
			continue
		}
		pat = f
	}
	return flag, pat
}

func wild(name, pat string) bool {
	name = strings.ToLower(name)
	pat = strings.ToLower(pat)
	return wild1(name, pat)
}

func wild1(s, p string) bool {
	for {
		if p == "" {
			return s == ""
		}
		if p[0] == '*' {
			for i := 0; i <= len(s); i++ {
				if wild1(s[i:], p[1:]) {
					return true
				}
			}
			return false
		}
		if s == "" {
			return false
		}
		if p[0] != '?' && p[0] != s[0] {
			return false
		}
		s, p = s[1:], p[1:]
	}
}

func (r *runner) enter() (int, error) {
	if err := r.reload(); err != nil {
		return 0, err
	}
	if r.note == 0 {
		r.note = r.latestLiveNum()
	}
	if r.oflag&(oflagS|oflagX) != 0 {
		if n := r.searchNew(0); n == 0 {
			if r.oflag&oflagS != 0 {
				return actNextBoard, nil
			}
		} else {
			r.status = modeOpen
			r.note = n
			r.resn = 0
		}
	}
	for {
		switch r.status {
		case modeIndex:
			r.dispIndex()
		case modeOpen:
			if err := r.dispMessage(); err != nil {
				return 0, err
			}
		}
	again:
		c, err := r.nextKey()
		if err != nil {
			return 0, err
		}
		r.env.Sess.Print("\n")
		code, err := r.dispatch(c)
		if err != nil {
			return 0, err
		}
		switch code {
		case actSilent:
			goto again
		case actNextBoard, actAbort:
			return code, nil
		}
	}
}

func (r *runner) nextKey() (byte, error) {
	if r.status == modeOpen && (r.oflag&oflagN != 0 || r.nflag&nflagN != 0) {
		if r.env.Sess.KeyWaiting() {
			r.oflag &^= oflagN | oflagS
			r.nflag &^= nflagN
		} else {
			return 'l', nil
		}
	}
	// 統一ルール: 「対象 モード>」。対象＝ボード名（フル。OPEN はノート番号も）。
	// スラグ（junk 等）は junk.test/junk.sandbox で共通のため板を特定できない。
	target := r.board.Name
	mode := "INDEX"
	if r.status == modeOpen {
		mode = "OPEN"
		target = target + ":" + strconv.Itoa(r.note)
	}
	r.env.Sess.Print(target + " " + mode + "> ")
	return readKeyCtx(r.env)
}

func readKeyCtx(env *Env) (byte, error) {
	type rec struct {
		c byte
		e error
	}
	ch := make(chan rec, 1)
	go func() {
		c, e := env.Sess.ReadKey()
		ch <- rec{c, e}
	}()
	select {
	case <-env.Ctx.Done():
		return 0, env.Ctx.Err()
	case x := <-ch:
		return x.c, x.e
	}
}

func (r *runner) reload() error {
	notes, err := r.env.Store.ListNotes(r.env.Ctx, r.board.ID)
	if err != nil {
		return err
	}
	r.notes = notes
	if b, err := r.env.Store.GetBoard(r.env.Ctx, r.board.Name); err == nil {
		r.board = b
	}
	return r.reloadRes()
}

func (r *runner) reloadRes() error {
	n, err := r.curNote()
	if err != nil {
		r.res = nil
		return nil
	}
	rs, err := r.env.Store.ListResponses(r.env.Ctx, n.ID)
	if err != nil {
		return err
	}
	r.res = rs
	return nil
}

func (r *runner) curNote() (store.Note, error) {
	for _, n := range r.notes {
		if n.Num == r.note {
			return n, nil
		}
	}
	if r.note > 0 {
		return r.env.Store.GetNote(r.env.Ctx, r.board.ID, r.note)
	}
	return store.Note{}, store.ErrNotFound
}

func (r *runner) maxNote() int {
	m := 0
	for _, n := range r.notes {
		if n.Num > m {
			m = n.Num
		}
	}
	return m
}

func (r *runner) searchNew(after int) int {
	for _, n := range r.notes {
		if n.Num <= after || n.Flags&store.MsgDeleted != 0 {
			continue
		}
		if n.LastUpdate.After(r.noteSeq) || r.oflag&oflagA != 0 || r.nflag&nflagA != 0 {
			return n.Num
		}
	}
	return 0
}

func (r *runner) liveNotes() []store.Note {
	var live []store.Note
	for _, n := range r.notes {
		if n.Flags&store.MsgDeleted == 0 {
			live = append(live, n)
		}
	}
	return live
}

func (r *runner) indexNotes() []store.Note {
	live := r.liveNotes()
	n := len(live)
	if n <= indexWindow {
		r.idxOff = 0
		return live
	}
	maxOff := n - indexWindow
	if r.idxOff < 0 {
		r.idxOff = 0
	}
	if r.idxOff > maxOff {
		r.idxOff = maxOff
	}
	end := n - r.idxOff
	return live[end-indexWindow : end]
}

func (r *runner) idxMaxOff() int {
	if m := len(r.liveNotes()) - indexWindow; m > 0 {
		return m
	}
	return 0
}

func (r *runner) indexPage(dir int) (int, error) {
	max := r.idxMaxOff()
	if max == 0 {
		return actSilent, nil
	}
	r.idxOff += dir * indexWindow
	if r.idxOff < 0 {
		r.idxOff = 0
	}
	if r.idxOff > max {
		r.idxOff = max
	}
	return actDisp, nil
}

func (r *runner) indexSearch() (int, error) {
	r.env.Sess.Print("検索語 : ")
	kw, err := r.env.Sess.ReadLine(40)
	if err != nil {
		return 0, err
	}
	kw = strings.TrimSpace(kw)
	if kw == "" {
		return actSilent, nil
	}
	low := strings.ToLower(kw)
	var hits []store.Note
	for _, n := range r.liveNotes() {
		if strings.Contains(strings.ToLower(n.Title), low) ||
			strings.Contains(strings.ToLower(n.Body), low) {
			hits = append(hits, n)
		}
	}
	if len(hits) == 0 {
		r.env.Sess.Printf("-- \"%s\" は見つかりません --\n", kw)
		return actSilent, nil
	}
	r.env.Sess.Printf("-- \"%s\" : %d 件 --\n", kw, len(hits))
	for _, n := range hits {
		r.env.Sess.Printf("%5d %s %-8s %s\n",
			n.Num, n.PostTime.Format("01/02"), n.Author, n.Title)
	}
	r.env.Sess.Print("開く番号 (Enter で戻る) : ")
	line, err := r.env.Sess.ReadCommand(8)
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return actSilent, nil
	}
	num, err := strconv.Atoi(line)
	if err != nil || num < 1 {
		return actSilent, nil
	}
	r.note = num
	r.resn = 0
	r.status = modeOpen
	return actDisp, nil
}

func (r *runner) indexSetSeque() (int, error) {
	r.env.Sess.Printf("現在の未読基準 : %s\n", formatSeqTime(r.boardSeq))
	r.env.Sess.Print("この時刻以降を未読に (YYYY/MM/DD HH:MM:SS, Enter で中止) : ")
	line, err := r.env.Sess.ReadCommand(32)
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return actSilent, nil
	}
	t, err := parseIndexTime(line)
	if err != nil {
		r.env.Sess.Print("invalid\n")
		return actSilent, nil
	}
	r.boardSeq = t
	r.noteSeq = t
	r.env.Sess.Printf("-- %s 以降を未読とします（l/TAB で回収）--\n", formatSeqTime(t))
	return actSilent, nil
}

func (r *runner) showSign() (int, error) {
	if b, err := r.env.Store.GetBoard(r.env.Ctx, r.board.Name); err == nil {
		r.board = b
	}
	sign := strings.TrimRight(r.board.Sign, "\n")
	if sign == "" {
		r.env.Sess.Print("-- 看板はありません --\n")
		return actSilent, nil
	}
	r.env.Sess.Printf("\n=== %s の看板 ===\n", r.board.Name)
	r.env.Sess.Print(sign)
	r.env.Sess.Print("\n===\n")
	return actSilent, nil
}

func formatSeqTime(t time.Time) string {
	if t.IsZero() {
		return "(none)"
	}
	return t.Format("2006/01/02 15:04:05")
}

func parseIndexTime(s string) (time.Time, error) {
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
	return time.Time{}, errBadTime
}

var errBadTime = errors.New("bad time")

func (r *runner) latestLiveNum() int {
	live := r.liveNotes()
	if len(live) == 0 {
		return 1
	}
	return live[len(live)-1].Num
}

func (r *runner) dispIndex() {
	shown := r.indexNotes()
	live := len(r.liveNotes())
	r.env.Sess.Printf("\n[ INDEX ]  %s  (%s)\n", r.board.Desc, r.board.Name)
	r.env.Sess.Print(" Num   Date     Last      Res Author   Title\n")
	for _, n := range shown {
		mark := ' '
		switch {
		case n.Num == r.note:
			mark = '>'
		case n.Flags&store.MsgImportant != 0:
			mark = '!'
		case n.Flags&store.MsgClosed != 0:
			mark = '%'
		case n.Flags&store.MsgAWO != 0:
			mark = '*'
		}
		r.env.Sess.Printf("%c%5d %s %s %5d %-8s %s\n",
			mark, n.Num,
			n.PostTime.Format("01/02"), n.LastUpdate.Format("01/02"),
			n.Response, n.Author, n.Title)
	}
	if live > indexWindow {
		newest := live - r.idxOff
		oldest := newest - len(shown) + 1
		r.env.Sess.Printf("-- %d..%d / %d  (Space=古 BS=新 =最新 *最古 f=検索 k=看板) --\n",
			oldest, newest, live)
	} else {
		r.env.Sess.Print("-- END --\n")
	}
}

func (r *runner) dispMessage() error {
	n, err := r.curNote()
	if err != nil {
		r.env.Sess.Print("no such note\n")
		r.status = modeIndex
		return nil
	}
	if err := r.reloadRes(); err != nil {
		return err
	}
	if r.resn == 0 {
		r.nflag = 0
		r.noteSeq = r.boardSeq
	}
	r.env.Sess.NoteBoard = r.board.Name
	r.env.Sess.NoteNum = n.Num
	r.env.Sess.NoteID = n.ID
	r.env.Sess.Printf("Note %-5d   %s  (%s)\n", n.Num, r.board.Desc, r.board.Name)
	if r.resn == 0 {
		r.env.Sess.Printf("[ BASENOTE with%4dRes ]", n.Response)
		r.showFlags(n.Flags, n.Flags)
		r.env.Sess.Print("\n")
		if n.Flags&store.MsgDeleted != 0 {
			r.env.Sess.Print("** 削除されています **\n\n")
			return nil
		}
		r.env.Sess.Printf("Title: %s\n", n.Title)
		r.printMsg(n.Author, n.Handle, n.PostTime, n.Body)
		r.env.Sess.Seen(n.LastUpdate)
		return nil
	}
	rs, err := r.env.Store.GetResponse(r.env.Ctx, n.ID, r.resn)
	if err != nil {
		r.resn = 0
		return r.dispMessage()
	}
	r.env.Sess.Printf("[ RESPONSE:%4d of%4d ]", rs.Num, n.Response)
	r.showFlags(rs.Flags, n.Flags)
	r.env.Sess.Print("\n")
	if rs.Flags&store.MsgDeleted != 0 {
		r.env.Sess.Print("** 削除されています **\n\n")
		return nil
	}
	r.env.Sess.Printf("Title: %s\n", n.Title)
	if rs.Flags&store.MsgSubject != 0 && rs.Title != "" {
		r.env.Sess.Printf("Subject: %s\n", rs.Title)
	}
	r.printMsg(rs.Author, rs.Handle, rs.PostTime, rs.Body)
	r.env.Sess.Seen(rs.PostTime)
	return nil
}

func (r *runner) showFlags(msg, base int) {
	switch {
	case msg&store.MsgImportant != 0:
		r.env.Sess.Print("  ** Important **")
	case base&store.MsgClosed != 0:
		r.env.Sess.Print("  ** Closed **")
	case base&store.MsgAWO != 0:
		r.env.Sess.Print("  ** Author Write Only **")
	}
}

func (r *runner) printMsg(author, handle string, t time.Time, body string) {
	r.env.Sess.Printf("Bytes: %-6d Date : %s  Author: %s (%s)\n\n",
		len(body), t.Format("2006-01-02 15:04:05"), author, handle)
	r.env.Sess.Print(body)
	if !strings.HasSuffix(body, "\n") {
		r.env.Sess.Print("\n")
	}
	if warn.HasLink(body) {
		r.env.Sess.Print(warn.External + "\n")
	}
	r.env.Sess.Print("\n")
}

func (r *runner) dispatch(c byte) (int, error) {
	if r.status == modeIndex {
		return r.dispIndexKey(c)
	}
	return r.dispOpenKey(c)
}

func (r *runner) dispIndexKey(c byte) (int, error) {
	switch c {
	case 0:
		return actSilent, nil
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return r.gotoNote(c)
	case 0x1b, 'q':
		return actNextBoard, nil
	case 0x04:
		return actAbort, nil
	case 0x0c, 'v':
		return actDisp, nil
	case 0x12:
		return r.toggleFlag("Subject", store.MsgSubject, true)
	case '\n':
		r.status = modeOpen
		r.resn = 0
		return actDisp, nil
	case 'i':
		// 一覧（INDEX）の再表示。三種の場で i=一覧 に揃えるため。
		return actDisp, nil
	case '\t', 'l':
		return r.openNew()
	case ' ':
		return r.indexPage(+1)
	case '\b', 0x7f:
		return r.indexPage(-1)
	case '=':
		r.idxOff = 0
		return actDisp, nil
	case '*':
		r.idxOff = r.idxMaxOff()
		return actDisp, nil
	case 'f':
		return r.indexSearch()
	case 'A':
		return r.indexSetSeque()
	case 'k':
		return r.showSign()
	case '?':
		return r.help("open0.hlp")
	case '!':
		return r.toggleFlag("Important", store.MsgImportant, false)
	case '%':
		return r.toggleFlag("Close", store.MsgClosed, true)
	case 'a':
		return r.toggleAllUnread()
	case 'd':
		return r.deleteMsg()
	case 'g':
		r.env.Sess.Print("-- Author Write Only --\n")
		return r.postBase(store.MsgAWO)
	case 'w':
		return r.postBase(0)
	}
	r.env.Sess.Print("[ i=一覧  q=抜ける  ?=ヘルプ ]\n")
	return actSilent, nil
}

func (r *runner) dispOpenKey(c byte) (int, error) {
	switch c {
	case 0:
		return actSilent, nil
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return r.gotoRes(c)
	case 0x1b, 'q':
		return actNextBoard, nil
	case 0x04:
		return actAbort, nil
	case 0x05, '\b', 0x7f, '<':
		return r.prevRes()
	case 0x07:
		r.env.Sess.Print("-- Author Write Only --\n")
		return r.postBase(store.MsgAWO)
	case 0x0c, 'v':
		return actDisp, nil
	case 0x12:
		return r.toggleFlag("Subject", store.MsgSubject, true)
	case 0x17:
		return r.postBase(0)
	case 0x18, '\n', '>', ' ':
		return r.nextRes()
	case '\t', 'l':
		return r.nextNewRes()
	case '?':
		return r.help("open1.hlp")
	case '&':
		return r.help("open2.hlp")
	case '!':
		return r.toggleFlag("Important", store.MsgImportant, false)
	case '%':
		return r.toggleFlag("Close", store.MsgClosed, true)
	case '=':
		r.resn = 0
		return actDisp, nil
	case '*':
		n, err := r.curNote()
		if err == nil {
			r.resn = n.Response
		}
		return actDisp, nil
	case '+', 'j':
		return r.nextNote()
	case '-':
		return r.prevNote()
	case 'a':
		return r.toggleNoteUnread()
	case 'b':
		return r.gotoNote(0)
	case 'c':
		r.nflag |= nflagN
		return actSilent, nil
	case 'C':
		r.oflag |= oflagN | oflagS
		return actSilent, nil
	case 'd':
		return r.deleteMsg()
	case 'e':
		return r.editTitle()
	case 'G':
		return r.toggleFlag("Author Write Only", store.MsgAWO, true)
	case 'i':
		r.status = modeIndex
		r.resn = 0
		return actDisp, nil
	case 'I':
		return r.listTitles(store.MsgImportant)
	case 'L':
		return r.nextNewNote()
	case 't':
		return r.listTitles(0)
	case 'T':
		r.resn = 0
		return r.listTitles(0)
	case 'w':
		return r.postRes()
	}
	r.env.Sess.Print("[ i=一覧  q=抜ける  ?=ヘルプ ]\n")
	return actSilent, nil
}

func (r *runner) help(name string) (int, error) {
	text, err := r.env.Assets.Read("help", name)
	if err != nil {
		r.env.Sess.Print("no help\n")
		return actSilent, nil
	}
	r.env.Sess.Print(text)
	return actSilent, nil
}

func (r *runner) gotoNote(first byte) (int, error) {
	r.env.Sess.Print("Basenote番号 : ")
	var rest string
	var err error
	if first >= '0' && first <= '9' {
		r.env.Sess.Print(string(first))
		rest, err = r.env.Sess.ReadCommand(8)
		if err != nil {
			return 0, err
		}
		rest = string(first) + rest
	} else {
		rest, err = r.env.Sess.ReadCommand(8)
		if err != nil {
			return 0, err
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || n < 1 {
		return actSilent, nil
	}
	if m := r.maxNote(); m > 0 && n > m {
		n = m
	}
	r.note = n
	r.resn = 0
	r.status = modeOpen
	return actDisp, nil
}

func (r *runner) gotoRes(first byte) (int, error) {
	r.env.Sess.Print("Response番号 : ")
	r.env.Sess.Print(string(first))
	rest, err := r.env.Sess.ReadCommand(8)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(first) + rest))
	if err != nil {
		return actSilent, nil
	}
	note, e := r.curNote()
	if e != nil {
		return actSilent, nil
	}
	if n > note.Response {
		n = note.Response
	}
	if n < 0 {
		n = 0
	}
	r.resn = n
	return actDisp, nil
}

func (r *runner) nextRes() (int, error) {
	n, err := r.curNote()
	if err != nil || r.resn+1 > n.Response {
		return r.nextNote()
	}
	r.resn++
	return actDisp, nil
}

func (r *runner) prevRes() (int, error) {
	if r.resn > 0 {
		r.resn--
		return actDisp, nil
	}
	return r.prevNote()
}

func (r *runner) nextNote() (int, error) {
	if r.note+1 > r.maxNote() {
		return r.queryEnd()
	}
	r.note++
	r.resn = 0
	return actDisp, nil
}

func (r *runner) prevNote() (int, error) {
	if r.note > 1 {
		r.note--
		r.resn = 0
		return actDisp, nil
	}
	return actSilent, nil
}

func (r *runner) nextNewRes() (int, error) {
	n, err := r.curNote()
	if err != nil {
		return r.nextNewNote()
	}
	if err := r.reloadRes(); err != nil {
		return 0, err
	}
	for _, rs := range r.res {
		if rs.Num <= r.resn || rs.Flags&store.MsgDeleted != 0 {
			continue
		}
		if rs.PostTime.After(r.noteSeq) || r.oflag&oflagA != 0 || r.nflag&nflagA != 0 {
			r.resn = rs.Num
			return actDisp, nil
		}
	}
	_ = n
	return r.nextNewNote()
}

func (r *runner) nextNewNote() (int, error) {
	if r.nflag&nflagN != 0 {
		r.nflag &^= nflagN
		return actSilent, nil
	}
	n := r.searchNew(r.note)
	if n == 0 {
		if r.oflag&oflagN != 0 {
			return actNextBoard, nil
		}
		return r.queryEnd()
	}
	r.note = n
	r.resn = 0
	return actDisp, nil
}

func (r *runner) openNew() (int, error) {
	cur := r.note
	r.note = 0
	code, err := r.nextNewNote()
	if err != nil {
		return 0, err
	}
	if code != actDisp {
		r.note = cur
		return code, nil
	}
	r.status = modeOpen
	return actDisp, nil
}

func (r *runner) queryEnd() (int, error) {
	r.env.Sess.Print("-- 続きなし --  (Enter/Space/>) 次レス  (l/TAB) 未読  (BS) 戻る  (^D) 終了 : ")
	c, err := readKeyCtx(r.env)
	if err != nil {
		return 0, err
	}
	r.env.Sess.Print("\n")
	switch c {
	case '\n', ' ', '>':
		r.env.Sess.Print("次のレスはありません\n")
		return actSilent, nil
	case 'l', 'L', '\t':
		return actNextBoard, nil
	case '\b', 0x7f, 0x05, '<':
		return actSilent, nil
	case 0x04, 'q', 0x1b:
		return actAbort, nil
	default:
		return actSilent, nil
	}
}

func (r *runner) postBase(flags int) (int, error) {
	if !r.board.CanBasenote(r.env.Sess.User.Flags) {
		r.env.Sess.Print("** 書き込めません **\n")
		return actSilent, nil
	}
	if r.maxNote() >= store.MaxNotes {
		r.env.Sess.Print("** too many notes **\n")
		return actSilent, nil
	}
	r.env.Sess.Print("----  Basenote  ----\nTitle : ")
	title, err := r.env.Sess.ReadLine(40)
	if err != nil {
		return 0, err
	}
	title = session.ClipWidth(strings.TrimSpace(title), store.MaxTitle)
	if title == "" {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	body, ok, err := r.editBody()
	if err != nil {
		return 0, err
	}
	if !ok {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	body = store.ApplyAutosign(body, r.env.Sess.User.Autosign)
	if r.env.Host != nil {
		u := r.env.Host.LockBoard(r.board.ID)
		defer u()
	}
	n, err := r.env.Store.CreateNote(r.env.Ctx, store.Note{
		BoardID: r.board.ID, Title: title,
		Author: r.env.Sess.User.ID, Handle: r.env.Sess.User.Handle,
		Flags: flags, Body: body,
	})
	if err != nil {
		if errors.Is(err, store.ErrTooManyNotes) {
			r.env.Sess.Print("** too many notes **\n")
			return actSilent, nil
		}
		return 0, err
	}
	if err := r.reload(); err != nil {
		return 0, err
	}
	r.status = modeOpen
	r.note = n.Num
	r.resn = 0
	r.env.Sess.Print("-- 書き込み完了 --\n")
	return actDisp, nil
}

func (r *runner) postRes() (int, error) {
	n, err := r.curNote()
	if err != nil {
		return actSilent, nil
	}
	if n.Flags&store.MsgClosed != 0 || !r.board.CanWrite(r.env.Sess.User.Flags) {
		r.env.Sess.Print("** 書き込めません **\n")
		return actSilent, nil
	}
	if n.Flags&store.MsgAWO != 0 && !strings.EqualFold(n.Author, r.env.Sess.User.ID) {
		r.env.Sess.Print("** 書き込めません **\n")
		return actSilent, nil
	}
	if n.Response >= store.MaxResponses {
		r.env.Sess.Print("** too many responses **\n")
		return actSilent, nil
	}
	r.env.Sess.Print("----  Response  ----\n")
	title := "Re :" + n.Title
	flags := 0
	if n.Flags&store.MsgSubject != 0 {
		r.env.Sess.Print("Subject : ")
		t, err := r.env.Sess.ReadLine(40)
		if err != nil {
			return 0, err
		}
		t = strings.TrimSpace(t)
		if t == "" {
			r.env.Sess.Print("-- 中止しました --\n")
			return actSilent, nil
		}
		title = t
		flags = store.MsgSubject
	}
	title = session.ClipWidth(title, store.MaxTitle)
	body, ok, err := r.editBody()
	if err != nil {
		return 0, err
	}
	if !ok {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	body = store.ApplyAutosign(body, r.env.Sess.User.Autosign)
	if r.env.Host != nil {
		u := r.env.Host.LockBoard(r.board.ID)
		defer u()
	}
	if _, err := r.env.Store.CreateResponse(r.env.Ctx, store.Response{
		NoteID: n.ID, Title: title,
		Author: r.env.Sess.User.ID, Handle: r.env.Sess.User.Handle,
		Flags: flags, Body: body,
	}); err != nil {
		if errors.Is(err, store.ErrTooManyResponses) {
			r.env.Sess.Print("** too many responses **\n")
			return actSilent, nil
		}
		return 0, err
	}
	if err := r.reload(); err != nil {
		return 0, err
	}
	r.env.Sess.Print("-- 書き込み完了 --\n")
	return actSilent, nil
}

func (r *runner) editBody() (string, bool, error) {
	r.env.Sess.Print("本文 (矢印で移動して修正可。送信は単独行の . / 中止は Ctrl-C):\n")
	text, submitted, err := r.env.Sess.EditText("", 500)
	if err != nil {
		return "", false, err
	}
	if !submitted || strings.TrimRight(text, "\n") == "" {
		return "", false, nil
	}
	return text + "\n", true, nil
}

func (r *runner) deleteMsg() (int, error) {
	if r.status == modeIndex {
		r.env.Sess.Printf("削除する番号 [%d]: ", r.note)
		line, err := r.env.Sess.ReadCommand(8)
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line != "" {
			n, err := strconv.Atoi(line)
			if err != nil || n < 1 {
				r.env.Sess.Print("invalid\n")
				return actSilent, nil
			}
			r.note = n
		}
		r.resn = 0
	}
	n, err := r.curNote()
	if err != nil {
		r.env.Sess.Print("no such note\n")
		return actSilent, nil
	}
	if r.resn == 0 {
		r.env.Sess.Printf("Basenote %d: %s (%s)\n", n.Num, n.Title, n.Author)
		if !strings.EqualFold(n.Author, r.env.Sess.User.ID) {
			r.env.Sess.Print("** 自分の書き込みだけ削除できます **\n")
			return actSilent, nil
		}
		return r.toggleFlag("Delete", store.MsgDeleted, false)
	}
	rs, err := r.env.Store.GetResponse(r.env.Ctx, n.ID, r.resn)
	if err != nil {
		return actSilent, nil
	}
	r.env.Sess.Printf("Response %d: %s (%s)\n", rs.Num, rs.Title, rs.Author)
	if !strings.EqualFold(rs.Author, r.env.Sess.User.ID) {
		r.env.Sess.Print("** 自分の書き込みだけ削除できます **\n")
		return actSilent, nil
	}
	return r.toggleResFlag("Delete", store.MsgDeleted)
}

func (r *runner) toggleFlag(label string, bit int, authorOnly bool) (int, error) {
	n, err := r.curNote()
	if err != nil {
		return actSilent, nil
	}
	if authorOnly && !strings.EqualFold(n.Author, r.env.Sess.User.ID) {
		return actSilent, nil
	}
	on := n.Flags&bit != 0
	ok, err := r.confirm(label, on)
	if err != nil || !ok {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	if on {
		n.Flags &^= bit
	} else {
		n.Flags |= bit
	}
	if err := r.env.Store.UpdateNote(r.env.Ctx, n); err != nil {
		return 0, err
	}
	_ = r.reload()
	r.env.Sess.Print("-- 変更しました --\n")
	return actSilent, nil
}

func (r *runner) toggleResFlag(label string, bit int) (int, error) {
	n, err := r.curNote()
	if err != nil {
		return actSilent, nil
	}
	rs, err := r.env.Store.GetResponse(r.env.Ctx, n.ID, r.resn)
	if err != nil {
		return actSilent, nil
	}
	on := rs.Flags&bit != 0
	ok, err := r.confirm(label, on)
	if err != nil || !ok {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	if on {
		rs.Flags &^= bit
	} else {
		rs.Flags |= bit
	}
	if err := r.env.Store.UpdateResponse(r.env.Ctx, rs); err != nil {
		return 0, err
	}
	r.env.Sess.Print("-- 変更しました --\n")
	return actSilent, nil
}

func (r *runner) confirm(label string, on bool) (bool, error) {
	q := label + " 設定しますか"
	if on {
		q = label + " 解除しますか"
	}
	r.env.Sess.Print(q + " [Y/n]: ")
	line, err := r.env.Sess.ReadCommand(8)
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "" || line == "y" || line == "yes", nil
}

func (r *runner) editTitle() (int, error) {
	n, err := r.curNote()
	if err != nil {
		return actSilent, nil
	}
	author := n.Author
	if r.resn != 0 {
		rs, err := r.env.Store.GetResponse(r.env.Ctx, n.ID, r.resn)
		if err != nil {
			return actSilent, nil
		}
		author = rs.Author
	}
	if !strings.EqualFold(author, r.env.Sess.User.ID) {
		return actSilent, nil
	}
	r.env.Sess.Print("New Title : ")
	title, err := r.env.Sess.ReadLine(40)
	if err != nil {
		return 0, err
	}
	title = session.ClipWidth(strings.TrimSpace(title), store.MaxTitle)
	if title == "" {
		return actSilent, nil
	}
	ok, err := r.confirm("変更", false)
	if err != nil || !ok {
		r.env.Sess.Print("-- 中止しました --\n")
		return actSilent, nil
	}
	if r.resn == 0 {
		n.Title = title
		if err := r.env.Store.UpdateNote(r.env.Ctx, n); err != nil {
			return 0, err
		}
	} else {
		rs, _ := r.env.Store.GetResponse(r.env.Ctx, n.ID, r.resn)
		rs.Title = title
		if err := r.env.Store.UpdateResponse(r.env.Ctx, rs); err != nil {
			return 0, err
		}
	}
	_ = r.reload()
	r.env.Sess.Print("-- 変更しました --\n")
	return actSilent, nil
}

func (r *runner) toggleAllUnread() (int, error) {
	if r.oflag&oflagA != 0 {
		r.env.Sess.Print("-- 未読設定を通常に戻しました --\n")
		r.oflag &^= oflagA
	} else {
		r.env.Sess.Print("-- 全メッセージを未読とします --\n")
		r.oflag |= oflagA
	}
	return actSilent, nil
}

func (r *runner) toggleNoteUnread() (int, error) {
	if r.nflag&nflagA != 0 {
		r.env.Sess.Print("-- 未読設定を通常に戻しました --\n")
		r.nflag &^= nflagA
	} else {
		r.env.Sess.Print("-- このノートの全メッセージを未読とします --\n")
		r.nflag |= nflagA
	}
	return actSilent, nil
}

func (r *runner) listTitles(mask int) (int, error) {
	n, err := r.curNote()
	if err != nil {
		return actSilent, nil
	}
	if mask == 0 || n.Flags&mask != 0 {
		r.env.Sess.Printf(">> %s %s %-8s (%s)\n", session.PadRight(n.Title, store.MaxTitle), n.PostTime.Format("01/02"), n.Author, n.Handle)
	}
	if err := r.reloadRes(); err != nil {
		return 0, err
	}
	start := r.resn
	if start < 1 {
		start = 1
	}
	for _, rs := range r.res {
		if rs.Num < start {
			continue
		}
		if mask != 0 && rs.Flags&mask == 0 {
			continue
		}
		r.env.Sess.Printf("%d) %s %s %-8s (%s)\n", rs.Num, session.PadRight(rs.Title, store.MaxTitle), rs.PostTime.Format("01/02"), rs.Author, rs.Handle)
	}
	return actSilent, nil
}

func IsCancel(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
