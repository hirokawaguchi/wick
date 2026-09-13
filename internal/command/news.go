package command

import (
	"strings"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/warn"
)

func (r *Registry) registerNews() {
	r.Register("readnews", cmdReadnews)
	r.Register("postnews", cmdPostnews)
}

const (
	newsNormal = iota
	newsNonstop
	newsList
	newsCheck
)

// newsAct は記事プロンプトから読みループへ返す操作。
type newsAct struct {
	kind  int
	group string
}

const (
	naNext = iota
	naNextGroup
	naUnsub
	naJump
	naQuitSave
	naQuitNoSave
)

func cmdReadnews(e *Env) error {
	if err := e.Store.SeedNewsGroups(e.Ctx); err != nil {
		return err
	}
	mode, group, ok := parseNewsArgs(e.Args)
	if !ok {
		e.Sess.Print("usage: readnews [-n|-l|-c] [group]\n")
		return nil
	}
	queue, err := newsQueue(e, group)
	if err != nil {
		if err == store.ErrNotFound {
			e.Sess.Print("no such group\n")
			return nil
		}
		return err
	}
	switch mode {
	case newsCheck:
		return newsCheckUnread(e, queue)
	case newsList:
		return newsListHeads(e, queue)
	default:
		return newsReadLoop(e, queue, mode == newsNonstop)
	}
}

// newsQueue は読む順のグループ列を作る。
// group を名指ししたときは購読解除中でもそのグループだけを返す（UNIX どおり）。
// 無指定なら購読中のグループだけを回す。
func newsQueue(e *Env, group string) ([]store.NewsGroup, error) {
	if group != "" {
		g, err := e.Store.GetNewsGroup(e.Ctx, group)
		if err != nil {
			return nil, err
		}
		return []store.NewsGroup{g}, nil
	}
	all, err := e.Store.ListNewsGroups(e.Ctx)
	if err != nil {
		return nil, err
	}
	var out []store.NewsGroup
	for _, g := range all {
		sub, err := e.Store.NewsSubscribed(e.Ctx, e.Sess.User.ID, g.Name)
		if err != nil {
			return nil, err
		}
		if sub {
			out = append(out, g)
		}
	}
	return out, nil
}

func newsCheckUnread(e *Env, queue []store.NewsGroup) error {
	for _, g := range queue {
		cur, err := e.Store.NewsCursor(e.Ctx, e.Sess.User.ID, g.Name)
		if err != nil {
			return err
		}
		arts, err := e.Store.ListNewsAfter(e.Ctx, g.Name, cur)
		if err != nil {
			return err
		}
		if len(arts) > 0 {
			e.Sess.Print("News.\n")
			return nil
		}
	}
	e.Sess.Print("No news.\n")
	return nil
}

func newsListHeads(e *Env, queue []store.NewsGroup) error {
	shown := 0
	for _, g := range queue {
		cur, err := e.Store.NewsCursor(e.Ctx, e.Sess.User.ID, g.Name)
		if err != nil {
			return err
		}
		arts, err := e.Store.ListNewsAfter(e.Ctx, g.Name, cur)
		if err != nil {
			return err
		}
		for _, a := range arts {
			if shown == 0 {
				e.Sess.Print("\n" + e.Sess.T("news.unread_heads") + "\n")
			}
			shown++
			e.Sess.Printf("%-16s %5d  %-8s %s\n", a.Group, a.Num, a.FromID, a.Subject)
		}
	}
	if shown == 0 {
		e.Sess.Print("No news.\n")
	}
	return nil
}

// printNewsGroupHeads は指定グループの見出し一覧を出す（リーダー内の i=一覧 用）。
func printNewsGroupHeads(e *Env, group string) {
	arts, err := e.Store.ListNewsAfter(e.Ctx, group, 0)
	if err != nil {
		e.Sess.Print(e.Sess.T("news.heads_fail") + "\n")
		return
	}
	if len(arts) == 0 {
		e.Sess.Print("No news.\n")
		return
	}
	e.Sess.Print("\n" + e.Sess.T("news.group_heads", group) + "\n")
	for _, a := range arts {
		e.Sess.Printf("%5d  %-8s %s\n", a.Num, a.FromID, a.Subject)
	}
}

func newsReadLoop(e *Env, queue []store.NewsGroup, nonstop bool) error {
	if !nonstop {
		// 操作の統一ガイド（入場時に一度）。三種の場で共通: i=一覧 / q=抜ける / ?=ヘルプ。
		e.Sess.Print(e.Sess.T("news.guide") + "\n")
	}
	// who に居場所を出す（読み終えたら元へ戻す）。
	prevDoing := e.Sess.GetDoing()
	defer e.Sess.SetDoing(prevDoing)
	progress := map[string]int{}
	shown := 0
	for gi := 0; gi < len(queue); gi++ {
		g := queue[gi]
		e.Sess.SetDoing("NEWS " + g.Name)
		cur, err := e.Store.NewsCursor(e.Ctx, e.Sess.User.ID, g.Name)
		if err != nil {
			return err
		}
		if p, ok := progress[g.Name]; ok {
			cur = p
		}
		arts, err := e.Store.ListNewsAfter(e.Ctx, g.Name, cur)
		if err != nil {
			return err
		}
	articles:
		for _, a := range arts {
			shown++
			printNewsHead(e, a)
			if nonstop {
				printNewsBody(e, a)
				progress[g.Name] = a.Num
				continue
			}
			act, err := newsPrompt(e, a)
			if err != nil {
				return err
			}
			switch act.kind {
			case naNext:
				progress[g.Name] = a.Num
			case naNextGroup:
				progress[g.Name] = a.Num
				break articles
			case naUnsub:
				progress[g.Name] = a.Num
				if err := e.Store.UnsubscribeNews(e.Ctx, e.Sess.User.ID, g.Name); err != nil {
					return err
				}
				e.Sess.Print(e.Sess.T("news.unsubscribed", g.Name) + "\n")
				break articles
			case naJump:
				progress[g.Name] = a.Num
				gi = newsJumpTo(&queue, act.group, gi)
				break articles
			case naQuitSave:
				progress[g.Name] = a.Num
				return saveNewsProgress(e, progress)
			case naQuitNoSave:
				return nil
			}
		}
	}
	if shown == 0 {
		e.Sess.Print("No news.\n")
		return nil
	}
	return saveNewsProgress(e, progress)
}

// newsJumpTo は名指しされたグループへ跳ぶ。既に列にあればその位置、無ければ末尾に足す。
// 返り値は「次に gi++ したときそのグループになる」インデックス。
func newsJumpTo(queue *[]store.NewsGroup, name string, cur int) int {
	for i, q := range *queue {
		if q.Name == name {
			return i - 1
		}
	}
	*queue = append(*queue, store.NewsGroup{Name: name})
	return len(*queue) - 2
}

func newsPrompt(e *Env, a store.NewsArticle) (newsAct, error) {
	body := false
	for {
		e.Sess.Printf("%s NEWS> ", a.Group)
		line, err := e.Sess.ReadCommand(48)
		if err != nil {
			return newsAct{}, err
		}
		raw := strings.TrimSpace(session.FoldCommand(line))
		fields := strings.Fields(raw)
		key := ""
		if len(fields) > 0 {
			key = fields[0]
		}
		arg := ""
		if len(fields) > 1 {
			arg = strings.TrimSpace(raw[len(key):])
		}
		switch {
		case raw == "":
			if !body {
				printNewsBody(e, a)
				body = true
				continue
			}
			return newsAct{kind: naNext}, nil
		case raw == "n" || raw == "+" || raw == ";":
			return newsAct{kind: naNext}, nil
		case raw == "q":
			return newsAct{kind: naQuitSave}, nil
		case raw == "x":
			return newsAct{kind: naQuitNoSave}, nil
		case raw == "U":
			return newsAct{kind: naUnsub}, nil
		case key == "N":
			if arg == "" {
				return newsAct{kind: naNextGroup}, nil
			}
			g, err := e.Store.GetNewsGroup(e.Ctx, arg)
			if err != nil {
				e.Sess.Print("no such group\n")
				continue
			}
			return newsAct{kind: naJump, group: g.Name}, nil
		case raw == "r":
			if err := newsReply(e, a); err != nil {
				return newsAct{}, err
			}
			continue
		case raw == "f":
			if !canPostNews(e) {
				e.Sess.Print("postnews: permission denied\n")
				continue
			}
			if err := postNewsFollow(e, a); err != nil {
				return newsAct{}, err
			}
			continue
		case raw == "i":
			// 一覧: 今のグループの見出しを出す。三種の場で i=一覧 に揃える。
			printNewsGroupHeads(e, a.Group)
			continue
		case raw == "?" || raw == "-?":
			Usage(e, "readnews")
			continue
		default:
			e.Sess.Print(e.Sess.T("news.guide2") + "\n")
		}
	}
}

// newsReply は記事の著者へメールする（題は Re: …）。UNIX readnews の r。
func newsReply(e *Env, a store.NewsArticle) error {
	to := strings.ToLower(strings.TrimSpace(a.FromID))
	if to == "" {
		e.Sess.Print(e.Sess.T("news.no_dest") + "\n")
		return nil
	}
	if _, err := e.Store.GetUser(e.Ctx, to); err != nil {
		e.Sess.Print(e.Sess.T("news.no_author") + "\n")
		return nil
	}
	subj := a.Subject
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	e.Sess.Print(e.Sess.T("news.subj_re", subj))
	line, err := e.Sess.ReadLine(store.MaxMailSubject)
	if err != nil {
		return err
	}
	if t := strings.TrimSpace(line); t != "" {
		subj = t
	}
	subj = session.ClipRunes(subj, store.MaxMailSubject)
	raw, ok, err := composeBody(e)
	if err != nil || !ok {
		return err
	}
	body := store.ApplyAutosign(raw, e.Sess.User.Autosign)
	return mailSendOne(e, to, subj, body)
}

func cmdPostnews(e *Env) error {
	if err := e.Store.SeedNewsGroups(e.Ctx); err != nil {
		return err
	}
	if !canPostNews(e) {
		return Denied(e, "postnews")
	}
	group := store.CanonicalNewsGroup(e.Args)
	if group == "" {
		e.Sess.Print(e.Sess.T("news.group_prompt", store.DefaultNewsGroup))
		line, err := e.Sess.ReadCommand(32)
		if err != nil {
			return err
		}
		group = store.CanonicalNewsGroup(line)
		if group == "" {
			group = store.DefaultNewsGroup
		}
	}
	if _, err := e.Store.EnsureNewsGroup(e.Ctx, group); err != nil {
		if err == store.ErrTooManyNews {
			e.Sess.Print(e.Sess.T("news.too_many_groups") + "\n")
			return nil
		}
		return err
	}
	return postNewsNew(e, group, "", 0)
}

func postNewsFollow(e *Env, parent store.NewsArticle) error {
	subj := parent.Subject
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	return postNewsNew(e, parent.Group, subj, parent.Num)
}

func postNewsNew(e *Env, group, subj string, ref int) error {
	if subj == "" {
		e.Sess.Print(e.Sess.T("news.subj"))
		line, err := e.Sess.ReadLine(store.MaxNewsSubject)
		if err != nil {
			return err
		}
		subj = strings.TrimSpace(line)
		if subj == "" {
			subj = e.Sess.T("news.untitled")
		}
	} else {
		e.Sess.Print(e.Sess.T("news.subj_re", subj))
		line, err := e.Sess.ReadLine(store.MaxNewsSubject)
		if err != nil {
			return err
		}
		if t := strings.TrimSpace(line); t != "" {
			subj = t
		}
	}
	subj = session.ClipRunes(subj, store.MaxNewsSubject)
	raw, ok, err := composeBody(e)
	if err != nil || !ok {
		return err
	}
	body := store.ApplyAutosign(raw, e.Sess.User.Autosign)
	a, err := e.Store.PostNews(e.Ctx, store.NewsArticle{
		Group:      group,
		FromID:     e.Sess.User.ID,
		FromHandle: e.Sess.User.Handle,
		Subject:    subj,
		Body:       body,
		RefNum:     ref,
	})
	if err == store.ErrTooManyNews {
		e.Sess.Print("** too many articles **\n")
		return nil
	}
	if err != nil {
		return err
	}
	e.Sess.Print(e.Sess.T("news.posted", a.Group, a.Num) + "\n")
	return nil
}

func parseNewsArgs(args string) (mode int, group string, ok bool) {
	mode = newsNormal
	ok = true
	for _, f := range strings.Fields(session.FoldCommand(args)) {
		switch f {
		case "-n", "-N":
			mode = newsNonstop
		case "-l", "-L":
			mode = newsList
		case "-c", "-C":
			mode = newsCheck
		default:
			if strings.HasPrefix(f, "-") {
				return newsNormal, "", false
			}
			group = store.CanonicalNewsGroup(f)
		}
	}
	return mode, group, true
}

func canPostNews(e *Env) bool {
	if e.ACL != nil {
		if c, ok := e.ACL.LookupCommand("postnews"); ok {
			return acl.Allowed(e.Sess.User.Flags, c.Allow)
		}
	}
	return acl.Allowed(e.Sess.User.Flags, acl.FlagSys|acl.FlagCos)
}

func printNewsHead(e *Env, a store.NewsArticle) {
	e.Sess.Print("\n")
	e.Sess.Printf("Newsgroup: %s\n", a.Group)
	e.Sess.Printf("Article:   %d\n", a.Num)
	if a.RefNum > 0 {
		e.Sess.Printf("Followup:  %s:%d\n", a.Group, a.RefNum)
	}
	e.Sess.Printf("From:      %s (%s)\n", a.FromID, a.FromHandle)
	e.Sess.Printf("Date:      %s\n", a.Posted.Format("2006-01-02 15:04:05"))
	e.Sess.Printf("Subject:   %s\n", a.Subject)
}

func printNewsBody(e *Env, a store.NewsArticle) {
	e.Sess.Print("\n")
	e.Sess.Print(a.Body)
	if !strings.HasSuffix(a.Body, "\n") {
		e.Sess.Print("\n")
	}
	if warn.HasLink(a.Body) {
		e.Sess.Print(warn.External + "\n")
	}
}

func saveNewsProgress(e *Env, progress map[string]int) error {
	for group, last := range progress {
		if err := e.Store.SetNewsCursor(e.Ctx, e.Sess.User.ID, group, last); err != nil {
			return err
		}
	}
	return nil
}
