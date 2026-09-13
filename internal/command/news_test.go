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

func newsEnv(t *testing.T, flags uint32) (context.Context, store.Store, *Env) {
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
	if err := st.SeedNewsGroups(ctx); err != nil {
		t.Fatal(err)
	}
	root := testenv.Root(t)
	tbl, err := acl.Load(filepath.Join(root, "etc"))
	if err != nil {
		t.Fatal(err)
	}
	id := "alice"
	if flags&acl.FlagSys != 0 {
		id = "sysop"
	}
	u, err := st.GetUser(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	u.Flags = flags
	var out strings.Builder
	s := session.New("t", strings.NewReader(""), &out)
	s.User = u
	e := &Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st, ACL: tbl, Assets: assets.Dir{Root: root}}
	return ctx, st, e
}

func TestPostnewsDeniedForGen(t *testing.T) {
	_, _, e := newsEnv(t, acl.FlagGen)
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User.ID = "alice"
	e.Sess.User.Flags = acl.FlagGen
	err := cmdPostnews(e)
	if err != ErrDenied {
		t.Fatalf("err %v out=%q", err, out.String())
	}
}

func TestPostnewsAndReadnews(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	sys := e.Sess.User
	e.Sess = session.New("t", strings.NewReader("hello\n本文です\n.\n"), new(strings.Builder))
	e.Sess.User = sys
	e.Args = "local"
	if err := cmdPostnews(e); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountUnreadNews(ctx, "alice")
	if err != nil || n != 1 {
		t.Fatalf("unread %d err %v", n, err)
	}

	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("\nq\n"), &out)
	e.Sess.User = alice
	e.Args = ""
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "本文です") || !strings.Contains(out.String(), "hello") {
		t.Fatalf("read %q", out.String())
	}
	n, err = st.CountUnreadNews(ctx, "alice")
	if err != nil || n != 0 {
		t.Fatalf("after q %d err %v", n, err)
	}

	e.Sess = session.New("t", strings.NewReader("skip me\nsecond\n.\n"), new(strings.Builder))
	e.Sess.User = sys
	e.Args = "local"
	if err := cmdPostnews(e); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	e.Sess = session.New("t", strings.NewReader("n\n"), &out)
	e.Sess.User = alice
	e.Args = "-n"
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skip me") || !strings.Contains(out.String(), "second") {
		t.Fatalf("nonstop %q", out.String())
	}
}

func TestReadnewsCheckAndList(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "one", Body: "a\n",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "two", Body: "b\n",
	}); err != nil {
		t.Fatal(err)
	}
	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}

	// -c は有無だけ
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = "-c"
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "News.") {
		t.Fatalf("-c want News.: %q", out.String())
	}

	// -l は見出しだけ。既読位置は変えない
	out.Reset()
	e.Sess = session.New("t", strings.NewReader(""), &out)
	e.Sess.User = alice
	e.Args = "-l"
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "one") || !strings.Contains(out.String(), "two") {
		t.Fatalf("-l want headings: %q", out.String())
	}
	if n, _ := st.CountUnreadNews(ctx, "alice"); n != 2 {
		t.Fatalf("-l should not mark read, got %d", n)
	}
}

// TestReadnewsUnifiedList: 自由入力の場では i=一覧 に揃える（今のグループの見出し）。
func TestReadnewsUnifiedList(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "one", Body: "a\n",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "two", Body: "b\n",
	}); err != nil {
		t.Fatal(err)
	}
	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("i\nq\n"), &out)
	e.Sess.User = alice
	e.Args = ""
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "i=一覧") {
		t.Fatalf("入場ガイドに i=一覧 が無い: %q", got)
	}
	if !strings.Contains(got, "local の見出し") || !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("i で見出し一覧が出るはず: %q", got)
	}
}

func TestReadnewsUnsubscribe(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "hi", Body: "x\n",
	}); err != nil {
		t.Fatal(err)
	}
	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("U\n"), &out)
	e.Sess.User = alice
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	sub, err := st.NewsSubscribed(ctx, "alice", "local")
	if err != nil {
		t.Fatal(err)
	}
	if sub {
		t.Fatalf("local should be unsubscribed")
	}
	if n, _ := st.CountUnreadNews(ctx, "alice"); n != 0 {
		t.Fatalf("unsubscribed group should not count, got %d", n)
	}
	// 名指しなら解除中でも読める
	q, err := newsQueue(e, "local")
	if err != nil || len(q) != 1 {
		t.Fatalf("named queue %v %v", q, err)
	}
}

func TestReadnewsReplyMail(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop", Subject: "topic", Body: "x\n",
	}); err != nil {
		t.Fatal(err)
	}
	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("r\n\nreply body\n.\nq\n"), &out)
	e.Sess.User = alice
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	inbox, err := st.ListInbox(ctx, "sysop")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || !strings.HasPrefix(inbox[0].Subject, "Re:") {
		t.Fatalf("reply mail missing: %+v out=%q", inbox, out.String())
	}
}

func TestReadnewsXDoesNotSave(t *testing.T) {
	ctx, st, e := newsEnv(t, acl.FlagSys)
	if _, err := st.PostNews(ctx, store.NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop",
		Subject: "keep", Body: "secret\n",
	}); err != nil {
		t.Fatal(err)
	}
	alice, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	e.Sess = session.New("t", strings.NewReader("x\n"), &out)
	e.Sess.User = alice
	if err := cmdReadnews(e); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountUnreadNews(ctx, "alice")
	if err != nil || n != 1 {
		t.Fatalf("x should not save %d err %v out=%q", n, err, out.String())
	}
}
