package command

import (
	"strings"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

// cmdAgent は sysop 用のエージェント常駐の制御口。
//
//	agent            … 一覧（in/停止中）
//	agent list       … 一覧
//	agent start <id> … 起動
//	agent stop  <id> … 停止（who から消える）
//
// エージェントは人間と同じコマンドループ（mail/chat/notes/電報）を内部で回す。
// 実体の起動・心拍・停止は internal/agent の Manager（Env.Agents）が持つ。
func cmdAgent(e *Env) error {
	if e.Agents == nil {
		e.Sess.Print(e.Sess.T("agent.disabled") + "\n")
		return nil
	}
	fields := strings.Fields(strings.TrimSpace(e.Args))
	if len(fields) == 0 || strings.EqualFold(fields[0], "list") {
		agentList(e)
		return nil
	}
	switch strings.ToLower(fields[0]) {
	case "-?", "?":
		agentUsage(e)
	case "start":
		if len(fields) < 2 {
			agentUsage(e)
			return nil
		}
		if err := e.Agents.StartAgent(fields[1]); err != nil {
			e.Sess.Print(e.Sess.T("agent.start_fail", err) + "\n")
			return nil
		}
		e.Sess.Print(e.Sess.T("agent.started", fields[1]) + "\n")
	case "stop":
		if len(fields) < 2 {
			agentUsage(e)
			return nil
		}
		if e.Agents.StopAgent(fields[1]) {
			e.Sess.Print(e.Sess.T("agent.stopped", fields[1]) + "\n")
		} else {
			e.Sess.Print(e.Sess.T("agent.not_running", fields[1]) + "\n")
		}
	default:
		agentUsage(e)
	}
	return nil
}

func agentUsage(e *Env) {
	e.Sess.Print(e.Sess.T("agent.usage_head") + "\n")
	e.Sess.Print(e.Sess.T("agent.usage_list") + "\n")
	e.Sess.Print(e.Sess.T("agent.usage_start") + "\n")
	e.Sess.Print(e.Sess.T("agent.usage_stop") + "\n")
}

func agentList(e *Env) {
	list := e.Agents.ListAgents()
	if len(list) == 0 {
		e.Sess.Print(e.Sess.T("agent.none") + "\n")
		return
	}
	e.Sess.Printf("%-10s %s %s %s\n", "ID", session.PadRight("Handle", store.MaxHandle), session.PadRight(e.Sess.T("agent.col_state"), 6), "Doing")
	for _, a := range list {
		state := e.Sess.T("agent.state_stopped")
		if a.Running {
			state = e.Sess.T("agent.state_running")
		}
		e.Sess.Printf("%-10s %s %s %s\n", a.ID, session.PadRight(a.Handle, store.MaxHandle), session.PadRight(state, 6), a.Doing)
	}
}
