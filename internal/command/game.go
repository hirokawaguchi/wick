package command

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hirokawaguchi/wick/internal/rogue"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func (r *Registry) registerGame() {
	r.Register("rogue", cmdRogue)
}

// cmdRogue は内部ゲーム rogue を起動する。引数 -s でスコア一覧。
// 会員のみ（COMMAND.TXT のマスクで制限）。エージェントは遊べない。
func cmdRogue(e *Env) error {
	arg := strings.TrimSpace(e.Args)
	if arg == "-s" {
		return rogueScores(e)
	}
	if e.Sess.IsAgent() {
		// エージェント（AgentIO）は全画面ゲームに入らない。
		return nil
	}
	// 端末が 80x24 未満だと原典のマップが崩れる。
	w, h := e.Sess.User.TermWidth, e.Sess.User.TermHeight
	if (w > 0 && w < 80) || (h > 0 && h < 24) {
		e.Sess.Print(e.Sess.T("rogue.too_small") + "\n")
		return nil
	}

	prev := e.Sess.GetDoing()
	e.Sess.SetDoing("ROGUE")
	e.Sess.HoldNotices(true)
	defer func() {
		e.Sess.HoldNotices(false)
		e.Sess.SetDoing(prev)
		// 保留していた通知（電報など）をここで通常表示に流す。
		for _, n := range e.Sess.TakeHeldNotices() {
			e.Sess.PrintNotice(n)
		}
	}()

	lang := string(e.Sess.Lang)
	if lang == "" {
		lang = "ja"
	}
	gio := &rogueIO{e: e}
	pr := &roguePersist{e: e}
	_, err := rogue.Play(gio, pr, rogue.Options{Lang: lang, Name: e.Sess.User.Handle})
	if err != nil {
		// 切断など。上位ループに委ねる（EOF はログオフ扱い）。
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		return nil
	}
	// 画面を片付けてメニューへ戻る。
	e.Sess.Print("\x1b[2J\x1b[H\x1b[?25h")
	return nil
}

func rogueScores(e *Env) error {
	rows, err := e.Store.TopRogueScores(e.Ctx, 10)
	if err != nil {
		e.Sess.Print(e.Sess.T("rogue.score_fail") + "\n")
		return nil
	}
	if len(rows) == 0 {
		e.Sess.Print(e.Sess.T("rogue.no_scores") + "\n")
		return nil
	}
	e.Sess.Print(e.Sess.T("rogue.score_head") + "\n")
	for i, s := range rows {
		result := ""
		if s.Won {
			result = e.Sess.T("rogue.result_won")
		} else if s.Cause != "" {
			result = s.Cause
		}
		e.Sess.Print(fmt.Sprintf("%2d  %-16s %7d  %s%d  %s\n",
			i+1, session.ClipWidth(s.Handle, 16), s.Gold,
			e.Sess.T("rogue.depth_unit"), s.MaxDepth, result))
	}
	return nil
}

// rogueIO は rogue.IO をセッションで実装する薄いアダプタ。
type rogueIO struct {
	e *Env
}

func (r *rogueIO) ReadKey() (byte, error) {
	type res struct {
		c byte
		e error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := r.e.Sess.ReadKey()
		ch <- res{c, err}
	}()
	select {
	case <-r.e.Ctx.Done():
		return 0, r.e.Ctx.Err()
	case x := <-ch:
		return x.c, x.e
	}
}

func (r *rogueIO) Print(s string) { r.e.Sess.Print(s) }

func (r *rogueIO) Pending() []string {
	ns := r.e.Sess.TakeHeldNotices()
	if len(ns) == 0 {
		return nil
	}
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, r.e.Sess.NoticeSummary(n))
	}
	return out
}

func (r *rogueIO) Size() (int, int) {
	return r.e.Sess.User.TermWidth, r.e.Sess.User.TermHeight
}

// roguePersist は rogue.Persist を Store で実装する。
type roguePersist struct {
	e *Env
}

func (p *roguePersist) Load() ([]byte, bool, error) {
	blob, err := p.e.Store.LoadRogue(p.e.Ctx, p.e.Sess.User.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return blob, true, nil
}

func (p *roguePersist) Save(blob []byte) error {
	return p.e.Store.SaveRogue(p.e.Ctx, p.e.Sess.User.ID, blob)
}

func (p *roguePersist) Delete() error {
	return p.e.Store.DeleteRogue(p.e.Ctx, p.e.Sess.User.ID)
}

func (p *roguePersist) AddScore(sc rogue.Score) error {
	return p.e.Store.AddRogueScore(p.e.Ctx, store.RogueScore{
		UserID:   p.e.Sess.User.ID,
		Handle:   p.e.Sess.User.Handle,
		Gold:     sc.Gold,
		Depth:    sc.Depth,
		MaxDepth: sc.MaxDepth,
		Cause:    sc.Cause,
		Won:      sc.Won,
		Time:     sc.Time,
	})
}

func (p *roguePersist) Top(limit int) ([]rogue.Score, error) {
	rows, err := p.e.Store.TopRogueScores(p.e.Ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]rogue.Score, 0, len(rows))
	for _, s := range rows {
		out = append(out, rogue.Score{
			Gold: s.Gold, Depth: s.Depth, MaxDepth: s.MaxDepth,
			Cause: s.Cause, Won: s.Won, Time: s.Time,
		})
	}
	return out, nil
}
