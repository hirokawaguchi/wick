package command

import (
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/store"
)

func TestShutdownRequestsStop(t *testing.T) {
	h := host.New(10)
	var out strings.Builder
	s := session.New("t", strings.NewReader("y\n"), &out)
	s.User = store.User{ID: "sysop", Handle: "sysop"}
	e := &Env{Sess: s, Host: h, Args: "点検します"}

	if err := cmdShutdown(e); err != ErrLogOff {
		t.Fatalf("want ErrLogOff, got %v", err)
	}
	select {
	case <-h.ShutdownC():
	default:
		t.Fatal("停止が要求されていない")
	}
	if !strings.Contains(out.String(), "停止を要求") {
		t.Fatalf("確認出力なし: %q", out.String())
	}
}

func TestPowerShowsUptime(t *testing.T) {
	h := host.New(10)
	var out strings.Builder
	s := session.New("t", strings.NewReader(""), &out)
	s.User = store.User{ID: "sysop", Handle: "sysop"}
	e := &Env{Sess: s, Host: h}

	if err := cmdPower(e); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"起動時刻", "稼働時間", "在室"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q が出力に無い: %q", want, got)
		}
	}
}

func TestShutdownCancel(t *testing.T) {
	h := host.New(10)
	var out strings.Builder
	s := session.New("t", strings.NewReader("n\n"), &out)
	s.User = store.User{ID: "sysop", Handle: "sysop"}
	e := &Env{Sess: s, Host: h}

	if err := cmdShutdown(e); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	select {
	case <-h.ShutdownC():
		t.Fatal("中止時に停止が要求された")
	default:
	}
	if !strings.Contains(out.String(), "中止") {
		t.Fatalf("中止出力なし: %q", out.String())
	}
}
