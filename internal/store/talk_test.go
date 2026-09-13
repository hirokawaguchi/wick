package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTalkUnread(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedTalkRooms(ctx); err != nil {
		t.Fatal(err)
	}
	rooms, err := s.ListTalkRooms(ctx)
	if err != nil || len(rooms) != MaxTalkRooms {
		t.Fatalf("rooms %d err %v", len(rooms), err)
	}
	ln, err := s.CreateTalkLine(ctx, TalkLine{
		Room: 1, Author: "alice", Handle: "Alice", Body: "hello talk",
	})
	if err != nil || ln.Num != 1 {
		t.Fatalf("line %+v err %v", ln, err)
	}
	r, err := s.GetTalkRoom(ctx, 1)
	if err != nil || r.LineCount != 1 || r.LastUpdate.IsZero() {
		t.Fatalf("room %+v err %v", r, err)
	}
	old := ln.PostTime.Add(-time.Second)
	got, err := s.ListTalkLinesSince(ctx, 1, old)
	if err != nil || len(got) != 1 || got[0].Body != "hello talk" {
		t.Fatalf("since %+v err %v", got, err)
	}
	fresh, err := s.ListTalkLinesSince(ctx, 1, ln.PostTime)
	if err != nil || len(fresh) != 0 {
		t.Fatalf("already seen %+v err %v", fresh, err)
	}
	all, err := s.ListTalkLines(ctx, 1, 1)
	if err != nil || len(all) != 1 {
		t.Fatalf("all %+v err %v", all, err)
	}
}
