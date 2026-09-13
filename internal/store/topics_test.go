package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSeedTopicsAndDelete(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	// junk.test には seed 済みの説明ベースノートが1件ある想定。まず一掃してから話題を seed。
	if _, err := s.DeleteBoardNotes(ctx, "junk.test"); err != nil {
		t.Fatal(err)
	}
	topics := []NoteSeed{{Title: "雑談", Body: "なんでも\n"}, {Title: "本と音楽", Body: "本の話\n"}}
	if err := s.SeedTopics(ctx, "junk.test", "sysop", "Sysop", topics); err != nil {
		t.Fatal(err)
	}
	b, _ := s.GetBoard(ctx, "junk.test")
	notes, _ := s.ListNotes(ctx, b.ID)
	if len(notes) != 2 {
		t.Fatalf("話題が2件にならない: %d", len(notes))
	}
	if notes[0].Title != "雑談" {
		t.Fatalf("題が違う: %q", notes[0].Title)
	}
	// 冪等: 既にノートがあるので再 seed しても増えない。
	if err := s.SeedTopics(ctx, "junk.test", "sysop", "Sysop", topics); err != nil {
		t.Fatal(err)
	}
	notes, _ = s.ListNotes(ctx, b.ID)
	if len(notes) != 2 {
		t.Fatalf("冪等でない: %d", len(notes))
	}
	// レスを付けてから DeleteBoardNotes で全消去できること。
	if _, err := s.CreateResponse(ctx, Response{NoteID: notes[0].ID, Author: "sysop", Handle: "Sysop", Body: "レス"}); err != nil {
		t.Fatal(err)
	}
	n, err := s.DeleteBoardNotes(ctx, "junk.test")
	if err != nil || n != 2 {
		t.Fatalf("消去数が違う: %d %v", n, err)
	}
	notes, _ = s.ListNotes(ctx, b.ID)
	if len(notes) != 0 {
		t.Fatalf("消去後もノートが残る: %d", len(notes))
	}
}
