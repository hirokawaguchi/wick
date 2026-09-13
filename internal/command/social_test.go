package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
)

func TestChatEchoToggle(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var out strings.Builder
	s := session.New("t", strings.NewReader("hi\n/e\nbye\n.\n"), &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	e := &Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st, Assets: assets.Dir{Root: testenv.Root(t)}}
	e.Args = "1"
	if err := cmdChat(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	// 部屋内プロンプトは "alice> "。自分の行はプロンプト＋ローカルエコーで
	// 1 回だけ出る（既定 echoback OFF なので反射は無い）。
	if n := strings.Count(got, "alice> hi"); n != 1 {
		t.Fatalf("既定は自分の行 1 回のはず: %d 回 / %q", n, got)
	}
	// 既定 OFF なので /e で ON になる。
	if !strings.Contains(got, "echoback on") {
		t.Fatalf("toggle message missing: %q", got)
	}
	// echoback ON のあいだは、ローカルエコー＋反射で 2 回出る。
	if n := strings.Count(got, "alice> bye"); n != 2 {
		t.Fatalf("echoback on で 2 回のはず: %d 回 / %q", n, got)
	}
}

// TestChatUnifiedKeys: 発言の場では /i=一覧・/q=退出 に揃える。
func TestChatUnifiedKeys(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var out strings.Builder
	s := session.New("t", strings.NewReader("/i\n/q\n"), &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	e := &Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st, Assets: assets.Dir{Root: testenv.Root(t)}}
	e.Args = "1"
	if err := cmdChat(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	// /i で部屋一覧が出る（printRooms の見出し）。
	if !strings.Contains(got, "No  N  Title") {
		t.Fatalf("/i で一覧が出るはず: %q", got)
	}
	// /q で退室できる。
	if !strings.Contains(got, "-- 退室 --") {
		t.Fatalf("/q で退室するはず: %q", got)
	}
	// 入室ガイドに統一キーが出る。
	if !strings.Contains(got, "/i=一覧") || !strings.Contains(got, "/q=退出") {
		t.Fatalf("入室ガイドに統一キーが無い: %q", got)
	}
}
