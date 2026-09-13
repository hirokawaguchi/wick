package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
)

func newSignupEnv(t *testing.T, input string) (*Env, store.Store, *strings.Builder) {
	t.Helper()
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedGuest(ctx, "guest"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader(input), &out)
	s.User = store.User{ID: "guest", Handle: "Guest", Flags: acl.FlagGst, TLimit: 10}
	e := &Env{Ctx: ctx, Sess: s, Store: st, Assets: assets.Dir{Root: testenv.Root(t)}}
	return e, st, &out
}

func TestSignupCreatesProbation(t *testing.T) {
	// ID はユーザ指定できず自動採番。入力はハンドル→パスワード×2 のみ。
	e, st, out := newSignupEnv(t, "Newbie\nsecret\nsecret\n")
	if err := cmdSignup(e); err != nil {
		t.Fatal(err)
	}
	// 最初の会員IDは prd00001。
	u, err := st.Authenticate(e.Ctx, "prd00001", "secret")
	if err != nil {
		t.Fatalf("auth new user: %v (out=%q)", err, out.String())
	}
	if u.Flags != acl.FlagPro {
		t.Fatalf("new user flags %08x, want probation", u.Flags)
	}
	if u.Handle != "Newbie" {
		t.Fatalf("handle %q", u.Handle)
	}
	if !strings.Contains(out.String(), "prd00001") {
		t.Fatalf("assigned ID not shown: %q", out.String())
	}
}

func TestSignupAssignsSequentialIDs(t *testing.T) {
	e, st, _ := newSignupEnv(t, "Alice\npass1\npass1\n")
	if err := cmdSignup(e); err != nil {
		t.Fatal(err)
	}
	// 2 人目は同じ store で続けて登録（ID は +1 されるはず）。
	var out2 strings.Builder
	s2 := session.New("t", strings.NewReader("Bob\npass2\npass2\n"), &out2)
	s2.User = store.User{ID: "guest", Handle: "Guest", Flags: acl.FlagGst, TLimit: 10}
	e2 := &Env{Ctx: e.Ctx, Sess: s2, Store: st, Assets: e.Assets}
	if err := cmdSignup(e2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetUser(e.Ctx, "prd00001"); err != nil {
		t.Fatalf("first member prd00001 missing: %v", err)
	}
	if u, err := st.GetUser(e.Ctx, "prd00002"); err != nil {
		t.Fatalf("second member prd00002 missing: %v", err)
	} else if u.Handle != "Bob" {
		t.Fatalf("prd00002 handle %q, want Bob", u.Handle)
	}
}

func TestUsereditPromotes(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.CreateUser(ctx, store.User{ID: "newbie", Handle: "N", Flags: acl.FlagPro, TLimit: 30}, "p"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader("1\ngen\n2\n60\n\n"), &out)
	s.User = store.User{ID: "sysop", Handle: "Sysop", Flags: acl.FlagSys, TLimit: 65535}
	e := &Env{Ctx: ctx, Sess: s, Store: st, Assets: assets.Dir{Root: testenv.Root(t)}}
	e.Args = "newbie"
	if err := cmdUseredit(e); err != nil {
		t.Fatal(err)
	}
	u, _ := st.GetUser(ctx, "newbie")
	if u.Flags != acl.FlagGen {
		t.Fatalf("flags %08x, want gen", u.Flags)
	}
	if u.TLimit != 60 {
		t.Fatalf("tlimit %d", u.TLimit)
	}
}
