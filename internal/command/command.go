package command

import (
	"context"
	"errors"
	"strings"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

var (
	ErrLogOff  = errors.New("logoff")
	ErrDenied  = errors.New("denied")
	ErrUnknown = errors.New("unknown")
	ErrLater   = errors.New("not implemented")
)

type Env struct {
	Ctx    context.Context
	Sess   *session.Session
	Host   *host.Host
	Store  store.Store
	ACL    *acl.Table
	Assets assets.Dir
	Args   string
	Agents AgentControl // エージェント常駐の起動/停止/一覧（注入。nil 可）
}

// AgentInfo は agent コマンドの一覧表示用。
type AgentInfo struct {
	ID      string
	Handle  string
	Doing   string
	Running bool
}

// AgentControl はエージェント常駐の制御口。internal/agent の Manager が実装し、
// 配線時に Env へ注入する。command は agent パッケージを import しない（循環回避）。
type AgentControl interface {
	StartAgent(id string) error
	StopAgent(id string) bool
	ListAgents() []AgentInfo
}

type Func func(*Env) error

type Registry struct {
	fn map[string]Func
}

func NewRegistry() *Registry {
	r := &Registry{fn: map[string]Func{}}
	r.registerBuiltins()
	r.registerNotes()
	r.registerSocial()
	r.registerMail()
	r.registerNews()
	r.registerSignup()
	r.registerGame()
	return r
}

func (r *Registry) Register(name string, f Func) {
	r.fn[strings.ToLower(name)] = f
}

func (r *Registry) Dispatch(env *Env, name string) error {
	name = strings.ToLower(name)
	f, ok := r.fn[name]
	if !ok {
		return ErrLater
	}
	return f(env)
}
