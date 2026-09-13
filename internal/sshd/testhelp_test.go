package sshd

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/store"
	"github.com/hirokawaguchi/wick/internal/testenv"
)

func startTestSSH(t *testing.T) (*Server, string) {
	t.Helper()
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
	if err := st.SeedBoards(ctx); err != nil {
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
	return srv, addr
}
