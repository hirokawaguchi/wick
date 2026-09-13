package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func TestPrivateAndScanlist(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	h := host.New(10)
	var out strings.Builder
	s := session.New("t", strings.NewReader("1\n山田太郎\n\n"), &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	e := &Env{Ctx: ctx, Sess: s, Host: h, Store: st}
	if err := cmdPrivate(e); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(ctx, "alice")
	if err != nil || u.RealName != "山田太郎" {
		t.Fatalf("private %+v err %v out=%q", u, err, out.String())
	}

	out.Reset()
	s = session.New("t", strings.NewReader("junk.test\n\n"), &out)
	s.User = store.User{ID: "alice", Flags: acl.FlagGen, TLimit: 65535}
	e.Sess = s
	if err := cmdScanlist(e); err != nil {
		t.Fatal(err)
	}
	u, err = st.GetUser(ctx, "alice")
	if err != nil || u.ScanList != "junk.test" {
		t.Fatalf("scan %q err %v", u.ScanList, err)
	}

	out.Reset()
	s = session.New("t", strings.NewReader("alice / Wick\n.\n"), &out)
	s.User = store.User{ID: "alice", Flags: acl.FlagGen, TLimit: 65535}
	e.Sess = s
	if err := cmdRegsign(e); err != nil {
		t.Fatal(err)
	}
	u, err = st.GetUser(ctx, "alice")
	if err != nil || u.Autosign != "alice / Wick" {
		t.Fatalf("sign %q err %v", u.Autosign, err)
	}
}
