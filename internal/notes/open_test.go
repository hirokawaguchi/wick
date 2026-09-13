package notes

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func TestPostAndUnreadScan(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}

	in := strings.NewReader("wHello\nfrom alice\n.\nq")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	env := &Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st}
	if err := Open(env, "junk.test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "書き込み完了") {
		t.Fatalf("post missing: %q", out.String())
	}

	b, err := st.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	var hello store.Note
	ns, err := st.ListNotes(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range ns {
		if n.Title == "Hello" {
			hello = n
		}
	}
	if hello.ID == 0 {
		t.Fatal("hello note missing")
	}
	hello.LastUpdate = hello.LastUpdate.Add(2 * time.Second)
	if err := st.UpdateNote(ctx, hello); err != nil {
		t.Fatal(err)
	}
	b.LastUpdate = hello.LastUpdate
	if err := st.UpdateBoard(ctx, b); err != nil {
		t.Fatal(err)
	}

	var out2 strings.Builder
	s2 := session.New("t", strings.NewReader("q"), &out2)
	s2.User = store.User{ID: "bob", Handle: "Bob", Flags: acl.FlagGen, TLimit: 65535}
	s2.Sequencer = time.Unix(hello.LastUpdate.Unix()-1, 0)
	env2 := &Env{Ctx: ctx, Sess: s2, Host: host.New(10), Store: st}
	if err := Open(env2, "-s junk.test"); err != nil {
		t.Fatal(err)
	}
	got := out2.String()
	if !strings.Contains(got, "Hello") && !strings.Contains(got, "from alice") {
		t.Fatalf("unread missing: %q", got)
	}
}

func TestIndexRecentWindow(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	b, err := st.CreateBoard(ctx, store.Board{
		Name: "win.test", Desc: "window",
		Read: 0xffffffff, Write: 0xffffffff, Basenote: 0xffffffff,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 1; i <= 25; i++ {
		if _, err := st.CreateNote(ctx, store.Note{
			BoardID: b.ID, Title: "N" + strconv.Itoa(i),
			Author: "alice", Handle: "Alice",
			PostTime: now, LastUpdate: now, Body: "x\n",
		}); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader("q"), &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	if err := Open(&Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st}, "win.test"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "6..25 / 25") {
		t.Fatalf("window footer missing: %q", got)
	}
	if strings.Contains(got, "N1\r") || strings.Contains(got, "N5\r") {
		t.Fatalf("old notes should be hidden: %q", got)
	}
	if !strings.Contains(got, "N6\r") || !strings.Contains(got, "N25\r") {
		t.Fatalf("recent notes missing: %q", got)
	}
}

func TestWild(t *testing.T) {
	if !wild("junk.test", "*") || !wild("junk.test", "junk.*") || wild("sys.only", "junk.*") {
		t.Fatal("wild")
	}
}

func TestIndexPagingSearchSign(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	b, err := st.CreateBoard(ctx, store.Board{
		Name: "page.test", Desc: "paging", Sign: "看板テキスト\n",
		Read: 0xffffffff, Write: 0xffffffff, Basenote: 0xffffffff,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 1; i <= 25; i++ {
		if _, err := st.CreateNote(ctx, store.Note{
			BoardID: b.ID, Title: "N" + strconv.Itoa(i),
			Author: "alice", Handle: "Alice",
			PostTime: now, LastUpdate: now, Body: "x\n",
		}); err != nil {
			t.Fatal(err)
		}
	}
	user := store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}

	// * で最古頁へ: N1 が見えて N25 は隠れる
	var out strings.Builder
	s := session.New("t", strings.NewReader("*q"), &out)
	s.User = user
	if err := Open(&Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st}, "page.test"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, " N1\r") && !strings.Contains(got, "    1 ") {
		t.Fatalf("oldest page missing N1: %q", got)
	}

	// f で検索して 1 件だけ出る
	var out2 strings.Builder
	s2 := session.New("t", strings.NewReader("fN25\n\nq"), &out2)
	s2.User = user
	if err := Open(&Env{Ctx: ctx, Sess: s2, Host: host.New(10), Store: st}, "page.test"); err != nil {
		t.Fatal(err)
	}
	if got := out2.String(); !strings.Contains(got, "1 件") || !strings.Contains(got, "N25") {
		t.Fatalf("search result missing: %q", got)
	}

	// k で看板を表示
	var out3 strings.Builder
	s3 := session.New("t", strings.NewReader("kq"), &out3)
	s3.User = user
	if err := Open(&Env{Ctx: ctx, Sess: s3, Host: host.New(10), Store: st}, "page.test"); err != nil {
		t.Fatal(err)
	}
	if got := out3.String(); !strings.Contains(got, "看板テキスト") {
		t.Fatalf("sign missing: %q", got)
	}
}

func TestDeniedBoard(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateBoard(ctx, store.Board{
		Name: "hidden.sys", Desc: "no",
		Read: acl.FlagSys, Write: acl.FlagSys, Basenote: acl.FlagSys,
	}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader(""), &out)
	s.User = store.User{ID: "alice", Flags: acl.FlagGen, TLimit: 65535}
	if err := Open(&Env{Ctx: ctx, Sess: s, Store: st}, "hidden.sys"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "INDEX") {
		t.Fatalf("should skip: %q", out.String())
	}
}

func TestEndPromptKeys(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	b, err := st.CreateBoard(ctx, store.Board{
		Name: "end.test", Desc: "end",
		Read: 0xffffffff, Write: 0xffffffff, Basenote: 0xffffffff,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := st.CreateNote(ctx, store.Note{
		BoardID: b.ID, Title: "Only", Author: "alice", Handle: "Alice",
		PostTime: now, LastUpdate: now, Body: "body\n",
	}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader("\n\n q"), &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535}
	if err := Open(&Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st}, "end.test"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "go next") {
		t.Fatalf("old prompt: %q", got)
	}
	if !strings.Contains(got, "次レス") || !strings.Contains(got, "未読") {
		t.Fatalf("key hint missing: %q", got)
	}
	if !strings.Contains(got, "次のレスはありません") {
		t.Fatalf("end message missing: %q", got)
	}
}

func TestScanListLimitsNew(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	test, err := st.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	sand, err := st.GetBoard(ctx, "junk.sandbox")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := st.CreateNote(ctx, store.Note{
		BoardID: test.ID, Title: "OnTest", Author: "alice", Handle: "Alice",
		PostTime: now, LastUpdate: now, Body: "t\n",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNote(ctx, store.Note{
		BoardID: sand.ID, Title: "OnSand", Author: "alice", Handle: "Alice",
		PostTime: now, LastUpdate: now, Body: "s\n",
	}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s := session.New("t", strings.NewReader("q"), &out)
	s.User = store.User{ID: "alice", Flags: acl.FlagGen, TLimit: 65535, ScanList: "junk.sandbox"}
	s.Sequencer = now.Add(-time.Hour)
	if err := Open(&Env{Ctx: ctx, Sess: s, Store: st}, "-s@ *"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "<junk.sandbox>") {
		t.Fatalf("sandbox missing: %q", got)
	}
	if strings.Contains(got, "<junk.test>") {
		t.Fatalf("test should be skipped: %q", got)
	}
}

func TestAutosignOnPost(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader("wSigned\nhello\n.\nq")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", Flags: acl.FlagGen, TLimit: 65535, Autosign: "alice / Wick"}
	if err := Open(&Env{Ctx: ctx, Sess: s, Host: host.New(10), Store: st}, "junk.test"); err != nil {
		t.Fatal(err)
	}
	b, err := st.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	ns, err := st.ListNotes(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, n := range ns {
		if n.Title == "Signed" {
			body = n.Body
		}
	}
	if !strings.Contains(body, "hello") || !strings.Contains(body, "alice / Wick") {
		t.Fatalf("sign missing: %q", body)
	}
}
