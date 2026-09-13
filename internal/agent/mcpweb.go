package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hirokawaguchi/wick/internal/mcp"
)

// mcpWebProvider は MCP サーバの検索/取得ツールを WebProvider/WebFetcher として使うアダプタ。
// tool（既定 web_search）を argKey（既定 query）＋limit で呼び、結果を WebHit へ変換する。
// 取得は getTool（既定 web_get）を url 引数で呼ぶ。
type mcpWebProvider struct {
	client  *mcp.Client
	tool    string
	argKey  string
	getTool string
}

// NewMCPWebProvider は MCP クライアントを web 検索 Provider（＋取得 Fetcher）として包む。
func NewMCPWebProvider(client *mcp.Client, tool, argKey string) *mcpWebProvider {
	if tool == "" {
		tool = "web_search"
	}
	if argKey == "" {
		argKey = "query"
	}
	return &mcpWebProvider{client: client, tool: tool, argKey: argKey, getTool: "web_get"}
}

// Fetch は web_get ツールを呼んで本文抜粋を返す（WebFetcher 実装）。
func (p *mcpWebProvider) Fetch(ctx context.Context, rawURL string) (WebPage, error) {
	tr, err := p.client.CallTool(ctx, p.getTool, map[string]any{"url": rawURL})
	if err != nil {
		return WebPage{}, err
	}
	return parseMCPPage(tr), nil
}

// parseMCPPage はツール結果（text ブロックの JSON）を WebPage へ変換する。
func parseMCPPage(tr mcp.ToolResult) WebPage {
	for _, blk := range tr.Content {
		if blk.Type != "" && blk.Type != "text" {
			continue
		}
		text := strings.TrimSpace(blk.Text)
		if text == "" {
			continue
		}
		var pg struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			Text      string `json:"text"`
			Truncated bool   `json:"truncated"`
		}
		if err := json.Unmarshal([]byte(text), &pg); err == nil && (pg.Text != "" || pg.URL != "" || pg.Title != "") {
			return WebPage{Title: pg.Title, URL: pg.URL, Text: pg.Text, Truncated: pg.Truncated}
		}
		// JSON でなければ本文そのものとして扱う。
		return WebPage{Text: text}
	}
	return WebPage{}
}

func (p *mcpWebProvider) Search(ctx context.Context, query string, limit int) ([]WebHit, error) {
	tr, err := p.client.CallTool(ctx, p.tool, map[string]any{
		p.argKey: query,
		"limit":  limit,
	})
	if err != nil {
		return nil, err
	}
	return parseMCPHits(tr, limit), nil
}

// parseMCPHits はツール結果（text ブロック）を WebHit へ変換する。
// 期待する形は {"results":[{title,url,snippet}...]} か、素の配列 [{...}]。
// JSON でなければ、そのテキストを 1 件のスニペットとして拾う（URL なし）。
func parseMCPHits(tr mcp.ToolResult, limit int) []WebHit {
	if limit <= 0 {
		limit = 5
	}
	var out []WebHit
	for _, blk := range tr.Content {
		if blk.Type != "" && blk.Type != "text" {
			continue
		}
		text := strings.TrimSpace(blk.Text)
		if text == "" {
			continue
		}
		if hits, ok := decodeHitsJSON(text); ok {
			out = append(out, hits...)
			continue
		}
		out = append(out, WebHit{Snippet: oneLine(text)})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

type mcpHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func decodeHitsJSON(text string) ([]WebHit, bool) {
	// {"results":[...]}
	var wrap struct {
		Results []mcpHit `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &wrap); err == nil && len(wrap.Results) > 0 {
		return toWebHits(wrap.Results), true
	}
	// 素の配列 [...]
	var arr []mcpHit
	if err := json.Unmarshal([]byte(text), &arr); err == nil && len(arr) > 0 {
		return toWebHits(arr), true
	}
	return nil, false
}

func toWebHits(hs []mcpHit) []WebHit {
	out := make([]WebHit, 0, len(hs))
	for _, h := range hs {
		out = append(out, WebHit{Title: h.Title, URL: h.URL, Snippet: h.Snippet})
	}
	return out
}
