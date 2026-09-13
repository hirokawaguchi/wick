package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeServer は Streamable HTTP の最小 MCP サーバ。
// initialize で Mcp-Session-Id を払い出し、以降はセッション必須。tools/call は SSE で返す。
type fakeServer struct {
	token      string
	sessions   map[string]bool
	sawProto   string
	sawSession string
	sawAuth    string
	callSSE    bool // true なら tools/call を SSE で返す
}

func (f *fakeServer) handler(t *testing.T) http.HandlerFunc {
	if f.sessions == nil {
		f.sessions = map[string]bool{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		f.sawAuth = r.Header.Get("Authorization")
		if f.token != "" && f.sawAuth != "Bearer "+f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)

		if p := r.Header.Get("MCP-Protocol-Version"); p != "" {
			f.sawProto = p
		}
		if s := r.Header.Get("Mcp-Session-Id"); s != "" {
			f.sawSession = s
			if !f.sessions[s] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}

		switch req.Method {
		case "initialize":
			sid := "sess-abc"
			f.sessions[sid] = true
			w.Header().Set("Mcp-Session-Id", sid)
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, *req.ID, `{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}`)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			res := `{"content":[{"type":"text","text":"{\"results\":[{\"title\":\"T1\",\"url\":\"https://example.com/a\",\"snippet\":\"snip\"}]}"}],"isError":false}`
			if f.callSSE {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":%s}\n\n", *req.ID, res)
			} else {
				w.Header().Set("Content-Type", "application/json")
				writeJSON(w, *req.ID, res)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func writeJSON(w http.ResponseWriter, id int64, result string) {
	fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, id, result)
}

func TestClientHandshakeAndToolCall(t *testing.T) {
	for _, sse := range []bool{false, true} {
		fs := &fakeServer{token: "secret-123", callSSE: sse}
		srv := httptest.NewServer(fs.handler(t))
		defer srv.Close()

		cli, err := DialTimeout(srv.URL, "secret-123", 5*time.Second)
		if err != nil {
			t.Fatalf("sse=%v dial: %v", sse, err)
		}
		if cli.SessionID() != "sess-abc" {
			t.Fatalf("sse=%v session id = %q", sse, cli.SessionID())
		}
		if cli.Protocol() != "2025-06-18" {
			t.Fatalf("sse=%v protocol = %q", sse, cli.Protocol())
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		tr, err := cli.CallTool(ctx, "web_search", map[string]any{"query": "月"})
		cancel()
		if err != nil {
			t.Fatalf("sse=%v CallTool: %v", sse, err)
		}
		if len(tr.Content) != 1 || tr.Content[0].Type != "text" {
			t.Fatalf("sse=%v content = %+v", sse, tr.Content)
		}
		// 後続リクエストでセッションとプロトコルヘッダが付いていること。
		if fs.sawSession != "sess-abc" {
			t.Errorf("sse=%v server session = %q", sse, fs.sawSession)
		}
		if fs.sawProto != "2025-06-18" {
			t.Errorf("sse=%v server proto header = %q", sse, fs.sawProto)
		}
		cli.Close()
	}
}

func TestClientAuthRequired(t *testing.T) {
	fs := &fakeServer{token: "need-token"}
	srv := httptest.NewServer(fs.handler(t))
	defer srv.Close()

	if _, err := DialTimeout(srv.URL, "wrong", 3*time.Second); err == nil {
		t.Fatal("誤トークンで接続が成功してしまった")
	}
}
