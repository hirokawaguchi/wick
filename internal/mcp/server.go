package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ToolHandler は tools/call のツール本体。arguments は生 JSON のまま渡す。
type ToolHandler func(args json.RawMessage) (ToolResult, error)

// ToolSpec は 1 つのツールの定義。
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema。省略時は空オブジェクト。
	Handler     ToolHandler
}

// Server は Streamable HTTP の MCP サーバ（クライアントと対称）。
//   - 単一エンドポイントに JSON-RPC を POST で受ける
//   - initialize で Mcp-Session-Id を払い出し、以降のリクエストで必須にする
//   - 応答は SSE ストリーム（text/event-stream）で返す
//   - 認可は Authorization: Bearer（token 未設定なら認証なし）
type Server struct {
	token    string
	protocol string

	mu       sync.Mutex
	tools    map[string]ToolSpec
	order    []string
	sessions map[string]bool
}

// NewServer はサーバを作る。token が空なら認証なし。
func NewServer(token string) *Server {
	return &Server{
		token:    token,
		protocol: DefaultProtocol,
		tools:    map[string]ToolSpec{},
		sessions: map[string]bool{},
	}
}

// AddTool はツールを登録する。
func (s *Server) AddTool(spec ToolSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[spec.Name]; !ok {
		s.order = append(s.order, spec.Name)
	}
	s.tools[spec.Name] = spec
}

func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type incoming struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// ServeHTTP は MCP エンドポイント。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodDelete:
		// セッション終了。
		if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
			s.mu.Lock()
			delete(s.sessions, sid)
			s.mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var msg incoming
	if err := json.Unmarshal(body, &msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// initialize 以外はセッション必須。
	if msg.Method != "initialize" {
		sid := r.Header.Get("Mcp-Session-Id")
		s.mu.Lock()
		ok := sid != "" && s.sessions[sid]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound) // 期限切れ扱い→クライアントは再初期化
			return
		}
	}

	// 通知（id 無し）は本文なしで受理。
	if msg.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch msg.Method {
	case "initialize":
		sid := newSessionID()
		s.mu.Lock()
		s.sessions[sid] = true
		s.mu.Unlock()
		w.Header().Set("Mcp-Session-Id", sid)
		s.writeResult(w, *msg.ID, map[string]any{
			"protocolVersion": s.protocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "wick-websearch", "version": "0.9"},
		})
	case "tools/list":
		s.writeResult(w, *msg.ID, map[string]any{"tools": s.toolList()})
	case "tools/call":
		s.handleToolCall(w, *msg.ID, msg.Params)
	default:
		s.writeError(w, *msg.ID, -32601, "method not found: "+msg.Method)
	}
}

func (s *Server) toolList() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.order))
	for _, name := range s.order {
		t := s.tools[name]
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
	}
	return out
}

func (s *Server) handleToolCall(w http.ResponseWriter, id int64, params json.RawMessage) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeError(w, id, -32602, "invalid params")
		return
	}
	s.mu.Lock()
	t, ok := s.tools[p.Name]
	s.mu.Unlock()
	if !ok {
		s.writeError(w, id, -32602, "unknown tool: "+p.Name)
		return
	}
	res, err := t.Handler(p.Arguments)
	if err != nil {
		// ツールのエラーは isError の結果として返す（プロトコルエラーにしない）。
		res = ToolResult{
			Content: []ToolContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}
	}
	s.writeResult(w, id, res)
}

// writeResult は SSE ストリームで 1 件の応答を返して閉じる。
func (s *Server) writeResult(w http.ResponseWriter, id int64, result any) {
	env := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	s.writeSSE(w, env)
}

func (s *Server) writeError(w http.ResponseWriter, id int64, code int, message string) {
	env := map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
	s.writeSSE(w, env)
}

func (s *Server) writeSSE(w http.ResponseWriter, env map[string]any) {
	data, _ := json.Marshal(env)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
