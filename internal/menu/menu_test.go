package menu

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
)

func loadAssets(t *testing.T) (assets.Dir, *acl.Table) {
	t.Helper()
	root := testenv.Root(t)
	tbl, err := acl.Load(filepath.Join(root, "etc"))
	if err != nil {
		t.Fatal(err)
	}
	return assets.Dir{Root: root}, tbl
}

func TestNumberAndAlias(t *testing.T) {
	as, tbl := loadAssets(t)
	// [5] others に入って戻り、alias w で who、[6] off で終了
	in := strings.NewReader("5\r\n.\r\nw\r\n6\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	h := host.New(10)
	h.TryEnter(s)
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   h,
		ACL:    tbl,
		Assets: as,
	}
	err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	if !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v out=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("who missing: %q", out.String())
	}
}

func TestHelpAndUnknown(t *testing.T) {
	as, tbl := loadAssets(t)
	in := strings.NewReader("who -?\r\nnosuch\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   host.New(10),
		ACL:    tbl,
		Assets: as,
	}
	err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	if !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ログイン中") {
		t.Fatalf("usage missing: %q", got)
	}
	if !strings.Contains(got, "unknown command") {
		t.Fatalf("unknown missing: %q", got)
	}
}

// TestHelpListAndPerCommand は簡易ヘルプ（?）が説明つき一覧を出すことと、
// `? 名前` が個別コマンドの使い方（-? と同じ）を出すことを検証する。
func TestHelpListAndPerCommand(t *testing.T) {
	as, tbl := loadAssets(t)
	in := strings.NewReader("?\r\n? who\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   host.New(10),
		ACL:    tbl,
		Assets: as,
	}
	if err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1); !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v", err)
	}
	got := out.String()
	// 一覧に説明が添えられている（羅列ではない）。
	if !strings.Contains(got, "コマンド一覧") {
		t.Fatalf("help header missing: %q", got)
	}
	if !strings.Contains(got, "ログイン中ユーザー一覧") {
		t.Fatalf("who summary missing from list: %q", got)
	}
	// `? who` は who の詳しい使い方。
	if !strings.Contains(got, "ログイン中のユーザー一覧を表示") {
		t.Fatalf("`? who` usage missing: %q", got)
	}
}

// TestLaterUnimplemented は「権限あり・ハンドラ無し」→「まだ実装されていません」を検証する。
// COMMAND.TXT には未実装コマンドが無くなったので、専用の ACL 表で確かめる。
func TestLaterUnimplemented(t *testing.T) {
	as, _ := loadAssets(t)
	tbl := &acl.Table{Commands: []acl.Command{
		{Name: "faketodo", Allow: acl.FlagGen},
		{Name: "off", Allow: acl.FlagGen},
	}}
	in := strings.NewReader("faketodo\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   host.New(10),
		ACL:    tbl,
		Assets: as,
	}
	err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	if !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v", err)
	}
	if !strings.Contains(out.String(), "まだ実装されていません") {
		t.Fatalf("later missing: %q", out.String())
	}
}

func TestMenuFullwidthCommand(t *testing.T) {
	as, tbl := loadAssets(t)
	// 全角 ｗ で who、全角 ６（[6] off）で終了
	in := strings.NewReader("ｗ\r\n６\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	h := host.New(10)
	h.TryEnter(s)
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   h,
		ACL:    tbl,
		Assets: as,
	}
	err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	if !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v out=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("who missing: %q", out.String())
	}
}

func TestEmptyEnterShowsMenu(t *testing.T) {
	as, tbl := loadAssets(t)
	in := strings.NewReader("\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	env := &command.Env{
		Ctx:    context.Background(),
		Sess:   s,
		Host:   host.New(10),
		ACL:    tbl,
		Assets: as,
	}
	err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1)
	if !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v out=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "ノートファイル") {
		t.Fatalf("menu missing on enter: %q", out.String())
	}
}

func TestNotesHierarchy(t *testing.T) {
	as, tbl := loadAssets(t)
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader("2\r\n1\r\n1\r\nq\r\n.\r\n.\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	h := host.New(10)
	h.TryEnter(s)
	env := &command.Env{
		Ctx: ctx, Sess: s, Host: h, Store: st, ACL: tbl, Assets: as,
	}
	if err := (&Engine{Reg: command.NewRegistry()}).Enter(env, "MAIN", "", 1); !errors.Is(err, command.ErrLogOff) {
		t.Fatalf("err %v out=%q", err, out.String())
	}
	got := out.String()
	// カテゴリは日本語名 <id.*>、コマンドは (…) で区別する。件数は出さない。
	if !strings.Contains(got, "なんでもボード") || !strings.Contains(got, "<junk.*>") {
		t.Fatalf("category (jp name + <id.*>) missing: %q", got)
	}
	if !strings.Contains(got, "(new)") || !strings.Contains(got, "(bbslist)") {
		t.Fatalf("command items missing: %q", got)
	}
	// サブメニューはフルIDを <…> で添える。
	if !strings.Contains(got, "<junk.sandbox>") {
		t.Fatalf("board list missing: %q", got)
	}
	if !strings.Contains(got, "[ INDEX ]") {
		t.Fatalf("did not open board: %q", got)
	}
}

func TestMenuLine(t *testing.T) {
	as, tbl := loadAssets(t)
	env := &command.Env{ACL: tbl, Assets: as}
	e := &Engine{}
	line, ok := e.menuLine(env, "MAIN", 3)
	if !ok || line != "setup" {
		t.Fatalf("line %q ok=%v", line, ok)
	}
	if !e.hasMenu(env, "setup") {
		t.Fatal("setup menu")
	}
}
