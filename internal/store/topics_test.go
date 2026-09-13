package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hirokawaguchi/wick/internal/i18n"
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

func TestSeedBoardsEN(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.SeedBoards(ctx, i18n.EN); err != nil {
		t.Fatal(err)
	}
	sb, err := s.GetBoard(ctx, "junk.sandbox")
	if err != nil {
		t.Fatal(err)
	}
	if sb.Desc != "another junk" {
		t.Fatalf("sandbox desc = %q", sb.Desc)
	}
	jobs, err := s.GetBoard(ctx, "sys.jobs")
	if err != nil {
		t.Fatal(err)
	}
	if jobs.Desc != "jobs for agents" {
		t.Fatalf("jobs desc = %q", jobs.Desc)
	}
	notes, err := s.ListNotes(ctx, jobs.ID)
	if err != nil || len(notes) == 0 {
		t.Fatalf("jobs notes: %d %v", len(notes), err)
	}
	if notes[0].Title != "Research: HyperNotes examples" {
		t.Fatalf("job title = %q", notes[0].Title)
	}
	if strings.Contains(notes[0].Body, "まとめて") {
		t.Fatalf("ja job body leaked: %q", notes[0].Body)
	}
	test, err := s.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	welcome, err := s.ListNotes(ctx, test.ID)
	if err != nil || len(welcome) == 0 {
		t.Fatalf("welcome notes: %d %v", len(welcome), err)
	}
	if strings.Contains(welcome[0].Body, "ベースノート") {
		t.Fatalf("ja welcome leaked: %q", welcome[0].Body)
	}
}
