package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/session"
)

func TestComposeBodySubmit(t *testing.T) {
	var out strings.Builder
	s := session.New("t", strings.NewReader("hi\nthere\n.\n"), &out)
	e := &Env{Sess: s}
	body, ok, err := composeBody(e)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if body != "hi\nthere\n" {
		t.Fatalf("body %q", body)
	}
}

func TestComposeBodyCancel(t *testing.T) {
	var out strings.Builder
	s := session.New("t", strings.NewReader("x\x03"), &out) // Ctrl-C
	e := &Env{Sess: s}
	if _, ok, _ := composeBody(e); ok {
		t.Fatal("Ctrl-C 中止が効いていない")
	}
}

func TestEditFieldTruncateLines(t *testing.T) {
	var out strings.Builder
	s := session.New("t", strings.NewReader("a\nb\nc\n.\n"), &out)
	e := &Env{Sess: s}
	text, ok, err := editField(e, "", 2, 78)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if text != "a\nb" {
		t.Fatalf("行数上限が効いていない: %q", text)
	}
}

func TestMailComposeCancelAtTitle(t *testing.T) {
	var out strings.Builder
	s := session.New("t", strings.NewReader("\x03"), &out) // 題入力で Ctrl-C
	e := &Env{Sess: s}
	_, _, ok, err := mailCompose(e)
	if ok {
		t.Fatal("Ctrl-C で中止にならない")
	}
	if !errors.Is(err, session.ErrInterrupt) {
		t.Fatalf("ErrInterrupt が返らない: %v", err)
	}
}

func TestEditFieldCancelKeeps(t *testing.T) {
	var out strings.Builder
	s := session.New("t", strings.NewReader("\x03"), &out) // Ctrl-C
	e := &Env{Sess: s}
	if _, ok, _ := editField(e, "既存", 8, 78); ok {
		t.Fatal("中止時に ok=true になっている")
	}
}
