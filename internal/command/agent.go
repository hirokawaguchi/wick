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
		e.Sess.Print("** エージェント機能が無効です **\n")
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
			e.Sess.Printf("起動できません: %v\n", err)
			return nil
		}
		e.Sess.Printf("-- %s を起動しました --\n", fields[1])
	case "stop":
		if len(fields) < 2 {
			agentUsage(e)
			return nil
		}
		if e.Agents.StopAgent(fields[1]) {
			e.Sess.Printf("-- %s を停止しました --\n", fields[1])
		} else {
			e.Sess.Printf("-- %s は動いていません --\n", fields[1])
		}
	default:
		agentUsage(e)
	}
	return nil
}

func agentUsage(e *Env) {
	e.Sess.Print("使い方:\n")
	e.Sess.Print("  agent            一覧\n")
	e.Sess.Print("  agent start <id> 起動\n")
	e.Sess.Print("  agent stop <id>  停止\n")
}

func agentList(e *Env) {
	list := e.Agents.ListAgents()
	if len(list) == 0 {
		e.Sess.Print("-- エージェントは登録されていません --\n")
		return
	}
	e.Sess.Printf("%-10s %s %s %s\n", "ID", session.PadRight("Handle", store.MaxHandle), session.PadRight("状態", 6), "Doing")
	for _, a := range list {
		state := "停止"
		if a.Running {
			state = "稼働"
		}
		e.Sess.Printf("%-10s %s %s %s\n", a.ID, session.PadRight(a.Handle, store.MaxHandle), session.PadRight(state, 6), a.Doing)
	}
}
