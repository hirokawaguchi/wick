package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyTimeZoneTokyo(t *testing.T) {
	c := Config{TimeZone: "Asia/Tokyo"}
	loc, _ := c.ApplyTimeZone()
	if loc == nil {
		t.Fatal("nil loc")
	}
	_, off := time.Now().In(loc).Zone()
	if off != 9*60*60 {
		t.Fatalf("offset %d", off)
	}
	if time.Local != loc {
		t.Fatal("time.Local not set")
	}
}

// TestConfigFilePrecedence は「env > ファイル > 既定」を確かめる。
func TestConfigFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wick.conf")
	body := `
# コメント行
export WICK_LANG = en
WICK_LISTEN=":2200"
WICK_AGENT_MODEL_ENDPOINT=http://localhost:11434/v1
WICK_AGENT_MODEL='deepseek-v4-flash:cloud'
WICK_AGENT_MODEL_KEY=sk-has#hash   # 値中の # は残す
WICK_MAX_SESSIONS=123
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WICK_CONFIG", path)
	// env はファイルより優先されることを確認するため 1 つだけ env で上書き。
	t.Setenv("WICK_LANG", "ja")

	c := Load()

	if c.Lang != "ja" {
		t.Errorf("env 優先が効いていない: Lang=%q want ja", c.Lang)
	}
	if c.Listen != ":2200" {
		t.Errorf("ファイル値が読めていない: Listen=%q want :2200", c.Listen)
	}
	if c.AgentModelEndpoint != "http://localhost:11434/v1" {
		t.Errorf("endpoint=%q", c.AgentModelEndpoint)
	}
	if c.AgentModelName != "deepseek-v4-flash:cloud" {
		t.Errorf("シングルクォート除去が効いていない: model=%q", c.AgentModelName)
	}
	if c.AgentModelKey != "sk-has#hash" {
		t.Errorf("値中の # を残せていない: key=%q", c.AgentModelKey)
	}
	if c.MaxSessions != 123 {
		t.Errorf("int 変換が効いていない: MaxSessions=%d want 123", c.MaxSessions)
	}
}

// TestConfigNoFile はファイルが無くても既定で動くことを確かめる。
func TestConfigNoFile(t *testing.T) {
	t.Setenv("WICK_CONFIG", filepath.Join(t.TempDir(), "nope.conf"))
	// 主要 env をクリア（テスト環境の汚染を避ける）。
	for _, k := range []string{"WICK_LANG", "WICK_LISTEN", "WICK_MAX_SESSIONS"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.Listen != ":2222" {
		t.Errorf("既定が効いていない: Listen=%q want :2222", c.Listen)
	}
	if c.MaxSessions != 500 {
		t.Errorf("既定が効いていない: MaxSessions=%d want 500", c.MaxSessions)
	}
}
