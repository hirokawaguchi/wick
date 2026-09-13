package agent

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/menu"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

// pilot は 1 体分。人間と同じコマンドループをパイプ越しに回し、
// 心拍ごとに頭脳のスクリプトを入力へ流し込む。
type pilot struct {
	spec    Spec
	sess    *session.Session
	io      *session.AgentIO
	brain   Brain
	tempo   *tempo
	host    *host.Host
	jobFeed func() []Job // 求人観測（worker のみ。nil 可）
	cancel  context.CancelFunc

	mu      sync.Mutex
	running bool
}

func (m *Manager) startPilot(sp Spec, u store.User) (*pilot, error) {
	sess, aio := session.NewPipe("agent", u.ID, u.Handle)
	sess.User = u
	sess.SetDoing("MAIN")
	if !m.host.EnterAgent(sess) {
		_ = aio.Close()
		return nil, ErrNoRoom
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &pilot{
		spec:    sp,
		sess:    sess,
		io:      aio,
		brain:   m.buildBrainFor(sp, u),
		tempo:   m.tempo,
		host:    m.host,
		cancel:  cancel,
		running: true,
	}
	if sp.Behavior == "worker" {
		p.jobFeed = m.jobFeed
	}

	// (1) 人間と同じコマンドループ。入力パイプが閉じられると EOF で抜ける。
	go func() {
		env := &command.Env{
			Ctx:    ctx,
			Sess:   sess,
			Host:   m.host,
			Store:  m.store,
			ACL:    m.acl,
			Assets: m.assets,
		}
		_ = (&menu.Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
		m.host.LeaveAgent(u.ID)
		p.setRunning(false)
		m.forget(u.ID)
	}()

	// (2) 心拍。頭脳の手を入力へ流す。
	go p.heartbeat(ctx, m.host.ShutdownC())

	return p, nil
}

func (p *pilot) heartbeat(ctx context.Context, shutdown <-chan struct{}) {
	hb := p.spec.Heartbeat
	if hb <= 0 {
		hb = 3 * time.Second
	}
	t := time.NewTicker(hb)
	defer t.Stop()
	budget := p.spec.Budget
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-shutdown:
			return
		case <-t.C:
			if budget == 0 {
				return // 予算切れ。ループ側は idle のまま在室を続ける
			}
			tick++
			screen := p.io.Drain()
			doing := p.sess.GetDoing()
			room, inRoom := parseRoom(doing)
			if inRoom && p.tempo != nil && screenHasHumanSpeech(screen, p.tempo.isAgent) {
				p.tempo.humanSpoke(room, time.Now()) // 人が来たら連続ターンを解く
			}
			obs := Observation{Screen: screen, Tick: tick, Doing: doing, Now: time.Now()}
			// チャット部屋にいるときは「人が見ているか（観客の有無）」を観測に載せる。
			// 無人の部屋では頭脳が自発発話を止め、人が入ると活発に会話する。
			if inRoom {
				obs.Audience = p.host.HumanInRoom(room)
			}
			if p.jobFeed != nil && !inRoom {
				obs.Jobs = p.jobFeed() // MAIN にいるときだけ求人を観測
			}
			script, done := p.brain.Next(obs)
			if done {
				return
			}
			if script == "" {
				continue // 観測の結果「黙る」。予算は減らさない
			}
			// 部屋にいるときは監督のテンポ（floor/間隔/連続ターン上限）に従う。
			if inRoom && p.tempo != nil && !p.tempo.tryGrant(room, p.spec.ID, time.Now()) {
				continue // いまは喋らない。予算も減らさない
			}
			if err := p.io.Feed(script); err != nil {
				return
			}
			if budget > 0 {
				budget-- // 予算は「実際に打った手数」で数える
			}
		}
	}
}

func (p *pilot) stop() {
	p.cancel()
	p.sess.Close() // AgentIO.Close で入力を閉じ、コマンドループを EOF 終了させる
}

func (p *pilot) isRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *pilot) setRunning(v bool) {
	p.mu.Lock()
	p.running = v
	p.mu.Unlock()
}

// parseRoom は Doing 表示（"CHAT1" など）からチャット部屋番号を取り出す。
// 部屋にいないとき（MAIL/MAIN 等）は ok=false。
func parseRoom(doing string) (int, bool) {
	const p = "CHAT"
	if !strings.HasPrefix(doing, p) {
		return 0, false
	}
	n, err := strconv.Atoi(doing[len(p):])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
