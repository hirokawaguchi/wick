// Package mcp は最小限の MCP（Model Context Protocol）クライアント。
// トランスポートは現行の Streamable HTTP（2025-03-26 以降）:
//   - 単一エンドポイントへ JSON-RPC を POST する
//   - 応答は Content-Type により application/json（単発）か text/event-stream（SSE）
//   - セッションは initialize 応答の Mcp-Session-Id ヘッダで払い出され、以降のリクエストに付ける
//   - 認可は Authorization: Bearer（2025-06-18 の OAuth 2.1 リソースサーバ想定。トークンは sysop が事前発行）
//   - initialize 後は MCP-Protocol-Version ヘッダを全リクエストに付ける
//
// Wick はこのクライアントとして外部 MCP サーバ（web 検索など）のツールを使う。
// モデルには MCP を直接触らせず、agent 側の橋渡し（構造化アクション）から呼ぶ。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultProtocol は initialize で要求するプロトコル版（最新）。
const DefaultProtocol = "2025-06-18"

// Options は接続設定。
type Options struct {
	Token      string       // Authorization: Bearer（空なら付けない）
	HTTPClient *http.Client // 省略時は既定
	Protocol   string       // 省略時は DefaultProtocol
}

// Client は 1 つの MCP サーバへの接続。並行呼び出しに対応する。
type Client struct {
	hc       *http.Client
	endpoint string
	token    string
	protocol string

	mu        sync.Mutex
	sessionID string
	nextID    int64
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolContent は tools/call の結果 1 ブロック（text など）。
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolResult は tools/call の結果。
type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError"`
}

// Dial はエンドポイントへ initialize ハンドシェイクを行い、セッションを確立する。
// ctx はハンドシェイクの待ち時間を縛る。
func Dial(ctx context.Context, endpoint string, opts Options) (*Client, error) {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	proto := opts.Protocol
	if proto == "" {
		proto = DefaultProtocol
	}
	c := &Client{hc: hc, endpoint: endpoint, token: opts.Token, protocol: proto}
	if err := c.initialize(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// DialTimeout は Dial の簡易版（ハンドシェイクの待ちを timeout で縛る）。
func DialTimeout(endpoint, token string, timeout time.Duration) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Dial(ctx, endpoint, Options{Token: token})
}

// Close は現在のセッションを終了する（DELETE。失敗は無視）。
func (c *Client) Close() {
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint, nil)
	if err != nil {
		return
	}
	c.setHeaders(req, true)
	if resp, err := c.hc.Do(req); err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func (c *Client) setHeaders(req *http.Request, withSession bool) {
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	// initialize 後はプロトコル版を必須で付ける（2025-06-18）。
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocol)
	}
	if withSession && sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
}

// call は id 付きリクエストを送り、応答（result）を待つ。
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req, true)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	// initialize 応答でセッションが払い出される。
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("mcp: session expired (404)")
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("mcp: http %d", resp.StatusCode)
	}
	return c.readResult(resp, id)
}

// notify は id 無し（通知）を送る。応答は 202 で本文なし。
func (c *Client) notify(ctx context.Context, method string, params any) error {
	body, _ := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", method, params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req, true)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("mcp: notify http %d", resp.StatusCode)
	}
	return nil
}

// readResult は POST 応答（JSON か SSE）から id に一致する JSON-RPC 応答を取り出す。
func (c *Client) readResult(resp *http.Response, id int64) (json.RawMessage, error) {
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResult(resp.Body, id)
	}
	// 既定は単発 JSON。
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return matchRPC(data, id)
}

// matchRPC は 1 つの JSON-RPC メッセージ（data）が id に一致すれば result を返す。
func matchRPC(data []byte, id int64) (json.RawMessage, error) {
	var m struct {
		ID     *int64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.ID == nil || *m.ID != id {
		return nil, errNoMatch
	}
	if m.Error != nil {
		return nil, fmt.Errorf("mcp: rpc error %d: %s", m.Error.Code, m.Error.Message)
	}
	return m.Result, nil
}

var errNoMatch = errors.New("mcp: id 不一致")

// readSSEResult は SSE ストリームを読み、id に一致する message を返す。
func readSSEResult(body io.Reader, id int64) (json.RawMessage, error) {
	r := bufio.NewReader(body)
	var data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if data != "" {
				if res, mErr := matchRPC([]byte(data), id); mErr == nil {
					return res, nil
				}
			}
			if err == io.EOF {
				return nil, errors.New("mcp: 応答が来ないままストリーム終了")
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" { // イベント確定
			if data != "" {
				if res, mErr := matchRPC([]byte(data), id); mErr == nil {
					return res, nil
				} else if mErr != errNoMatch {
					return nil, mErr
				}
			}
			data = ""
			continue
		}
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimPrefix(line[len("data:"):], " ")
			if data != "" {
				data += "\n"
			}
			data += d
		}
		// event:/id:/コメント行は無視（応答は data の JSON-RPC で判別）
	}
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": c.protocol,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "wick", "version": "0.9"},
	}
	raw, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	// サーバが交渉したプロトコル版を採用（あれば）。
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(raw, &res) == nil && res.ProtocolVersion != "" {
		c.mu.Lock()
		c.protocol = res.ProtocolVersion
		c.mu.Unlock()
	}
	return c.notify(ctx, "notifications/initialized", nil)
}

// CallTool は tools/call を呼ぶ。arguments はツールの引数。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return ToolResult{}, err
	}
	var tr ToolResult
	if err := json.Unmarshal(raw, &tr); err != nil {
		return ToolResult{}, err
	}
	if tr.IsError {
		return tr, errors.New("mcp: tool returned error")
	}
	return tr, nil
}

// SessionID は現在のセッション id（テスト・観測用）。
func (c *Client) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// Protocol は交渉済みプロトコル版。
func (c *Client) Protocol() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.protocol
}
