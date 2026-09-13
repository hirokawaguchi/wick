package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/reason"
)

func TestCreateUserFlagsAndGuest(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// 見習い会員を作る
	pro := uint32(1 << 12)
	if err := s.CreateUser(ctx, User{ID: "Newbie", Handle: "Newbie", Flags: pro, TLimit: 30}, "secret"); err != nil {
		t.Fatal(err)
	}
	u, err := s.Authenticate(ctx, "newbie", "secret")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if u.Flags != pro || u.TLimit != 30 {
		t.Fatalf("flags=%08x tlimit=%d", u.Flags, u.TLimit)
	}
	// 重複は弾く
	if err := s.CreateUser(ctx, User{ID: "newbie", Handle: "x"}, "p"); err != ErrAlreadyExists {
		t.Fatalf("want ErrAlreadyExists, got %v", err)
	}
	// 昇格
	if err := s.UpdateFlags(ctx, "newbie", 1<<31); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTLimit(ctx, "newbie", 60); err != nil {
		t.Fatal(err)
	}
	u, _ = s.GetUser(ctx, "newbie")
	if u.Flags != 1<<31 || u.TLimit != 60 {
		t.Fatalf("after promote flags=%08x tlimit=%d", u.Flags, u.TLimit)
	}

	// ゲスト口は冪等
	if err := s.SeedGuest(ctx, "guest"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedGuest(ctx, "guest"); err != nil {
		t.Fatal(err)
	}
	g, err := s.Authenticate(ctx, "guest", "guest")
	if err != nil {
		t.Fatalf("guest auth: %v", err)
	}
	if g.Flags != 1<<13 {
		t.Fatalf("guest flags %08x", g.Flags)
	}
}

func TestUserLangDefaultAndUpdate(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.CreateUser(ctx, User{ID: "u1", Handle: "U1", Flags: 1 << 31, TLimit: 30}, "p"); err != nil {
		t.Fatal(err)
	}
	// 既定は ja（列デフォルト）。
	u, err := s.GetUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Lang != "ja" {
		t.Fatalf("default lang = %q, want ja", u.Lang)
	}
	// 更新して読み戻す。
	if err := s.UpdateLang(ctx, "u1", "en"); err != nil {
		t.Fatal(err)
	}
	u, _ = s.GetUser(ctx, "u1")
	if u.Lang != "en" {
		t.Fatalf("after update lang = %q, want en", u.Lang)
	}
	// ListUsers でも lang が読める。
	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range users {
		if x.ID == "u1" {
			found = true
			if x.Lang != "en" {
				t.Fatalf("ListUsers lang = %q, want en", x.Lang)
			}
		}
	}
	if !found {
		t.Fatal("u1 not in ListUsers")
	}
}

func TestAuthenticateAndLog(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Authenticate(ctx, "alice", "wrong"); err != ErrBadPassword {
		t.Fatalf("want bad password, got %v", err)
	}
	if err := s.IncPWErr(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUser(ctx, "ALICE")
	if err != nil || u.PWErr != 1 {
		t.Fatalf("pwerr=%d err=%v", u.PWErr, err)
	}

	u, err = s.Authenticate(ctx, "alice", "wick")
	if err != nil {
		t.Fatal(err)
	}
	if u.Handle != "Alice" {
		t.Fatalf("handle %q", u.Handle)
	}
	if err := s.ClearPWErr(ctx, "alice"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	disc := now.Add(time.Minute)
	if err := s.InsertLog(ctx, AccessLog{
		UserID: u.ID, Handle: u.Handle,
		ConnectTime: now, DisconnectTime: &disc,
		Channel: "ssh:1", Reason: reason.LogOff,
	}); err != nil {
		t.Fatal(err)
	}
	logs, err := s.ListLogs(ctx, 5)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v %v", logs, err)
	}
	if logs[0].Reason != reason.LogOff {
		t.Fatalf("reason %d", logs[0].Reason)
	}
}

// TestStartFinishLog は「接続時に先行記録 → 切断時に更新」を検証する。
func TestStartFinishLog(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	conn := time.Now().Truncate(time.Second)
	id, err := s.StartLog(ctx, AccessLog{
		UserID: "alice", Handle: "Alice",
		ConnectTime: conn, Channel: "ssh:1", Reason: reason.Online,
	})
	if err != nil || id <= 0 {
		t.Fatalf("StartLog id=%d err=%v", id, err)
	}
	// 先行記録の時点では切断時刻なし・online。
	logs, err := s.ListLogs(ctx, 5)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v %v", logs, err)
	}
	if logs[0].DisconnectTime != nil || logs[0].Reason != reason.Online {
		t.Fatalf("expected open row, got %+v", logs[0])
	}

	disc := conn.Add(3 * time.Minute)
	if err := s.FinishLog(ctx, id, disc, reason.LogOff); err != nil {
		t.Fatal(err)
	}
	logs, err = s.ListLogs(ctx, 5)
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %v %v", logs, err)
	}
	if logs[0].DisconnectTime == nil || !logs[0].DisconnectTime.Equal(disc) {
		t.Fatalf("disconnect not updated: %+v", logs[0])
	}
	if logs[0].Reason != reason.LogOff {
		t.Fatalf("reason not updated: %d", logs[0].Reason)
	}
}

func TestSeedOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedIfEmpty(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	u, err := s.Authenticate(ctx, "sysop", "wick")
	if err != nil {
		t.Fatal(err)
	}
	if u.TLimit != 65535 {
		t.Fatalf("tlimit %d", u.TLimit)
	}
}

func TestUpdateProfile(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SeedIfEmpty(ctx, "wick"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateHandle(ctx, "alice", "Alicia"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateExpert(ctx, "alice", 2); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTerminal(ctx, "alice", 100, 30, 0, ">"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePassword(ctx, "alice", "secret"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUser(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.Handle != "Alicia" || u.Expert != 2 || u.TermWidth != 100 || u.Prompt != ">" {
		t.Fatalf("%+v", u)
	}
	if _, err := s.Authenticate(ctx, "alice", "secret"); err != nil {
		t.Fatal(err)
	}
}
