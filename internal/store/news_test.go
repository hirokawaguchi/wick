package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewsPostUnreadCursor(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedNewsGroups(ctx); err != nil {
		t.Fatal(err)
	}
	g, err := s.GetNewsGroup(ctx, DefaultNewsGroup)
	if err != nil || g.Name != "local" {
		t.Fatalf("%+v err %v", g, err)
	}
	a, err := s.PostNews(ctx, NewsArticle{
		Group: "local", FromID: "sysop", FromHandle: "Sysop",
		Subject: "hello", Body: "body\n",
	})
	if err != nil || a.Num != 1 {
		t.Fatalf("%+v err %v", a, err)
	}
	n, err := s.CountUnreadNews(ctx, "alice")
	if err != nil || n != 1 {
		t.Fatalf("unread %d err %v", n, err)
	}
	if err := s.SetNewsCursor(ctx, "alice", "local", 1); err != nil {
		t.Fatal(err)
	}
	n, err = s.CountUnreadNews(ctx, "alice")
	if err != nil || n != 0 {
		t.Fatalf("after cursor %d err %v", n, err)
	}
	if err := s.SetNewsCursor(ctx, "alice", "local", 1); err != nil {
		t.Fatal(err)
	}
	got, err := s.NewsCursor(ctx, "alice", "local")
	if err != nil || got != 1 {
		t.Fatalf("cursor %d err %v", got, err)
	}
}
