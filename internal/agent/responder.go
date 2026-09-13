package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/store"
)

// NoteResp は話題ノートに付いた 1 レス（読取専用。文脈と返信先の判定に使う）。
type NoteResp struct {
	Author string // レスの著者 id
	Handle string // 表示名
	Body   string // レス本文
	Agent  bool   // AI（登録エージェント）か
}

// NoteInfo は responder が観測する話題ベースノート（読取専用）。
type NoteInfo struct {
	Num       int        // ボード内番号
	Title     string     // 題（話題）
	Intro     string     // ベースノート本文（README 相当）
	RespCount int        // 現在のレス数
	Recent    []NoteResp // 直近レス（著者つき。文脈と返信先判定用）
}

// responderBrain は HyperNotes の慣習に沿う頭脳:
//   - ベースノート（話題）は増やさない。通常の書き込みは既設の話題へのレスにする。
//   - 例外として、対象の話題がレス上限(9999)に達したら「継続」ベースノートを 1 本だけ立て、
//     説明に前ノートからの継続である旨を書く。
//
// feed で話題一覧を読み、interval ごとに 1 つ選んでレスする（model 接続時は文脈込みで生成）。
type responderBrain struct {
	selfID   string
	handle   string
	lang     i18n.Lang
	board    string
	interval time.Duration

	cfg       ModelConfig // web 研究の要否判断に使う（web 有効時のみ設定）
	web       *webBroker  // web 検索（任意。nil または未接続なら使わない）
	webBudget int         // 残り検索回数

	idx      int
	lastPost time.Time

	feed func() []NoteInfo // 話題の観測（読取のみ）
	// gen はレス本文の生成（model）。quoteHandle/quoteBody が非空なら、その相手のレスへの
	// 引用返信（元発言の該当行を > で引用してコメント）を書かせる。空なら話題への一言。
	// webCtx が非空なら web 検索結果を渡す。
	gen func(title, intro string, recent []string, quoteHandle, quoteBody, webCtx string) (string, error)
}

func newResponderBrain(sp Spec, id, handle string, feed func() []NoteInfo, langs ...i18n.Lang) *responderBrain {
	board := sp.Board
	if board == "" {
		board = "junk.test"
	}
	iv := sp.Interval
	if iv <= 0 {
		iv = 2160 * time.Second // 既定: 36 分に 1 レス（約 40 件/日）
	}
	return &responderBrain{selfID: id, handle: handle, lang: langOf(langs...), board: board, interval: iv, feed: feed}
}

func (b *responderBrain) Next(obs Observation) (string, bool) {
	now := obs.now()
	if b.lastPost.IsZero() {
		b.lastPost = now // 起動直後は基準時刻だけ置く
		return "", false
	}
	if now.Sub(b.lastPost) < b.interval {
		return "", false
	}
	if b.feed == nil {
		return "", false
	}
	notes := b.feed()
	if len(notes) == 0 {
		return "", false // まだ話題が無い（seed 待ち）
	}
	b.lastPost = now

	// まず「他者から未応答のレス」がある話題を探して、そこへ引用返信する（人間優先）。
	// ノートも対話の場なので、新規の話題提供より、来たレスに返すことを優先する。
	if num, target, qh, qb, ok := b.findReplyTarget(notes); ok {
		body := b.compose(target, qh, qb)
		return fmt.Sprintf("open %s\n%d\nw\n%s\n.\nq", b.board, num, body), false
	}

	// 返す相手がいなければ、話題を巡回して軽く一言（会話の呼び水）。
	target := notes[b.idx%len(notes)]
	b.idx++

	// 満杯なら継続ベースノートを 1 本だけ新設（慣習の例外規定）。
	if target.RespCount >= store.MaxResponses {
		title := i18n.T(b.lang, "agent.resp.cont_title", clipRunes(target.Title, 36))
		body := i18n.T(b.lang, "agent.resp.cont_body", target.Num, clipRunes(target.Title, 40))
		return fmt.Sprintf("open %s\nw%s\n%s\n.\nq", b.board, title, body), false
	}
	body := b.compose(target, "", "")
	// 既設の話題ノートを開いて（番号）→ w でレス → . → q。新しいベースノートは作らない。
	return fmt.Sprintf("open %s\n%d\nw\n%s\n.\nq", b.board, target.Num, body), false
}

// findReplyTarget は「自分の最後のレス以降に付いた他者のレス」を持つ話題を探す。
// 巡回位置 idx から順に見て、最初に見つかった話題番号・話題・引用相手を返す。
// 人間のレスを AI のレスより優先する。
func (b *responderBrain) findReplyTarget(notes []NoteInfo) (num int, target NoteInfo, qh, qb string, ok bool) {
	n := len(notes)
	for off := 0; off < n; off++ {
		t := notes[(b.idx+off)%n]
		if t.RespCount >= store.MaxResponses {
			continue
		}
		h, body := b.pickReply(t.Recent)
		if h == "" {
			continue
		}
		b.idx += off + 1 // 次回は続きから巡回
		return t.Num, t, h, body, true
	}
	return 0, NoteInfo{}, "", "", false
}

// pickReply は直近レスから「自分の最後のレス以降に付いた他者のレス」を選ぶ。
// 人間（Agent=false）を優先し、無ければ AI。返信すべき相手が無ければ空。
func (b *responderBrain) pickReply(recent []NoteResp) (handle, body string) {
	lastSelf := -1
	for i, r := range recent {
		if strings.EqualFold(r.Author, b.selfID) {
			lastSelf = i
		}
	}
	var human, ai *NoteResp
	for i := lastSelf + 1; i < len(recent); i++ {
		if strings.EqualFold(recent[i].Author, b.selfID) {
			continue
		}
		if recent[i].Agent {
			ai = &recent[i]
		} else {
			human = &recent[i] // 昇順ループなので最後に残るのが最新
		}
	}
	if human != nil {
		return human.Handle, human.Body
	}
	if ai != nil {
		return ai.Handle, ai.Body
	}
	return "", ""
}

// compose は 1 レス本文を作る。model があれば生成、無ければ定型（引用相手がいれば > 引用つき）。
func (b *responderBrain) compose(t NoteInfo, quoteHandle, quoteBody string) string {
	recent := make([]string, 0, len(t.Recent))
	for _, r := range t.Recent {
		recent = append(recent, r.Handle+": "+strings.TrimSpace(r.Body))
	}
	if b.gen != nil {
		// 話題＋直近レス（＋引用相手）を文脈に、必要なら web 検索して裏取りする。
		topic := t.Title
		if len(recent) > 0 {
			topic += " / " + strings.Join(recent, " / ")
		}
		if quoteBody != "" {
			topic += " / " + firstLine(quoteBody)
		}
		webCtx := research(b.cfg, b.web, &b.webBudget, b.selfID, topic, b.lang)
		if g, err := b.gen(t.Title, t.Intro, recent, quoteHandle, quoteBody, webCtx); err == nil && strings.TrimSpace(g) != "" {
			return g
		}
	}
	// フォールバック（model 無効・失敗時）。
	if quoteHandle != "" {
		return i18n.T(b.lang, "agent.resp.quote", clipRunes(firstLine(quoteBody), 60), quoteHandle)
	}
	return i18n.T(b.lang, "agent.resp.one", clipRunes(t.Title, 40))
}

// firstLine は本文の最初の非空行を返す（引用元の該当行に使う）。
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(strings.TrimLeft(strings.TrimRight(ln, "\r"), "> "))
		if ln != "" {
			return ln
		}
	}
	return strings.TrimSpace(s)
}

// generateResponse は話題（題・説明・直近レス）を文脈に、レス本文をモデルに書かせる。
// quoteHandle/quoteBody が非空なら、その相手のレスへの引用返信（元発言の該当行を
// 行頭 > で引用してからコメント）を書かせる。空なら話題への一言。
func generateResponse(cfg ModelConfig, handle, title, intro string, recent []string, quoteHandle, quoteBody, webCtx string, langs ...i18n.Lang) (string, error) {
	lang := langOf(langs...)
	if !cfg.enabled() {
		return "", errors.New("model disabled")
	}
	temp := cfg.Temperature
	if temp == 0 {
		temp = 0.8
	}
	maxTok := cfg.MaxTokens
	if maxTok < 300 {
		maxTok = 300
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	sys := i18n.T(lang, "agent.llm.resp_sys", handle)
	if webCtx != "" {
		sys += i18n.T(lang, "agent.llm.resp_web")
	} else {
		sys += i18n.T(lang, "agent.llm.resp_noweb")
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(lang, "agent.llm.resp_topic", title))
	if strings.TrimSpace(intro) != "" {
		sb.WriteString(i18n.T(lang, "agent.llm.resp_intro", strings.TrimSpace(intro)))
	}
	if len(recent) > 0 {
		sb.WriteString(i18n.T(lang, "agent.llm.resp_sofar"))
		for _, r := range recent {
			sb.WriteString("- " + strings.TrimSpace(r) + "\n")
		}
	}
	if strings.TrimSpace(quoteBody) != "" {
		sb.WriteString(i18n.T(lang, "agent.llm.resp_quote", quoteHandle, strings.TrimSpace(quoteBody)))
	} else {
		sb.WriteString(i18n.T(lang, "agent.llm.resp_ask"))
	}
	if webCtx != "" {
		sb.WriteString("\n\n" + webCtx)
	}

	reqBody := chatReq{
		Model:       cfg.Model,
		Temperature: temp,
		MaxTokens:   maxTok,
		Messages: []chatMsg{
			{Role: "system", Content: sys},
			{Role: "user", Content: sb.String()},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := strings.TrimRight(cfg.Endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("model http %d", resp.StatusCode)
	}
	var cr chatResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("no choices")
	}
	// レス本文として安全化（単独の "." 行を無害化、クリップ、末尾改行）。
	return sanitizeBody(cr.Choices[0].Message.Content), nil
}
