package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMailSendReadKill(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUser(ctx, "alice")
	if err != nil || !u.MailSave {
		t.Fatalf("mail_save default %+v err %v", u, err)
	}

	m, err := s.SendMail(ctx, Mail{
		FromID: "alice", FromHandle: "Alice", ToID: "bob",
		Subject: "hello", Body: "hi bob\n", Saved: true,
	})
	if err != nil || m.ID == 0 {
		t.Fatalf("send %+v err %v", m, err)
	}
	n, err := s.CountUnreadMail(ctx, "bob")
	if err != nil || n != 1 {
		t.Fatalf("unread %d err %v", n, err)
	}
	inbox, err := s.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 1 || inbox[0].Subject != "hello" {
		t.Fatalf("inbox %+v err %v", inbox, err)
	}
	sent, err := s.ListSent(ctx, "alice")
	if err != nil || len(sent) != 1 {
		t.Fatalf("sent %+v err %v", sent, err)
	}

	if err := s.MarkMailRead(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.KillMail(ctx, m.ID, "alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kill read: %v", err)
	}
	n, err = s.CountUnreadMail(ctx, "bob")
	if err != nil || n != 0 {
		t.Fatalf("read unread %d err %v", n, err)
	}

	m2, err := s.SendMail(ctx, Mail{
		FromID: "alice", FromHandle: "Alice", ToID: "bob",
		Subject: "withdraw", Body: "bye\n", Saved: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent, err = s.ListSent(ctx, "alice"); err != nil || len(sent) != 1 {
		t.Fatalf("unsaved hidden %+v err %v", sent, err)
	}
	wd, err := s.ListWithdrawable(ctx, "alice")
	if err != nil || len(wd) != 1 || wd[0].ID != m2.ID {
		t.Fatalf("withdraw %+v err %v", wd, err)
	}
	if err := s.KillMail(ctx, m2.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	inbox, err = s.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("killed gone %+v err %v", inbox, err)
	}

	if err := s.DeleteInboxMail(ctx, m.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	inbox, err = s.ListInbox(ctx, "bob")
	if err != nil || len(inbox) != 0 {
		t.Fatalf("deleted %+v err %v", inbox, err)
	}
}

func TestMailGroup(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SaveMailGroup(ctx, MailGroup{Owner: "alice", Name: "friends", Members: "bob sysop bob"}); err != nil {
		t.Fatal(err)
	}
	g, err := s.GetMailGroup(ctx, "alice", "friends")
	if err != nil || g.Members != "bob sysop" {
		t.Fatalf("%+v err %v", g, err)
	}
	list, err := s.ListMailGroups(ctx, "alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v err %v", list, err)
	}
	if err := s.DeleteMailGroup(ctx, "alice", "friends"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetMailGroup(ctx, "alice", "friends"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted %v", err)
	}
}
