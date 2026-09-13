package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRogueSaveRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// 最初はセーブなし。
	if _, err := s.LoadRogue(ctx, "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("空のときは ErrNotFound のはず: %v", err)
	}
	if ok, _ := s.HasRogueSave(ctx, "u1"); ok {
		t.Fatalf("まだセーブは無いはず")
	}

	// 保存して読み戻す。
	blob := []byte{0x00, 0x01, 0x02, 0xff, 'r', 'o', 'g'}
	if err := s.SaveRogue(ctx, "u1", blob); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadRogue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(blob) {
		t.Fatalf("blob 不一致: %v", got)
	}

	// 上書き（1 人 1 本）。
	blob2 := []byte{9, 9, 9}
	if err := s.SaveRogue(ctx, "u1", blob2); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadRogue(ctx, "u1")
	if string(got) != string(blob2) {
		t.Fatalf("上書きされていない: %v", got)
	}

	// 削除。
	if err := s.DeleteRogue(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadRogue(ctx, "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("削除後は ErrNotFound のはず: %v", err)
	}
}

func TestRogueScores(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now()
	rows := []RogueScore{
		{UserID: "a", Handle: "Alice", Gold: 500, Depth: 8, MaxDepth: 10, Cause: "へびに殺された", Time: now},
		{UserID: "b", Handle: "Bob", Gold: 1200, Depth: 26, MaxDepth: 26, Won: true, Time: now},
		{UserID: "c", Handle: "Carol", Gold: 300, Depth: 4, MaxDepth: 4, Cause: "餓死した", Time: now},
	}
	for _, r := range rows {
		if err := s.AddRogueScore(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	top, err := s.TopRogueScores(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 3 {
		t.Fatalf("3 件のはず: %d", len(top))
	}
	// 金塊の多い順。
	if top[0].Handle != "Bob" || !top[0].Won {
		t.Fatalf("先頭は Bob（勝利）のはず: %+v", top[0])
	}
	if top[2].Handle != "Carol" {
		t.Fatalf("末尾は Carol のはず: %+v", top[2])
	}
}
