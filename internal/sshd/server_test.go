package sshd

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
	gossh "golang.org/x/crypto/ssh"
)

func TestSSHLoginAndOff(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	root := testenv.Root(t)
	tbl, err := acl.Load(filepath.Join(root, "etc"))
	if err != nil {
		t.Fatal(err)
	}
	as := assets.Dir{Root: root}
	srv := &Server{
		Store:  st,
		Host:   host.New(10),
		ACL:    tbl,
		Assets: as,
		Cfg: Config{
			Listen:  addr,
			HostKey: filepath.Join(dir, "hostkey"),
			MaxAuth: 4,
		},
	}
	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	var client *gossh.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client, err = gossh.Dial("tcp", addr, &gossh.ClientConfig{
			User:            "alice",
			Auth:            []gossh.AuthMethod{gossh.Password("wick")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
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

	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var buf []byte
	sess.Stdout = writeFunc(func(p []byte) (int, error) {
		buf = append(buf, p...)
		return len(p), nil
	})
	sess.Stderr = sess.Stdout
	if err := sess.Start(""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_, _ = stdin.Write([]byte("who\n"))
	time.Sleep(150 * time.Millisecond)
	_, _ = stdin.Write([]byte("off\n"))
	_ = sess.Wait()

	out := string(buf)
	if !contains(out, "alice") {
		t.Fatalf("output: %q", out)
	}
}

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
