package agent

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/store"
)

var (
	// ErrUnknownAgent は登録されていない ID。
	ErrUnknownAgent = errors.New("unknown agent")
	// ErrAlreadyRunning は既に稼働中。
	ErrAlreadyRunning = errors.New("already running")
	// ErrNoRoom は在室枠が足りない。
	ErrNoRoom = errors.New("no room for agent")
	// ErrNoAccount はエージェント口座（ユーザー）が無い。
	ErrNoAccount = errors.New("agent account not found")
)

// Manager は複数エージェントの登録・起動・停止・一覧を持つ。
// command.AgentControl を実装し、Env へ注入される。
type Manager struct {
	host   *host.Host
	store  store.Store
	acl    *acl.Table
	assets assets.Dir

	mu       sync.Mutex
	specs    map[string]Spec
	order    []string
	active   map[string]*pilot
	tempo    *tempo
	modelCfg ModelConfig
	lang     i18n.Lang // 局の既定言語。口座 Lang が空のとき頭脳が使う
	// webProvider は web 検索の実体。nil ならスタブ（固定ヒット）。
	// 将来 MCP クライアントをここに差す（SetWebProvider）。
	webProvider WebProvider

	// janitor（決着ジョブの自動クローズ）の設定。テストで差し替えられる。
	janitorEvery time.Duration
	jobSettle    time.Duration
	jobQuorum    int
}

// SetModel は実 Brain（OpenAI 互換）の接続設定を与える。Endpoint 空なら偽頭脳のまま。
func (m *Manager) SetModel(cfg ModelConfig) {
	m.mu.Lock()
	m.modelCfg = cfg
	m.mu.Unlock()
}

// SetLang は局の既定言語を頭脳に渡す（口座 Lang が空のときのフォールバック）。
func (m *Manager) SetLang(lang i18n.Lang) {
	m.mu.Lock()
	m.lang = i18n.Normalize(string(lang))
	m.mu.Unlock()
}

// agentLang は口座の言語があればそれ、なければ局の既定。
func (m *Manager) agentLang(u store.User) i18n.Lang {
	if strings.TrimSpace(u.Lang) != "" {
		return i18n.Normalize(u.Lang)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return i18n.Normalize(string(m.lang))
}

// SetWebProvider は web 検索の実体を差し替える（既定はスタブ）。
// 増分2 で MCP クライアントをここに渡す。
func (m *Manager) SetWebProvider(p WebProvider) {
	m.mu.Lock()
	m.webProvider = p
	m.mu.Unlock()
}

// newWebBroker は web 検索の入口を作る（Provider＋回数上限＋タイムアウト＋監査）。
// 呼び手（modelBrain）が能力(Spec.Web)と回数予算を持つ。
func (m *Manager) newWebBroker() *webBroker {
	m.mu.Lock()
	p := m.webProvider
	m.mu.Unlock()
	live := p != nil // 実プロバイダ（MCP 等）が接続されているか
	if p == nil {
		p = noopProvider{} // 未接続なら空を返す＝web は事実上無効
	}
	b := &webBroker{
		provider: p,
		live:     live,
		limit:    5,
		timeout:  8 * time.Second,
		audit: func(id, q string, n int) {
			log.Printf("agent %s web: %q → %d hits", id, q, n)
		},
	}
	// Provider が取得（web-get）に対応していれば fetcher も差す。
	if f, ok := p.(WebFetcher); ok {
		b.fetcher = f
	}
	return b
}

// webBudgetOf は Spec の web 予算（0 なら既定 20）。
func webBudgetOf(sp Spec) int {
	if sp.WebBudget > 0 {
		return sp.WebBudget
	}
	return 20
}

// buildBrainFor は Spec に応じた頭脳を作る。behavior=model かつ接続設定があれば
// 実モデル、無ければ会話型の偽頭脳へフォールバック（デプロイを壊さない）。
func (m *Manager) buildBrainFor(sp Spec, u store.User) Brain {
	m.mu.Lock()
	cfg := m.modelCfg
	m.mu.Unlock()
	inner := m.buildInner(sp, u, cfg)
	// 反応型 conversant には「相手がエージェントか」を教える（AI 同士の相づち往復を
	// 数回で打ち止めるため）。
	if cb, ok := inner.(*conversantBrain); ok && cb.isAgent == nil {
		cb.isAgent = m.isAgentID
	}
	// MAIN に居がちな頭脳（worker/responder）は、idle のあいだノートを読みに行かせ、
	// who での居場所を人間のように動いて見せる（読取のみ）。
	switch sp.Behavior {
	case "worker", "responder":
		flags := u.Flags
		return newWanderBrain(inner, func() []string { return m.readableBoards(flags) }, u.ID)
	}
	return inner
}

// buildInner は Spec に応じた本来の頭脳を作る（wander で包む前の中身）。
func (m *Manager) buildInner(sp Spec, u store.User, cfg ModelConfig) Brain {
	lang := m.agentLang(u)
	switch sp.Behavior {
	case "model":
		if cfg.enabled() {
			mb := newModelBrain(cfg, sp, u.ID, u.Handle, lang)
			if sp.Web || sp.WebGet {
				mb.web = m.newWebBroker()
				mb.webBudget = webBudgetOf(sp)
				mb.webSearch = sp.Web && mb.web.live           // 実接続時のみ検索を有効化
				mb.webGet = sp.WebGet && mb.web.fetcher != nil // fetcher 無しなら取得不可
			}
			return mb
		}
		sp2 := sp
		sp2.Behavior = "conversant"
		return buildBrain(sp2, u.ID, u.Handle, lang)
	case "poster":
		pb := newPosterBrain(sp, u.ID, u.Handle, lang)
		if cfg.enabled() {
			handle := u.Handle
			pb.gen = func(topic, webCtx string) (string, string, error) {
				return generateNote(cfg, handle, topic, webCtx, lang)
			}
			if sp.Web {
				pb.web = m.newWebBroker()
				pb.webBudget = webBudgetOf(sp)
				pb.cfg = cfg
			}
		}
		return pb
	case "talker":
		tb := newTalkerBrain(cfg, sp, u.ID, u.Handle, lang)
		if cfg.enabled() && sp.Web {
			tb.web = m.newWebBroker()
			tb.webBudget = webBudgetOf(sp)
		}
		return tb
	case "responder":
		board := sp.Board
		if board == "" {
			board = "junk.test"
		}
		rb := newResponderBrain(sp, u.ID, u.Handle, func() []NoteInfo { return m.noteFeed(board) }, lang)
		if cfg.enabled() {
			handle := u.Handle
			rb.gen = func(title, intro string, recent []string, quoteHandle, quoteBody, webCtx string) (string, error) {
				return generateResponse(cfg, handle, title, intro, recent, quoteHandle, quoteBody, webCtx, lang)
			}
			if sp.Web {
				rb.web = m.newWebBroker()
				rb.webBudget = webBudgetOf(sp)
				rb.cfg = cfg
			}
		}
		return rb
	}
	return buildBrain(sp, u.ID, u.Handle, lang)
}

// NewManager は Manager を作る。
func NewManager(h *host.Host, st store.Store, tbl *acl.Table, as assets.Dir) *Manager {
	m := &Manager{
		host:         h,
		store:        st,
		acl:          tbl,
		assets:       as,
		specs:        map[string]Spec{},
		active:       map[string]*pilot{},
		janitorEvery: janitorEvery,
		jobSettle:    jobSettleAfter,
		jobQuorum:    jobCloseQuorum,
	}
	m.tempo = newTempo(m.isAgentID)
	return m
}

// jobFeed は sys.jobs の未クローズ求人を観測する（worker への入力）。
// 読取のみ。書き込みはエージェントのコマンド経路（ACL 準拠）で行う。
func (m *Manager) jobFeed() []Job {
	ctx := context.Background()
	b, err := m.store.GetBoard(ctx, "sys.jobs")
	if err != nil {
		return nil
	}
	notes, err := m.store.ListNotes(ctx, b.ID)
	if err != nil {
		return nil
	}
	out := make([]Job, 0, len(notes))
	for _, n := range notes {
		if n.Flags&store.MsgClosed != 0 {
			continue
		}
		reps, _ := m.store.ListResponses(ctx, n.ID)
		who := make([]string, 0, len(reps))
		for _, r := range reps {
			who = append(who, r.Author)
		}
		out = append(out, Job{Num: n.Num, Title: n.Title, Author: n.Author, Responders: who, Updated: n.LastUpdate})
	}
	return out
}

// noteFeed は responder への入力。ボードの話題ベースノート（未クローズ）を
// 番号・題・説明・レス数・直近レスとして読取る（読取のみ。書き込みはコマンド経路）。
func (m *Manager) noteFeed(board string) []NoteInfo {
	ctx := context.Background()
	b, err := m.store.GetBoard(ctx, board)
	if err != nil {
		return nil
	}
	notes, err := m.store.ListNotes(ctx, b.ID)
	if err != nil {
		return nil
	}
	out := make([]NoteInfo, 0, len(notes))
	for _, n := range notes {
		if n.Flags&(store.MsgDeleted|store.MsgClosed) != 0 {
			continue
		}
		reps, _ := m.store.ListResponses(ctx, n.ID)
		recent := make([]NoteResp, 0, len(reps))
		for _, r := range reps {
			if r.Flags&store.MsgDeleted != 0 {
				continue
			}
			recent = append(recent, NoteResp{
				Author: r.Author,
				Handle: r.Handle,
				Body:   r.Body,
				Agent:  m.isAgentID(r.Author),
			})
		}
		if len(recent) > 6 {
			recent = recent[len(recent)-6:]
		}
		out = append(out, NoteInfo{Num: n.Num, Title: n.Title, Intro: n.Body, RespCount: n.Response, Recent: recent})
	}
	return out
}

// readableBoards はそのユーザーが読めるボード名の一覧（wander の巡回先）。読取のみ。
func (m *Manager) readableBoards(flags uint32) []string {
	ctx := context.Background()
	bs, err := m.store.ListBoards(ctx)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		if b.CanRead(flags) {
			out = append(out, b.Name)
		}
	}
	return out
}

// isAgentID は登録済みエージェント（＝人間ではない）かどうか。テンポ監督が使う。
func (m *Manager) isAgentID(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.specs[strings.ToLower(id)]
	return ok
}

// Register は Spec を登録する（AGENTS.txt 由来）。
func (m *Manager) Register(sp Spec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specs[sp.ID]; !ok {
		m.order = append(m.order, sp.ID)
	}
	m.specs[sp.ID] = sp
}

// StartConfigured は AutoStart 指定のエージェントを起動する（boot 時）。
func (m *Manager) StartConfigured() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if m.specs[id].AutoStart {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.StartAgent(id)
	}
}

// StartAgent は登録済みエージェントを起動する。command.AgentControl。
func (m *Manager) StartAgent(id string) error {
	m.mu.Lock()
	sp, ok := m.specs[id]
	if !ok {
		m.mu.Unlock()
		return ErrUnknownAgent
	}
	if _, running := m.active[id]; running {
		m.mu.Unlock()
		return ErrAlreadyRunning
	}
	m.mu.Unlock()

	u, err := m.store.GetUser(context.Background(), id)
	if err != nil {
		return ErrNoAccount
	}
	p, err := m.startPilot(sp, u)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.active[id] = p
	m.mu.Unlock()
	return nil
}

// StopAgent は稼働中エージェントを止める。command.AgentControl。
func (m *Manager) StopAgent(id string) bool {
	m.mu.Lock()
	p, ok := m.active[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	p.stop()
	return true
}

// forget は稼働終了したエージェントを active から外す（pilot から呼ぶ）。
func (m *Manager) forget(id string) {
	m.mu.Lock()
	delete(m.active, id)
	m.mu.Unlock()
}

// ListAgents は登録済みエージェントの状態一覧。command.AgentControl。
func (m *Manager) ListAgents() []command.AgentInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]command.AgentInfo, 0, len(m.specs))
	for _, id := range m.order {
		sp := m.specs[id]
		info := command.AgentInfo{ID: id, Handle: sp.ID}
		if p, ok := m.active[id]; ok {
			info.Running = p.isRunning()
			info.Handle = p.sess.User.Handle
			info.Doing = p.sess.GetDoing()
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
