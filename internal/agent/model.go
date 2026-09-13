package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/session"
)

// ModelConfig は実 Brain（OpenAI 互換 Chat Completions）への接続設定。
// Endpoint は /v1 までのベース URL（例: https://api.openai.com/v1、
// ローカルは http://localhost:11434/v1）。空なら実 Brain は無効。
type ModelConfig struct {
	Endpoint    string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
	TokenBudget int // 1 体あたりの累計トークン上限（0 で無制限）
}

func (c ModelConfig) enabled() bool { return c.Endpoint != "" && c.Model != "" }

// action はモデルが返す構造化アクション。生キー列は出させず、
// ここで許した種類だけを Pilot がコマンドへ翻訳する（安全側）。
type action struct {
	Action string `json:"action"` // say / telegram / web / get / idle
	Text   string `json:"text"`
	Target string `json:"target"` // telegram の宛先 id
}

// modelBrain は実モデル駆動の頭脳。チャット部屋に入り、観測（画面）をもとに
// 次の一手（say/telegram/idle）を JSON で決めさせ、コマンド列へ翻訳する。
type modelBrain struct {
	cfg     ModelConfig
	client  *http.Client
	selfID  string
	handle  string
	lang    i18n.Lang
	room    int
	entered bool

	silence      int // 連続沈黙の心拍数（LLM 呼び出しの間引き用）
	silenceLimit int // 誰も喋らなくてもこの回数ごとには一度考える
	lastErrLog   time.Time

	// 自発発話の時間帯（分・ローカルTZ）。activeSet が真のとき、窓の外では
	// 自分から話し始めない（話しかけられたら返す＝受け身のみ）。
	activeSet   bool
	activeStart int
	activeEnd   int

	history    []string // 観測した発話のローリング履歴（文脈用。id> 本文 の行）
	maxHistory int      // 保持する行数の上限

	pendingTele teleMsg // 未処理の受信電報（あれば telegram で返す）

	// web 検索ツール（エージェント専用）。web が nil なら能力なし＝使わせない。
	// モデルは {"action":"web","text":"検索語"} を出し、ここで whitelist/予算/監査を通して
	// broker を呼ぶ。結果は webCtx に入れ、次の思考でプロンプトに載せて URL 付きで発言させる。
	web       *webBroker
	webSearch bool   // action=web（検索）を許すか（能力 Spec.Web）
	webGet    bool   // action=get（URL 取得）を許すか（能力 Spec.WebGet ＋ fetcher あり）
	webBudget int    // 1 体あたりの検索＋取得の合計回数上限
	webUsed   int    // これまでの検索＋取得の回数
	webCtx    string // 直近の検索/取得の結果（発言に使ったら消す）

	tokensUsed int  // これまでの累計トークン（usage.total_tokens の合計）
	budgetHit  bool // 予算超過を一度ログしたか
}

// maxWebPerTurn は 1 心拍あたりに許す検索回数（無限ループ防止）。
const maxWebPerTurn = 2

// teleMsg は受信した個人電報（差出人 id と本文）。
type teleMsg struct {
	from string
	body string
}

// TokensUsed はこのエージェントが消費した累計トークン数を返す（観測・テスト用）。
func (b *modelBrain) TokensUsed() int { return b.tokensUsed }

func newModelBrain(cfg ModelConfig, sp Spec, id, handle string, langs ...i18n.Lang) *modelBrain {
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 400 // URL＋やや長めの発言が途中で切れないように
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	room := sp.Room
	if room <= 0 {
		room = 1
	}
	return &modelBrain{
		cfg:          cfg,
		client:       &http.Client{Timeout: cfg.Timeout},
		selfID:       id,
		handle:       handle,
		lang:         langOf(langs...),
		room:         room,
		silenceLimit: 5,
		maxHistory:   16,
		activeSet:    sp.ActiveSet,
		activeStart:  sp.ActiveStart,
		activeEnd:    sp.ActiveEnd,
	}
}

// inActiveWindow は now（ローカルTZ）が自発発話の時間帯かを返す。
// activeSet が偽なら常に true（終日 自発してよい）。日をまたぐ窓にも対応。
func (b *modelBrain) inActiveWindow(now time.Time) bool {
	if !b.activeSet {
		return true
	}
	m := now.Hour()*60 + now.Minute()
	if b.activeStart <= b.activeEnd {
		return m >= b.activeStart && m < b.activeEnd
	}
	// 例: 23:30-00:30 のように日付をまたぐ窓。
	return m >= b.activeStart || m < b.activeEnd
}

func (b *modelBrain) Next(obs Observation) (string, bool) {
	// まずは部屋に入る（ここは決め打ち。以降の発話をモデルが決める）。
	if !b.entered {
		b.entered = true
		return fmt.Sprintf("chat %d\n", b.room), false
	}
	// 観測した発話（自分＋他者）を履歴に積む。沈黙で LLM を呼ばない心拍でも
	// 積んでおき、次に考えるときに文脈として渡す（断片だけで迷子にならない）。
	b.ingest(obs.Screen)
	// トークン予算を使い切ったら、以後は考えず在室のまま黙る（安全側）。
	if b.cfg.TokenBudget > 0 && b.tokensUsed >= b.cfg.TokenBudget {
		if !b.budgetHit {
			b.budgetHit = true
			log.Printf("agent %s: token budget %d exhausted, staying idle (used=%d)",
				b.selfID, b.cfg.TokenBudget, b.tokensUsed)
		}
		return "", false
	}
	// 観客（人間の在室者）がいない部屋では自発発話しない。誰も見ておらず記録も
	// 残らないので、AI 同士だけの会話でトークンを浪費しないため。電報が来たら返す。
	tele := lastTelegram(obs.Screen, b.selfID)
	if !obs.Audience && tele.from == "" {
		if _, in := parseRoom(obs.Doing); in {
			b.silence = 0
			return "", false
		}
	}
	// LLM 呼び出しの間引き: 他者の発言か受信電報があるか、沈黙が一定続いたときだけ考える。
	who, _ := lastOtherUtterance(obs.Screen, b.selfID)
	if who == "" && tele.from == "" {
		// 窓の外では自分から話し始めない（話しかけられたら返す＝受け身）。
		if !b.inActiveWindow(obs.now()) {
			b.silence = 0
			return "", false
		}
		b.silence++
		if b.silenceLimit <= 0 || b.silence < b.silenceLimit {
			return "", false // 何も無いので LLM を呼ばずに静観
		}
	}
	b.silence = 0
	b.pendingTele = tele // 電報があれば userPrompt で「返信して」と促す
	// 思考ループ: モデルが web を要求したら検索し、結果を踏まえて考え直す
	// （1 心拍で maxWebPerTurn 回まで。以降は普通のアクションへ）。
	webThisTurn := 0
	for {
		act, err := b.decide(obs)
		if err != nil {
			if time.Since(b.lastErrLog) > 30*time.Second {
				b.lastErrLog = time.Now()
				log.Printf("agent %s model error: %v", b.selfID, err)
			}
			b.webCtx = ""
			return "", false // 失敗時は黙る（在室のまま）
		}
		if act.Action == "web" {
			q := oneLine(act.Text)
			if b.web != nil && b.webSearch && q != "" && b.webUsed < b.webBudget && webThisTurn < maxWebPerTurn {
				hits := b.web.Search(b.selfID, q) // whitelist/予算は呼び手、監査は broker
				b.webUsed++
				webThisTurn++
				b.webCtx = formatWebHits(q, hits, b.lang)
				continue // 検索結果を踏まえて考え直す
			}
			// 能力なし / 予算切れ / 上限 → 黙る（在室のまま）
			b.webCtx = ""
			return "", false
		}
		if act.Action == "get" {
			u := oneLineN(act.Text, 500) // URL は切らない
			if b.web != nil && b.webGet && u != "" && b.webUsed < b.webBudget && webThisTurn < maxWebPerTurn {
				page := b.web.Get(b.selfID, u) // SSRF/サイズ制限はサーバ側、監査は broker
				b.webUsed++
				webThisTurn++
				b.webCtx = formatWebPage(page, b.lang)
				continue // 取得本文を踏まえて考え直す
			}
			b.webCtx = ""
			return "", false
		}
		// 非 web アクションで確定。使い終えた検索文脈は捨てる。
		b.webCtx = ""
		switch act.Action {
		case "say":
			if _, in := parseRoom(obs.Doing); !in {
				return "", false // 部屋にいないときは発話しない
			}
			txt := sayText(act.Text)
			if txt == "" {
				return "", false
			}
			return txt + "\n", false
		case "telegram":
			t := oneLine(act.Text)
			target := strings.TrimSpace(act.Target)
			if target == "" {
				target = b.pendingTele.from // 宛先未指定なら差出人へ返す
			}
			b.pendingTele = teleMsg{}
			if target == "" || t == "" {
				return "", false
			}
			// 部屋の中では素の "!" は発言扱いになるので、部屋内コマンドの "/!" を使う。
			if _, in := parseRoom(obs.Doing); in {
				return "/! " + target + " " + t + "\n", false
			}
			return "! " + target + " " + t + "\n", false
		default: // idle / 未知
			b.pendingTele = teleMsg{}
			return "", false
		}
	}
}

func (b *modelBrain) decide(obs Observation) (action, error) {
	reqBody := chatReq{
		Model:       b.cfg.Model,
		Temperature: b.cfg.Temperature,
		MaxTokens:   b.cfg.MaxTokens,
		Messages: []chatMsg{
			{Role: "system", Content: b.systemPrompt()},
			{Role: "user", Content: b.userPrompt(obs)},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return action{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.Timeout)
	defer cancel()
	url := strings.TrimRight(b.cfg.Endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return action{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.cfg.APIKey)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return action{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return action{}, fmt.Errorf("model http %d", resp.StatusCode)
	}
	var cr chatResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return action{}, err
	}
	// トークン計上（usage が無いサーバもあるので TotalTokens が 0 なら概算）。
	if cr.Usage.TotalTokens > 0 {
		b.tokensUsed += cr.Usage.TotalTokens
	} else {
		b.tokensUsed += estimateTokens(reqBody) // フォールバックの粗い見積り
	}
	if len(cr.Choices) == 0 {
		return action{}, errors.New("no choices")
	}
	return parseAction(cr.Choices[0].Message.Content)
}

func (b *modelBrain) systemPrompt() string {
	verbs := []string{"say", "telegram"}
	webClause := ""
	if b.webSearch {
		verbs = append(verbs, "web")
		webClause += i18n.T(b.lang, "agent.llm.web_search")
	}
	if b.webGet {
		verbs = append(verbs, "get")
		webClause += i18n.T(b.lang, "agent.llm.web_get")
	}
	if webClause != "" {
		webClause += i18n.T(b.lang, "agent.llm.web_cite")
	}
	verbs = append(verbs, "idle")
	actions := strings.Join(verbs, "|")
	return i18n.T(b.lang, "agent.llm.chat_sys", b.handle, b.selfID) +
		webClause +
		i18n.T(b.lang, "agent.llm.chat_fmt", actions)
}

// ingest は画面断片から「id> 本文」の発話行を抜き出して履歴に積む（上限で古いのを捨てる）。
func (b *modelBrain) ingest(screen string) {
	if screen == "" {
		return
	}
	for _, ln := range strings.Split(screen, "\n") {
		ln = strings.TrimRight(ln, "\r")
		i := strings.Index(ln, "> ")
		if i <= 0 || !isIDToken(ln[:i]) {
			continue
		}
		if strings.TrimSpace(ln[i+2:]) == "" {
			continue // 空プロンプト（"poet> " だけ）は捨てる
		}
		b.history = append(b.history, strings.TrimSpace(ln))
	}
	if b.maxHistory > 0 && len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
}

func (b *modelBrain) userPrompt(obs Observation) string {
	convo := strings.Join(b.history, "\n")
	if r := []rune(convo); len(r) > 1500 {
		convo = string(r[len(r)-1500:]) // 直近だけ渡す
	}
	loc := obs.Doing
	if loc == "" {
		loc = "MAIN"
	}
	var sb strings.Builder
	sb.WriteString(i18n.T(b.lang, "agent.llm.here", loc, convo))
	if b.pendingTele.from != "" {
		sb.WriteString(i18n.T(b.lang, "agent.llm.tele", b.pendingTele.from, b.pendingTele.body, b.pendingTele.from))
	}
	if b.webCtx != "" {
		sb.WriteString("\n" + b.webCtx)
	}
	sb.WriteString(i18n.T(b.lang, "agent.llm.next"))
	return sb.String()
}

// lastTelegram は画面テキストから最後に届いた個人電報（差出人と本文）を取り出す。
// 通知は "** 電報 from <id> … **" / "** telegram from <id> … **" の次行が本文。
func lastTelegram(screen, selfID string) teleMsg {
	var out teleMsg
	lines := strings.Split(screen, "\n")
	for idx, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		rest, ok := telegramFromRest(ln)
		if !ok {
			continue
		}
		sp := strings.IndexByte(rest, ' ')
		if sp <= 0 {
			continue
		}
		from := rest[:sp]
		if !isIDToken(from) || strings.EqualFold(from, selfID) {
			continue
		}
		body := ""
		if idx+1 < len(lines) {
			body = strings.TrimSpace(strings.TrimRight(lines[idx+1], "\r"))
		}
		out = teleMsg{from: from, body: body}
	}
	return out
}

func telegramFromRest(ln string) (string, bool) {
	for _, p := range []string{"** 電報 from ", "** telegram from "} {
		if strings.HasPrefix(ln, p) {
			return ln[len(p):], true
		}
	}
	return "", false
}

func oneLine(s string) string {
	return oneLineN(s, session.TelegramMax)
}

// oneLineN は改行を空白に潰して max runes にクリップする。
func oneLineN(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	return session.ClipRunes(s, max)
}

// sayText はチャット発言用（電報より長め＝ChatMax。URL＋コメントが収まる）。
func sayText(s string) string {
	return oneLineN(s, session.ChatMax)
}

// parseAction はモデル応答から最初の JSON オブジェクトを取り出して解釈する。
func parseAction(s string) (action, error) {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j <= i {
		return action{Action: "idle"}, nil
	}
	var a action
	if err := json.Unmarshal([]byte(s[i:j+1]), &a); err != nil {
		return action{}, err
	}
	a.Action = strings.ToLower(strings.TrimSpace(a.Action))
	return a, nil
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// chatOnce は 1 往復だけの補助呼び出し（研究の要否判断など、単発の質問に使う）。
func chatOnce(cfg ModelConfig, sys, user string, maxTok int, temp float64) (string, error) {
	if !cfg.enabled() {
		return "", errors.New("model disabled")
	}
	if maxTok <= 0 {
		maxTok = 60
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 15 * time.Second
	}
	reqBody := chatReq{
		Model:       cfg.Model,
		Temperature: temp,
		MaxTokens:   maxTok,
		Messages: []chatMsg{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
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
	resp, err := (&http.Client{Timeout: to}).Do(req)
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
	return cr.Choices[0].Message.Content, nil
}

// estimateTokens は usage を返さないサーバ向けの粗い見積り（4 文字≒1 トークン）。
func estimateTokens(req chatReq) int {
	n := 0
	for _, m := range req.Messages {
		n += len([]rune(m.Content))
	}
	n += req.MaxTokens // 出力上限ぶんを上乗せ（保守的に多めに数える）
	return n / 4
}
