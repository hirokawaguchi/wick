package agent

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/store"
)

// janitor（用務員）は sys.jobs を定期巡回し、決着した依頼を自動でクローズする。
// これはエージェント（人間コマンド経路）ではなく、局側の保守処理。
// 依頼の著者でなくても閉じられるよう、Store 直接更新で MsgClosed を立てる。

const (
	janitorEvery   = 30 * time.Second // 巡回間隔
	jobSettleAfter = 2 * time.Minute  // 最終更新からこれだけ静かなら「落ち着いた」
	jobCloseQuorum = 2                // これだけ別々の担当がレスしたら決着とみなす（静穏条件つき）
	jobBoardName   = "sys.jobs"
)

// jobConclusionMarks は「結論が書かれた」と判断するレス本文の目印。
var jobConclusionMarks = []string{
	"結論", "【結論】", "まとめ:", "まとめ：", "完了しました", "以上で完了", "クローズします",
	"conclusion", "summary:", "completed", "that's all", "closing",
}

// StartJanitor は保守巡回を起動する（boot 時に main から呼ぶ）。
// host の shutdown で止まる。テストでは呼ばず sweepJobs を直接叩く。
func (m *Manager) StartJanitor() {
	go func() {
		t := time.NewTicker(m.janitorEvery)
		defer t.Stop()
		shutdown := m.host.ShutdownC()
		for {
			select {
			case <-shutdown:
				return
			case <-t.C:
				m.sweepJobs(time.Now())
			}
		}
	}()
}

// sweepJobs は sys.jobs を 1 度走査し、決着した依頼をクローズする。
func (m *Manager) sweepJobs(now time.Time) {
	ctx := context.Background()
	b, err := m.store.GetBoard(ctx, jobBoardName)
	if err != nil {
		return
	}
	notes, err := m.store.ListNotes(ctx, b.ID)
	if err != nil {
		return
	}
	for _, n := range notes {
		reps, _ := m.store.ListResponses(ctx, n.ID)
		if !jobConcluded(n, reps, now, m.jobSettle, m.jobQuorum) {
			continue
		}
		n.Flags |= store.MsgClosed
		if err := m.store.UpdateNote(ctx, n); err == nil {
			log.Printf("agent janitor: closed %s #%d %q", jobBoardName, n.Num, n.Title)
		}
	}
}

// jobConcluded は依頼が決着したかを判定する（純関数。テストしやすいよう外に出す）。
//   - 結論マーク付きのレスが 1 つでもあれば決着。
//   - もしくは quorum 人以上の別々の担当がレスし、かつ settle 以上静かなら決着。
func jobConcluded(n store.Note, reps []store.Response, now time.Time, settle time.Duration, quorum int) bool {
	if n.Flags&store.MsgClosed != 0 || n.Flags&store.MsgDeleted != 0 {
		return false
	}
	who := map[string]bool{}
	for _, r := range reps {
		if r.Flags&store.MsgDeleted != 0 {
			continue
		}
		if hasConclusionMark(r.Body) {
			return true
		}
		if a := strings.ToLower(strings.TrimSpace(r.Author)); a != "" {
			who[a] = true
		}
	}
	if quorum > 0 && len(who) >= quorum && settle > 0 && now.Sub(n.LastUpdate) >= settle {
		return true
	}
	return false
}

func hasConclusionMark(body string) bool {
	low := strings.ToLower(body)
	for _, mk := range jobConclusionMarks {
		if strings.Contains(body, mk) || strings.Contains(low, strings.ToLower(mk)) {
			return true
		}
	}
	return false
}
