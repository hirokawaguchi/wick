package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNotesUnread(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	if !b.CanRead(1 << 31) {
		t.Fatal("gen should read")
	}
	secret, err := s.CreateBoard(ctx, Board{
		Name: "sys.only", Desc: "hidden",
		Read: 1 << 15, Write: 1 << 15, Basenote: 1 << 15,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secret.CanRead(1 << 31) {
		t.Fatal("gen should not read sys.only")
	}

	n1, err := s.CreateNote(ctx, Note{
		BoardID: b.ID, Title: "from alice", Author: "alice", Handle: "Alice",
		Body: "hello bob\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateResponse(ctx, Response{
		NoteID: n1.ID, Title: "Re :from alice", Author: "bob", Handle: "Bob",
		Body: "hi alice\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetNote(ctx, b.ID, n1.Num)
	if err != nil || got.Response != 1 {
		t.Fatalf("res %d err %v", got.Response, err)
	}
	b2, err := s.GetBoard(ctx, "junk.test")
	if err != nil || b2.MsgCount < 2 {
		t.Fatalf("msgcount %d", b2.MsgCount)
	}
}

func TestPostgresNotes(t *testing.T) {
	dsn := os.Getenv("WICK_PG_DSN")
	if dsn == "" {
		t.Skip("WICK_PG_DSN not set")
	}
	ctx := context.Background()
	s, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedBoards(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.GetBoard(ctx, "junk.test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateNote(ctx, Note{
		BoardID: b.ID, Title: "pg", Author: "alice", Handle: "Alice",
		Body: "via postgres\n", PostTime: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoteLimits(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	b, err := s.CreateBoard(ctx, Board{Name: "lim.test", Read: 0xffffffff, Write: 0xffffffff, Basenote: 0xffffffff})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := s.exec(ctx, `
INSERT INTO notes (board_id, num, title, author, handle, post_time, last_update, flags, response, body)
VALUES (?, ?, 'full', 'alice', 'Alice', ?, ?, 0, 0, '')`, b.ID, MaxNotes, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNote(ctx, Note{BoardID: b.ID, Title: "x", Author: "alice", Handle: "A", Body: "x"}); !errors.Is(err, ErrTooManyNotes) {
		t.Fatalf("notes: %v", err)
	}
	n, err := s.GetNote(ctx, b.ID, MaxNotes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `
INSERT INTO responses (note_id, num, title, author, handle, post_time, flags, body)
VALUES (?, ?, '', 'alice', 'Alice', ?, 0, '')`, n.ID, MaxResponses, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateResponse(ctx, Response{NoteID: n.ID, Author: "alice", Handle: "A", Body: "x"}); !errors.Is(err, ErrTooManyResponses) {
		t.Fatalf("res: %v", err)
	}
}
