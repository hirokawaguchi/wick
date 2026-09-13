package sshd

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/testenv"
	gossh "golang.org/x/crypto/ssh"
)

func TestBannerFromAsset(t *testing.T) {
	srv := &Server{Assets: assets.Dir{Root: testenv.Root(t)}}
	b := srv.banner(nil)
	if !strings.Contains(b, "guest") || !strings.Contains(b, "signup") {
		t.Fatalf("banner should advertise guest signup: %q", b)
	}
	if !strings.HasSuffix(b, "\r\n") {
		t.Fatalf("banner should end with CRLF: %q", b)
	}
	for _, line := range strings.Split(b, "\r\n") {
		if strings.Contains(line, "\n") {
			t.Fatalf("lone LF left in banner: %q", b)
		}
	}
}

func TestBannerDefaultWhenMissing(t *testing.T) {
	srv := &Server{Assets: assets.Dir{Root: filepath.Join(t.TempDir(), "no-assets")}}
	b := srv.banner(nil)
	if !strings.Contains(b, "guest") {
		t.Fatalf("default banner should mention guest: %q", b)
	}
}

// 実際に SSH で接続し、クライアントが認証前バナーを受け取れることを確かめる。
func TestBannerSentToClient(t *testing.T) {
	_, addr := startTestSSH(t)
	var got string
	var client *gossh.Client
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client, err = gossh.Dial("tcp", addr, &gossh.ClientConfig{
			User:            "alice",
			Auth:            []gossh.AuthMethod{gossh.Password("wick")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			BannerCallback:  func(msg string) error { got = msg; return nil },
			Timeout:         time.Second,
		})
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	if !strings.Contains(got, "guest") || !strings.Contains(got, "signup") {
		t.Fatalf("client did not receive pre-auth banner: %q", got)
	}
}
