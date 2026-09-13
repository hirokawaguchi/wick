package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestScanListAndAutosign(t *testing.T) {
	if got := ParseScanList("junk.test\n\njunk.*\njunk.test"); len(got) != 2 || got[0] != "junk.test" {
		t.Fatalf("%v", got)
	}
	if ApplyAutosign("hi\n", "") != "hi\n" {
		t.Fatal("empty sign")
	}
	if ApplyAutosign("hi\n", "alice") != "hi\n--\nalice\n" {
		t.Fatalf("%q", ApplyAutosign("hi\n", "alice"))
	}

	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePrivate(ctx, "alice", "山田", "1990/01/02", "東京", "03-0000", "備考"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateScanList(ctx, "alice", "junk.*"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAutosign(ctx, "alice", "alice / Wick"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.RealName != "山田" || u.Birthday != "1990/01/02" || u.ScanList != "junk.*" || u.Autosign != "alice / Wick" {
		t.Fatalf("%+v", u)
	}
	if err := s.UpdateProfile(ctx, "alice", "公開の alice"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMailSave(ctx, "alice", false); err != nil {
		t.Fatal(err)
	}
	u, err = s.GetUser(ctx, "alice")
	if err != nil || u.Profile != "公開の alice" || u.MailSave {
		t.Fatalf("profile/save %+v err %v", u, err)
	}
}
