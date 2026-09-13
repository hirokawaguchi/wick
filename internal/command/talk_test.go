package command

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func talkEnv(t *testing.T, id string, in io.Reader, out io.Writer, st store.Store, h *host.Host) *Env {
	t.Helper()
	s := session.New("t", in, out)
	s.User = store.User{ID: id, Handle: id, Flags: acl.FlagGen, TLimit: 65535}
	if !h.TryEnter(s) {
		t.Fatal("enter")
	}
	t.Cleanup(func() { h.Leave(id) })
	return &Env{Ctx: context.Background(), Sess: s, Host: h, Store: st}
}

func TestTalkUnreadScan(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedTalkRooms(ctx); err != nil {
		t.Fatal(err)
	}
	ln, err := st.CreateTalkLine(ctx, store.TalkLine{
		Room: 2, Author: "alice", Handle: "Alice", Body: "line conference",
	})
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	e := talkEnv(t, "bob", strings.NewReader(""), &out, st, host.New(10))
	e.Sess.Sequencer = ln.PostTime.Add(-time.Second)
	e.Args = "-n"
	if err := cmdTalk(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "line conference") || !strings.Contains(got, "talk 2:") {
		t.Fatalf("unread missing: %q", got)
	}
	if !e.Sess.Sequencer.Equal(ln.PostTime) && !e.Sess.Sequencer.After(ln.PostTime.Add(-time.Second)) {
		t.Fatalf("sequencer not advanced: %v", e.Sess.Sequencer)
	}

	out.Reset()
	e.Args = "-n"
	if err := cmdTalk(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "未読なし") {
		t.Fatalf("second scan: %q", out.String())
	}
}

func TestTalkPostAndLive(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedTalkRooms(ctx); err != nil {
		t.Fatal(err)
	}
	h := host.New(10)
	var outA, outB strings.Builder
	alice := talkEnv(t, "alice", strings.NewReader("hello from alice\n.\n"), &outA, st, h)
	bob := talkEnv(t, "bob", strings.NewReader(""), &outB, st, h)
	if _, err := h.JoinTalk(1, store.TalkOpen, bob.Sess); err != nil {
		t.Fatal(err)
	}
	alice.Args = "1"
	if err := cmdTalk(alice); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outA.String(), "入室") {
		t.Fatalf("alice enter: %q", outA.String())
	}
	bob.Sess.DrainNotices()
	if !strings.Contains(outB.String(), "hello from alice") {
		t.Fatalf("live missing: %q", outB.String())
	}
	lines, err := st.ListTalkLines(ctx, 1, 1)
	if err != nil || len(lines) != 1 {
		t.Fatalf("persist %+v err %v", lines, err)
	}
}

// TestTalkUnifiedKeys: 発言の場では /i=一覧・/q=退出 に揃える。
func TestTalkUnifiedKeys(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedTalkRooms(ctx); err != nil {
		t.Fatal(err)
	}
	h := host.New(10)
	var out strings.Builder
	alice := talkEnv(t, "alice", strings.NewReader("/i\n/q\n"), &out, st, h)
	alice.Args = "1"
	if err := cmdTalk(alice); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "/i=一覧") || !strings.Contains(got, "/q=退出") {
		t.Fatalf("入室ガイドに統一キーが無い: %q", got)
	}
	if !strings.Contains(got, "-- 退室 --") {
		t.Fatalf("/q で退室するはず: %q", got)
	}
}

func TestTalkLockedNeedsSeat(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedTalkRooms(ctx); err != nil {
		t.Fatal(err)
	}
	room, err := st.GetTalkRoom(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	room.Status = store.TalkLocked
	if err := st.UpdateTalkRoom(ctx, room); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	e := talkEnv(t, "bob", strings.NewReader("secret\n.\n"), &out, st, host.New(10))
	e.Args = "3"
	if err := cmdTalk(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ノック") {
		t.Fatalf("role missing: %q", got)
	}
	if !strings.Contains(got, "座席がありません") {
		t.Fatalf("write should fail: %q", got)
	}
	lines, err := st.ListTalkLines(ctx, 3, 1)
	if err != nil || len(lines) != 0 {
		t.Fatalf("should not persist %+v err %v", lines, err)
	}
}

func TestRoomlist(t *testing.T) {
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var out strings.Builder
	e := talkEnv(t, "alice", strings.NewReader(""), &out, st, host.New(10))
	if err := cmdRoomlist(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Room 1") || !strings.Contains(out.String(), " Op ") {
		t.Fatalf("list: %q", out.String())
	}
}

func TestTalkEmptyEnterLists(t *testing.T) {
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var out strings.Builder
	e := talkEnv(t, "alice", strings.NewReader("\n.\n"), &out, st, host.New(10))
	if err := cmdTalk(e); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), "Room 1"); n < 2 {
		t.Fatalf("list should reprint, got %d: %q", n, out.String())
	}
}

func TestChatEmptyEnterLists(t *testing.T) {
	var out strings.Builder
	e := talkEnv(t, "alice", strings.NewReader("\n.\n"), &out, nil, host.New(10))
	if err := cmdChat(e); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), "Room 1"); n < 2 {
		t.Fatalf("chat list should reprint, got %d: %q", n, out.String())
	}
}
