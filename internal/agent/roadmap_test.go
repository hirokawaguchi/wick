package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/store"
)

// TestWorkerReclaim は、他者が引き受けたまま滞留した依頼を、時間経過後に
// 別の worker が引き取る（＝再割り当てする）ことを確かめる。
func TestWorkerReclaim(t *testing.T) {
	b := &workerBrain{selfID: "critic", acted: map[int]bool{}, reclaimAfter: 60 * time.Second}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)

	// scout が 10 秒前に引き受けたばかり（新鮮）→ 触らない。
	fresh := Observation{Doing: "MAIN", Now: now, Jobs: []Job{{
		Num: 1, Title: "調査", Author: "sysop",
		Responders: []string{"scout"}, Updated: now.Add(-10 * time.Second),
	}}}
	if s, _ := b.Next(fresh); s != "" {
		t.Fatalf("新鮮な依頼に手を出した: %q", s)
	}

	// 90 秒滞留 → critic が引き取る。
	stale := Observation{Doing: "MAIN", Now: now, Jobs: []Job{{
		Num: 1, Title: "調査", Author: "sysop",
		Responders: []string{"scout"}, Updated: now.Add(-90 * time.Second),
	}}}
	s, _ := b.Next(stale)
	if !strings.Contains(s, "open sys.jobs") || !strings.Contains(s, "1\nw\n") {
		t.Fatalf("再割り当てのコマンドになっていない: %q", s)
	}
	if !strings.Contains(s, "引き取ります") {
		t.Fatalf("引き取りのレス文でない: %q", s)
	}

	// 引き取った後は二度と同じ依頼に手を出さない。
	if s2, _ := b.Next(stale); s2 != "" {
		t.Fatalf("引き取り後に再度アクション: %q", s2)
	}
}

// TestWorkerSkipsOwnAndClaimsUnowned は、自分の応答済みは飛ばし、未着手は引き受けることを確かめる。
func TestWorkerSkipsOwnAndClaimsUnowned(t *testing.T) {
	b := &workerBrain{selfID: "scout", acted: map[int]bool{}, reclaimAfter: 60 * time.Second}
	now := time.Now()
	obs := Observation{Doing: "MAIN", Now: now, Jobs: []Job{
		{Num: 1, Title: "自分済み", Responders: []string{"scout"}, Updated: now},
		{Num: 2, Title: "未着手", Responders: nil},
	}}
	s, _ := b.Next(obs)
	if !strings.Contains(s, "2\nw\n") {
		t.Fatalf("未着手(#2)を引き受けていない: %q", s)
	}
}

// TestPosterTimeTrigger は、間隔を過ぎたら時刻起点でノートを立てることを確かめる。
func TestPosterTimeTrigger(t *testing.T) {
	b := newPosterBrain(Spec{Board: "junk.test", Interval: time.Minute}, "poet", "Poet")
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)

	// 起動直後は基準時刻をセットするだけ（投稿しない）。
	if s, _ := b.Next(Observation{Doing: "MAIN", Now: t0}); s != "" {
		t.Fatalf("起動直後に投稿した: %q", s)
	}
	// 間隔内は投稿しない。
	if s, _ := b.Next(Observation{Doing: "MAIN", Now: t0.Add(30 * time.Second)}); s != "" {
		t.Fatalf("間隔内で投稿した: %q", s)
	}
	// 間隔を過ぎたら open→w のベースノート投稿。
	s, _ := b.Next(Observation{Doing: "MAIN", Now: t0.Add(2 * time.Minute)})
	if !strings.HasPrefix(s, "open junk.test\nw") || !strings.HasSuffix(s, "\n.\nq") {
		t.Fatalf("時刻起点のノート投稿になっていない: %q", s)
	}
}

// TestPosterInterestTrigger は、画面に興味キーワードが出たらその話題で立てることを確かめる。
func TestPosterInterestTrigger(t *testing.T) {
	b := newPosterBrain(Spec{Board: "junk.test", Interval: time.Minute, Interests: []string{"月", "詩"}}, "poet", "Poet")
	t0 := time.Now()
	b.Next(Observation{Doing: "MAIN", Now: t0}) // 基準時刻セット

	// cooldown を過ぎ、画面に「詩」がある → その話題でノート。
	s, _ := b.Next(Observation{Doing: "MAIN", Now: t0.Add(2 * time.Minute), Screen: "alice> 詩を書きたい"})
	if !strings.Contains(s, "詩") || !strings.HasPrefix(s, "open junk.test\nw") {
		t.Fatalf("興味起点のノートになっていない: %q", s)
	}
}

// TestJobConcluded は自動クローズ判定（純関数）を確かめる。
func TestJobConcluded(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	base := store.Note{Num: 1, LastUpdate: now}

	// 結論マーク付きレスがあれば決着。
	withMark := []store.Response{{Author: "scout", Body: "調べました"}, {Author: "critic", Body: "【結論】十分です"}}
	if !jobConcluded(base, withMark, now, 2*time.Minute, 2) {
		t.Fatal("結論マークで決着しない")
	}
	// quorum 未満は決着しない（静穏でも）。
	one := []store.Response{{Author: "scout", Body: "作業中"}}
	old := store.Note{Num: 1, LastUpdate: now.Add(-10 * time.Minute)}
	if jobConcluded(old, one, now, 2*time.Minute, 2) {
		t.Fatal("1 人だけで決着してしまう")
	}
	// quorum 到達＋静穏なら決着。
	two := []store.Response{{Author: "scout", Body: "収集した"}, {Author: "critic", Body: "裏取り済み"}}
	if !jobConcluded(old, two, now, 2*time.Minute, 2) {
		t.Fatal("quorum＋静穏で決着しない")
	}
	// quorum 到達でも直近更新なら決着しない。
	if jobConcluded(base, two, now, 2*time.Minute, 2) {
		t.Fatal("直近更新なのに決着してしまう")
	}
	// すでにクローズ済みは対象外。
	closed := store.Note{Num: 1, LastUpdate: old.LastUpdate, Flags: store.MsgClosed}
	if jobConcluded(closed, two, now, 2*time.Minute, 2) {
		t.Fatal("クローズ済みを再度決着扱い")
	}
}

// TestSweepJobsClosesConcluded は、janitor が sys.jobs の決着ジョブを
// 実際に Store 上でクローズ（MsgClosed）することを確かめる。
func TestSweepJobsClosesConcluded(t *testing.T) {
	ctx := context.Background()
	root := repoData(t)
	tbl, err := acl.Load(filepath.Join(root, "etc"))
	if err != nil {
		t.Fatal(err)
	}
	as := assets.Dir{Root: root}
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}

	jobs, err := st.GetBoard(ctx, "sys.jobs")
	if err != nil {
		t.Fatal(err)
	}
	// シード依頼(#1)に結論マーク付きのレスを付ける。
	n1 := noteByNum(t, st, jobs.ID, 1)
	if n1.ID == 0 {
		t.Fatal("シード依頼が無い")
	}
	if _, err := st.CreateResponse(ctx, store.Response{
		NoteID: n1.ID, Author: "critic", Handle: "Critic", Body: "【結論】事例をまとめました。以上です。\n",
	}); err != nil {
		t.Fatal(err)
	}

	h := host.New(10)
	mgr := NewManager(h, st, tbl, as)
	mgr.sweepJobs(time.Now())

	got := noteByNum(t, st, jobs.ID, 1)
	if got.Flags&store.MsgClosed == 0 {
		t.Fatalf("決着ジョブがクローズされていない: flags=%d", got.Flags)
	}
}
