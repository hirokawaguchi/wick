package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// クライアントとサーバを実際につないで initialize〜tools/call を通す。
func TestServerClientRoundTrip(t *testing.T) {
	srv := NewServer("tok-1")
	srv.AddTool(ToolSpec{
		Name:        "echo",
		Description: "返すだけ",
		Handler: func(args json.RawMessage) (ToolResult, error) {
			var in struct {
				Q string `json:"query"`
			}
			_ = json.Unmarshal(args, &in)
			return ToolResult{Content: []ToolContent{{Type: "text", Text: "echo:" + in.Q}}}, nil
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", srv)
	hs := httptest.NewServer(mux)
	defer hs.Close()

	cli, err := DialTimeout(hs.URL+"/mcp", "tok-1", 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if cli.SessionID() == "" {
		t.Fatal("セッション id が空")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, err := cli.CallTool(ctx, "echo", map[string]any{"query": "月夜"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(tr.Content) != 1 || tr.Content[0].Text != "echo:月夜" {
		t.Fatalf("result=%+v", tr.Content)
	}
	cli.Close()
}

func TestServerRejectsBadToken(t *testing.T) {
	srv := NewServer("secret")
	mux := http.NewServeMux()
	mux.Handle("/mcp", srv)
	hs := httptest.NewServer(mux)
	defer hs.Close()

	if _, err := DialTimeout(hs.URL+"/mcp", "wrong", 3*time.Second); err == nil {
		t.Fatal("誤トークンで接続成功")
	}
}

func TestServerRequiresSession(t *testing.T) {
	srv := NewServer("")
	srv.AddTool(ToolSpec{Name: "x", Handler: func(json.RawMessage) (ToolResult, error) {
		return ToolResult{}, nil
	}})
	// セッションヘッダ無しの tools/call は 404。
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"x","arguments":{}}}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("session 無しで code=%d（404 期待）", rec.Code)
	}
}
