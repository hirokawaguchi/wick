package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
	"github.com/hirokawaguchi/wick/internal/warn"
)

func mailEnv(t *testing.T) (context.Context, store.Store, *Env) {
	t.Helper()
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	as := assets.Dir{Root: testenv.Root(t)}
	var out strings.Builder
	s := session.New("t", strings.NewReader(""), &out)
	s.User = u
	e := &Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st, Assets: as}
	return ctx, st, e
}

func TestPostmailReadDeleteKill(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User

	e.Sess = session.New("t", strings.NewReader("題A\n本文A\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	e.Args = "bob"
	if err := cmdPostmail(e); err != nil {
		t.Fatal(err)
	}
	inbox, err := st.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 1 || inbox[0].Subject != "題A" {
		t.Fatalf("inbox %+v err %v", inbox, err)
	}
	if !strings.Contains(inbox[0].Body, "本文A") {
		t.Fatalf("body %q", inbox[0].Body)
	}

	bob, err := st.GetUser(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("1\nq\n"), &out)
	e.Sess.User = bob
	e.Args = ""
	if err := cmdReadmail(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "本文A") || !strings.Contains(out.String(), "alice") {
		t.Fatalf("read %q", out.String())
	}
	n, err := st.CountUnreadMail(ctx, "bob")
	if err != nil || n != 0 {
		t.Fatalf("unread after read %d err %v", n, err)
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader("1\nq\n"), &out)
	e.Sess.User = bob
	if err := cmdDeletema(e); err != nil {
		t.Fatal(err)
	}
	inbox, err = st.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 0 {
		t.Fatalf("deleted %+v err %v", inbox, err)
	}

	e.Sess = session.New("t", strings.NewReader("題B\n未読\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	e.Args = "bob"
	if err := cmdPostmail(e); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	e.Sess = session.New("t", strings.NewReader("1\nq\n"), &out)
	e.Sess.User = alice
	if err := cmdKillmail(e); err != nil {
		t.Fatal(err)
	}
	inbox, err = st.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 0 {
		t.Fatalf("killed %+v err %v", inbox, err)
	}
}

// TestReadmailUnifiedKeys: 自由入力の場では i=一覧・q=抜ける に揃える。
func TestReadmailUnifiedKeys(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User

	e.Sess = session.New("t", strings.NewReader("題X\n本文X\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	e.Args = "bob"
	if err := cmdPostmail(e); err != nil {
		t.Fatal(err)
	}
	bob, err := st.GetUser(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("i\nq\n"), &out)
	e.Sess.User = bob
	e.Args = ""
	if err := cmdReadmail(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "i=一覧") {
		t.Fatalf("入場ガイドに i=一覧 が無い: %q", got)
	}
	// i で一覧が再表示される（受信箱ヘッダが 2 回以上）。
	if strings.Count(got, "受信箱") < 2 {
		t.Fatalf("i で一覧が再表示されるはず: %q", got)
	}
}

func TestMultiposGroupAndMbox(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User

	var out strings.Builder

	if err := st.SaveMailGroup(ctx, store.MailGroup{Owner: "alice", Name: "friends", Members: "bob"}); err != nil {
		t.Fatal(err)
	}
	e.Sess = session.New("t", strings.NewReader("同報\nhello all\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	e.Args = "@friends"
	if err := cmdMultipos(e); err != nil {
		t.Fatal(err)
	}
	inbox, err := st.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 1 || inbox[0].Subject != "同報" {
		t.Fatalf("multi %+v err %v", inbox, err)
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader("q\n"), &out)
	e.Sess.User = alice
	if err := cmdLookreco(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "同報") {
		t.Fatalf("lookreco %q", out.String())
	}

	out.Reset()
	e.Sess = session.New("t", strings.NewReader("off\n"), &out)
	e.Sess.User = alice
	e.Args = ""
	if err := cmdRegmbox(e); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetUser(ctx, "alice")
	if err != nil || u.MailSave {
		t.Fatalf("mbox %+v err %v", u, err)
	}
}

func TestReggroupAndPfile(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("friends\nbob sysop\n\n"), &out)
	e.Sess.User = alice
	if err := cmdReggroup(e); err != nil {
		t.Fatal(err)
	}
	g, err := st.GetMailGroup(ctx, "alice", "friends")
	if err != nil || g.Members != "bob sysop" {
		t.Fatalf("%+v err %v", g, err)
	}
}

func TestMailBodyLinkWarning(t *testing.T) {
	ctx, st, e := mailEnv(t)
	alice := e.Sess.User

	// 本文に URL を書いて送る
	e.Sess = session.New("t", strings.NewReader("題\n資料はこちら https://example.com/a.zip\n.\n"), new(strings.Builder))
	e.Sess.User = alice
	e.Args = "bob"
	if err := cmdPostmail(e); err != nil {
		t.Fatal(err)
	}
	// bob が読むと警告が出る
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader(""), &out)
	bob, err := st.GetUser(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	e.Sess.User = bob
	inbox, err := st.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox %+v %v", inbox, err)
	}
	if err := printMail(e, inbox[0]); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "https://example.com/a.zip") || !strings.Contains(got, warn.External) {
		t.Fatalf("mail warning: %q", got)
	}
}
