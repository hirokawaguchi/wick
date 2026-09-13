package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

// safeBuf は複数ゴルーチンから安全に書ける文字列バッファ（テスト用）。
type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// repoData は data/ の場所を探す（テストの作業ディレクトリ基準）。
func repoData(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		root := filepath.Join(dir, "data")
		if _, err := os.Stat(filepath.Join(root, "etc", "COMMAND.TXT")); err == nil {
			return root
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	t.Fatal("data/etc/COMMAND.TXT not found")
	return ""
}

// TestPilotGreeterActsAcrossChannels は、偽頭脳 greeter が人間と同じコマンドループで
// メール送信・ノート作成まで実際に行い、who に AI として現れ、停止で離脱することを確かめる。
func TestPilotGreeterActsAcrossChannels(t *testing.T) {
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
	if err := st.SeedAgents(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}

	board, err := st.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	notesBefore, _ := st.ListNotes(ctx, board.ID)

	h := host.New(10)
	mgr := NewManager(h, st, tbl, as)
	mgr.Register(Spec{
		ID:        "scout",
		Behavior:  "greeter",
		Heartbeat: 40 * time.Millisecond,
		Budget:    -1,
		Target:    "sysop",
		Board:     "junk.test",
	})

	if err := mgr.StartAgent("scout"); err != nil {
		t.Fatalf("StartAgent: %v", err)
	}

	// 起動直後、who に AI として現れる。
	if !whoHasAgent(h, "scout") {
		// 起動処理はゴルーチンなので少しだけ待つ
		waitFor(t, 500*time.Millisecond, func() bool { return whoHasAgent(h, "scout") })
	}

	// メール（sysop 受信箱）とノート（junk.test 新規ベース）が実際にできること。
	waitFor(t, 3*time.Second, func() bool {
		inbox, _ := st.ListInbox(ctx, "sysop")
		notesNow, _ := st.ListNotes(ctx, board.ID)
		return len(inbox) >= 1 && len(notesNow) > len(notesBefore)
	})

	inbox, _ := st.ListInbox(ctx, "sysop")
	if len(inbox) < 1 {
		t.Fatalf("sysop 宛メールが作られていない: %d", len(inbox))
	}
	notesNow, _ := st.ListNotes(ctx, board.ID)
	if len(notesNow) <= len(notesBefore) {
		t.Fatalf("ノートが増えていない: before=%d now=%d", len(notesBefore), len(notesNow))
	}
	// 新規ベースの著者が scout であること。
	foundScout := false
	for _, n := range notesNow {
		if n.Author == "scout" {
			foundScout = true
		}
	}
	if !foundScout {
		t.Fatal("scout が書いたベースノートが見つからない")
	}

	// list に稼働として出ること。
	found := false
	for _, a := range mgr.ListAgents() {
		if a.ID == "scout" && a.Running {
			found = true
		}
	}
	if !found {
		t.Fatal("ListAgents に稼働中の scout が無い")
	}

	// 停止すると who から消える（Close→EOF でループ終了）。
	if !mgr.StopAgent("scout") {
		t.Fatal("StopAgent が false")
	}
	waitFor(t, 2*time.Second, func() bool { return !whoHasAgent(h, "scout") })
	if whoHasAgent(h, "scout") {
		t.Fatal("停止後も who に scout が残っている")
	}
}

// TestPilotConversantReactsInChat は、反応型 conversant が画面を観測して
// 他者のチャット発言に返答する（観測→判断ループ）ことを確かめる。
func TestPilotConversantReactsInChat(t *testing.T) {
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
	if err := st.SeedAgents(ctx, "wick"); err != nil {
		t.Fatal(err)
	}

	h := host.New(10)
	mgr := NewManager(h, st, tbl, as)
	mgr.Register(Spec{
		ID:        "critic",
		Behavior:  "conversant",
		Heartbeat: 30 * time.Millisecond,
		Budget:    -1,
		Room:      1,
	})
	if err := mgr.StartAgent("critic"); err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	defer mgr.StopAgent("critic")

	// エージェントが chat 1 に入るまで待つ。
	waitFor(t, 2*time.Second, func() bool {
		for _, p := range h.Who() {
			if p.ID == "critic" && p.Doing == "CHAT1" {
				return true
			}
		}
		return false
	})

	// 人間 alice を同じ部屋に入れ、発言する。
	var aliceOut safeBuf
	alice := session.New("h", strings.NewReader(""), &aliceOut)
	alice.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen}
	if !h.TryEnter(alice) {
		t.Fatal("alice enter")
	}
	if _, err := h.JoinChat(1, alice); err != nil {
		t.Fatalf("alice join: %v", err)
	}
	if err := h.SayChat(1, alice, "critic いる？"); err != nil {
		t.Fatalf("say: %v", err)
	}

	// エージェントが観測して返答する → alice に届く。
	got := ""
	waitFor(t, 2*time.Second, func() bool {
		alice.DrainNotices()
		got = aliceOut.String()
		return strings.Contains(got, "critic>")
	})
	if !strings.Contains(got, "critic>") {
		t.Fatalf("conversant が反応していない: %q", got)
	}
	if !strings.Contains(got, "alice") {
		t.Fatalf("反応が相手(alice)に言及していない: %q", got)
	}
}

// TestPilotWorkerClaimsJob は、worker が sys.jobs の未着手依頼を観測し、
// 人間と同じコマンド経路（open→w）でレスして引き受けることを確かめる（UC11）。
func TestPilotWorkerClaimsJob(t *testing.T) {
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
	if err := st.SeedAgents(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}

	jobs, err := st.GetBoard(ctx, "sys.jobs")
	if err != nil {
		t.Fatal(err)
	}

	h := host.New(10)
	mgr := NewManager(h, st, tbl, as)
	mgr.Register(Spec{ID: "scout", Behavior: "worker", Heartbeat: 30 * time.Millisecond, Budget: -1})
	if err := mgr.StartAgent("scout"); err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	defer mgr.StopAgent("scout")

	// シード済みの依頼（ノート#1）に scout がレスする。
	waitFor(t, 3*time.Second, func() bool { return jobHasResponderFrom(t, st, jobs.ID, 1, "scout") })
	if !jobHasResponderFrom(t, st, jobs.ID, 1, "scout") {
		t.Fatal("scout がシード依頼を引き受けていない")
	}
	firstResp := countResponders(t, st, jobs.ID, 1, "scout")

	// 追加の依頼を立てると、それも引き受ける。
	now := time.Now()
	n2, err := st.CreateNote(ctx, store.Note{
		BoardID: jobs.ID, Title: "調査依頼: 2件目", Author: "sysop", Handle: "Sysop",
		PostTime: now, LastUpdate: now, Body: "もう一件お願いします。\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return jobHasResponderFrom(t, st, jobs.ID, n2.Num, "scout") })
	if !jobHasResponderFrom(t, st, jobs.ID, n2.Num, "scout") {
		t.Fatal("scout が追加依頼を引き受けていない")
	}

	// 同じ依頼を二重に埋め尽くさない（#1 への scout レスは 1 つのまま）。
	if got := countResponders(t, st, jobs.ID, 1, "scout"); got != firstResp {
		t.Fatalf("同じ依頼へ多重レス: %d (want %d)", got, firstResp)
	}
}

func noteByNum(t *testing.T, st store.Store, boardID int64, num int) store.Note {
	t.Helper()
	notes, err := st.ListNotes(context.Background(), boardID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Num == num {
			return n
		}
	}
	return store.Note{}
}

func jobHasResponderFrom(t *testing.T, st store.Store, boardID int64, num int, author string) bool {
	return countResponders(t, st, boardID, num, author) > 0
}

func countResponders(t *testing.T, st store.Store, boardID int64, num int, author string) int {
	t.Helper()
	n := noteByNum(t, st, boardID, num)
	if n.ID == 0 {
		return 0
	}
	reps, err := st.ListResponses(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := 0
	for _, r := range reps {
		if strings.EqualFold(r.Author, author) {
			c++
		}
	}
	return c
}

func whoHasAgent(h *host.Host, id string) bool {
	for _, p := range h.Who() {
		if p.ID == id && p.Agent {
			return true
		}
	}
	return false
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
