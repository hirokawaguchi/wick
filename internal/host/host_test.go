package host

import (
	"testing"

	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func TestDuplicate(t *testing.T) {
	h := New(10)
	a := session.New("1", nil, nil)
	a.User = store.User{ID: "alice"}
	b := session.New("2", nil, nil)
	b.User = store.User{ID: "alice"}
	if !h.TryEnter(a) {
		t.Fatal("first enter")
	}
	if h.TryEnter(b) {
		t.Fatal("duplicate should fail")
	}
	h.Leave("alice")
	if !h.TryEnter(b) {
		t.Fatal("re-enter")
	}
	if b.Chan != 1 {
		t.Fatalf("reuse lowest chan, got %d", b.Chan)
	}
}

func TestMaxSessions(t *testing.T) {
	h := New(1)
	a := session.New("1", nil, nil)
	a.User = store.User{ID: "alice"}
	c := session.New("2", nil, nil)
	c.User = store.User{ID: "bob"}
	if !h.TryEnter(a) {
		t.Fatal("alice")
	}
	if h.TryEnter(c) {
		t.Fatal("over max")
	}
}
