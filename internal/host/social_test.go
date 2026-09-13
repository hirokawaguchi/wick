package host

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

// safeBuf は複数ゴルーチンから安全に書ける文字列バッファ（テスト用）。
// 電報割り込みは読み取りゴルーチンの Print で書かれ、本文はメインで読まれる。
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

func TestTelegramInterrupt(t *testing.T) {
	h := New(10)
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	var out safeBuf
	bob := session.New("2", pr, &out)
	bob.User = store.User{ID: "bob", Handle: "Bob"}
	alice := session.New("1", strings.NewReader(""), io.Discard)
	alice.User = store.User{ID: "alice", Handle: "Alice"}
	if !h.TryEnter(alice) || !h.TryEnter(bob) {
		t.Fatal("enter")
	}
	done := make(chan string, 1)
	go func() {
		line, err := bob.ReadCommand(32)
		if err != nil {
			done <- "err:" + err.Error()
			return
		}
		done <- line
	}()
	time.Sleep(40 * time.Millisecond)
	if err := h.SendTelegram("bob", alice, "hi there"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !strings.Contains(out.String(), "hi there") {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "電報") || !strings.Contains(out.String(), "hi there") {
		t.Fatalf("interrupt missing: %q", out.String())
	}
	if _, err := pw.Write([]byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got != "ok" {
			t.Fatalf("line %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("read timeout")
	}
}

func TestTelegramByChan(t *testing.T) {
	h := New(10)
	var outB strings.Builder
	alice := session.New("ssh:a", strings.NewReader(""), io.Discard)
	alice.User = store.User{ID: "alice", Handle: "Alice"}
	bob := session.New("ssh:b", strings.NewReader(""), &outB)
	bob.User = store.User{ID: "bob", Handle: "Bob"}
	if !h.TryEnter(alice) || !h.TryEnter(bob) {
		t.Fatal("enter")
	}
	if alice.Chan != 1 || bob.Chan != 2 {
		t.Fatalf("chan alice=%d bob=%d", alice.Chan, bob.Chan)
	}
	if err := h.SendTelegram("2", alice, "by number"); err != nil {
		t.Fatal(err)
	}
	bob.DrainNotices()
	if !strings.Contains(outB.String(), "by number") {
		t.Fatalf("chan telegram missing: %q", outB.String())
	}
	if err := h.SendTelegram("9", alice, "none"); err != ErrNoChannel {
		t.Fatalf("empty chan: %v", err)
	}
}

type fakeConn struct{ closed bool }

func (f *fakeConn) Close() error { f.closed = true; return nil }

func TestKill(t *testing.T) {
	h := New(10)
	alice := session.New("1", strings.NewReader(""), io.Discard)
	alice.User = store.User{ID: "alice", Handle: "Alice"}
	bob := session.New("2", strings.NewReader(""), io.Discard)
	bob.User = store.User{ID: "bob", Handle: "Bob"}
	fc := &fakeConn{}
	bob.SetConn(fc)
	if !h.TryEnter(alice) || !h.TryEnter(bob) {
		t.Fatal("enter")
	}

	id, err := h.Kill("bob", alice)
	if err != nil || id != "bob" {
		t.Fatalf("kill: id=%q err=%v", id, err)
	}
	if !fc.closed {
		t.Fatal("接続が閉じられていない")
	}
	if !bob.Closed() {
		t.Fatal("セッションが closed になっていない")
	}
	if _, err := h.Kill("9", alice); err != ErrNoChannel {
		t.Fatalf("bad chan: %v", err)
	}
	if _, err := h.Kill("nouser", alice); err != ErrOffline {
		t.Fatalf("offline: %v", err)
	}
}

func TestTelegramOffline(t *testing.T) {
	h := New(10)
	alice := session.New("1", strings.NewReader(""), io.Discard)
	alice.User = store.User{ID: "alice", Handle: "Alice"}
	h.TryEnter(alice)
	if err := h.SendTelegram("bob", alice, "x"); err != ErrOffline {
		t.Fatalf("got %v", err)
	}
}

func TestChatSay(t *testing.T) {
	h := New(10)
	var outB strings.Builder
	alice := session.New("1", strings.NewReader(""), io.Discard)
	alice.User = store.User{ID: "alice", Handle: "Alice"}
	bob := session.New("2", strings.NewReader(""), &outB)
	bob.User = store.User{ID: "bob", Handle: "Bob"}
	if !h.TryEnter(alice) || !h.TryEnter(bob) {
		t.Fatal("enter")
	}
	if _, err := h.JoinChat(1, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := h.JoinChat(1, bob); err != nil {
		t.Fatal(err)
	}
	if err := h.SayChat(1, alice, "hello"); err != nil {
		t.Fatal(err)
	}
	bob.DrainNotices()
	if !strings.Contains(outB.String(), "hello") || !strings.Contains(outB.String(), "alice>") {
		t.Fatalf("chat missing: %q", outB.String())
	}
	h.Leave("alice")
	bob.DrainNotices()
	if !strings.Contains(outB.String(), "退室") {
		t.Fatalf("leave missing: %q", outB.String())
	}
}
