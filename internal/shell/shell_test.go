package shell

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/reason"
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

func newRunner(t *testing.T, st store.Store) *Runner {
	t.Helper()
	as, tbl := loadAssets(t)
	return &Runner{Host: host.New(10), Store: st, ACL: tbl, Assets: as}
}

func TestOff(t *testing.T) {
	h := host.New(10)
	as, tbl := loadAssets(t)
	in := strings.NewReader("who\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	s.User = store.User{ID: "alice", Handle: "Alice", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	h.TryEnter(s)
	r := &Runner{Host: h, ACL: tbl, Assets: as}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code := r.Run(ctx, s)
	if code != reason.LogOff {
		t.Fatalf("code %d out=%q", code, out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("who missing: %q", out.String())
	}
}

func TestEOFIsCarrierDown(t *testing.T) {
	r := newRunner(t, nil)
	s := session.New("t", strings.NewReader(""), io.Discard)
	s.User = store.User{ID: "bob", Handle: "Bob", TLimit: 65535, Flags: acl.FlagGen, Expert: 1}
	code := r.Run(context.Background(), s)
	if code != reason.CarrierDown {
		t.Fatalf("code %d", code)
	}
}

func TestSetupHandleAndDenied(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}

	as, tbl := loadAssets(t)
	in := strings.NewReader("3\r\nhandle NewAlice\r\nexpert 1\r\n.\r\nlog\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	u, err := st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	s.User = u
	s.User.Expert = 1
	h := host.New(10)
	h.TryEnter(s)
	r := &Runner{Host: h, Store: st, ACL: tbl, Assets: as}
	code := r.Run(ctx, s)
	if code != reason.LogOff {
		t.Fatalf("code %d out=%q", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "handle = NewAlice") {
		t.Fatalf("handle not changed: %q", got)
	}
	if !strings.Contains(got, "expert = 1") {
		t.Fatalf("expert not changed: %q", got)
	}
	if !strings.Contains(got, "permission denied") {
		t.Fatalf("log should be denied: %q", got)
	}
	u, err = st.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Handle != "NewAlice" || u.Expert != 1 {
		t.Fatalf("store handle=%q expert=%d", u.Handle, u.Expert)
	}
}

func TestUnreadMailOnLogin(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SendMail(ctx, store.Mail{
		FromID: "alice", FromHandle: "Alice", ToID: "bob",
		Subject: "hi", Body: "hello\n", Saved: true,
	}); err != nil {
		t.Fatal(err)
	}
	as, tbl := loadAssets(t)
	in := strings.NewReader("off\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	u, err := st.GetUser(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	s.User = u
	s.User.Expert = 1
	h := host.New(10)
	h.TryEnter(s)
	r := &Runner{Host: h, Store: st, ACL: tbl, Assets: as}
	if code := r.Run(ctx, s); code != reason.LogOff {
		t.Fatalf("code %d out=%q", code, out.String())
	}
	if !strings.Contains(out.String(), "メールが 1 通あります") {
		t.Fatalf("unread missing: %q", out.String())
	}
}

func TestGuestSignupLoop(t *testing.T) {
	ctx := context.Background()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedGuest(ctx, "guest"); err != nil {
		t.Fatal(err)
	}
	as, tbl := loadAssets(t)
	// signup を選び新規登録、そのあと off
	// 新方式: ID は自動採番。入力はハンドル→パスワード×2 のみ。
	in := strings.NewReader("signup\r\nNewbie\r\nsecret\r\nsecret\r\noff\r\n")
	var out strings.Builder
	s := session.New("t", in, &out)
	g, err := st.GetUser(ctx, "guest")
	if err != nil {
		t.Fatal(err)
	}
	s.User = g
	h := host.New(10)
	h.TryEnter(s)
	r := &Runner{Host: h, Store: st, ACL: tbl, Assets: as}
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	code := r.Run(ctx2, s)
	if code != reason.LogOff {
		t.Fatalf("code %d out=%q", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "新規登録") {
		t.Fatalf("signup menu missing: %q", got)
	}
	// ゲスト専用ループなので MAIN は出ない
	if strings.Contains(got, "電子メール") {
		t.Fatalf("guest should not see MAIN: %q", got)
	}
	u, err := st.Authenticate(ctx, "prd00001", "secret")
	if err != nil {
		t.Fatalf("new user not created: %v out=%q", err, got)
	}
	if u.Handle != "Newbie" {
		t.Fatalf("handle %q", u.Handle)
	}
	if u.Flags != acl.FlagPro {
		t.Fatalf("new user flags %08x", u.Flags)
	}
}

func TestExpandLogin(t *testing.T) {
	got := expandLogin(`hi \ID \HANDLE \REMAIN`, store.User{ID: "alice", Handle: "Alice", TLimit: 30})
	if !strings.Contains(got, "alice") || !strings.Contains(got, "Alice") || !strings.Contains(got, "30") {
		t.Fatalf("%q", got)
	}
}
