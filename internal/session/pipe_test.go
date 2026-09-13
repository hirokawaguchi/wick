package session

import (
	"strings"
	"testing"
	"time"
)

// TestNewPipeNonBlockingOut は、読み手が居なくても Print が固まらないことを確かめる。
// 出力は同期パイプではなく上限付き観測シンクなので、大量出力でもブロックしない。
func TestNewPipeNonBlockingOut(t *testing.T) {
	s, aio := NewPipe("agent", "scout", "Scout")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			s.Print("0123456789ABCDEF\n") // 誰も読まない
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Print がブロックした（観測シンクが非ブロッキングでない）")
	}

	// 上限（agentOutCap）を超えないこと。
	snap := aio.Snapshot()
	if len(snap) > agentOutCap {
		t.Fatalf("Snapshot が上限超過: %d > %d", len(snap), agentOutCap)
	}
	if len(snap) == 0 {
		t.Fatal("Snapshot が空")
	}
	// Drain で空になること。
	if got := aio.Drain(); got == "" {
		t.Fatal("Drain が空")
	}
	if got := aio.Snapshot(); got != "" {
		t.Fatalf("Drain 後も残っている: %q", got)
	}
}

// TestNewPipeFeedRead は Feed した文字列が入力として読めることを確かめる。
func TestNewPipeFeedRead(t *testing.T) {
	s, aio := NewPipe("agent", "scout", "Scout")
	go func() { _ = aio.Feed("hello\n") }()
	line, err := s.ReadLine(64)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "hello" {
		t.Fatalf("ReadLine=%q, want hello", line)
	}
}

// TestNewPipeCloseEOF は Close で入力が閉じ、読み取りが EOF になることを確かめる。
func TestNewPipeCloseEOF(t *testing.T) {
	s, _ := NewPipe("agent", "scout", "Scout")
	go func() {
		time.Sleep(20 * time.Millisecond)
		s.Close() // AgentIO.Close 経由で入力パイプを閉じる
	}()
	if _, err := s.ReadLine(64); err == nil {
		t.Fatal("Close 後に EOF エラーが返らない")
	}
}
