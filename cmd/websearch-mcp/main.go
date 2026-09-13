// websearch-mcp はエージェント専用の web 検索 MCP サーバ（別サービス）。
// Wick 本体が MCP クライアントとして接続し、web_search ツールを使う。
// Provider は無課金を既定: stub（オフライン）／SearXNG（自ホスト）／DuckDuckGo。
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/mcp"
	"github.com/hirokawaguchi/wick/internal/websearch"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	listen := env("WEBSEARCH_LISTEN", ":8080")
	token := env("WEBSEARCH_TOKEN", "")
	kind := env("WEBSEARCH_PROVIDER", "stub")
	base := env("SEARXNG_URL", "")
	defLimit, _ := strconv.Atoi(env("WEBSEARCH_LIMIT", "5"))
	if defLimit <= 0 {
		defLimit = 5
	}

	prov, err := websearch.New(kind, base, &http.Client{Timeout: 8 * time.Second})
	if err != nil {
		log.Fatalf("provider: %v", err)
	}

	srv := mcp.NewServer(token)
	srv.AddTool(mcp.ToolSpec{
		Name:        "web_search",
		Description: "Web を検索し、タイトル・URL・抜粋の一覧を返す。",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "検索語"},
				"limit": map[string]any{"type": "integer", "description": "件数（既定5・上限20）"},
			},
			"required": []string{"query"},
		},
		Handler: searchHandler(prov, defLimit),
	})

	// web_get（取得＋要約）は重く危険なので既定 off。WEBSEARCH_GET=on で有効化。
	if isOn(env("WEBSEARCH_GET", "")) {
		fetcher := websearch.NewFetcher(false) // 本番は私有アドレス禁止（SSRF）
		srv.AddTool(mcp.ToolSpec{
			Name:        "web_get",
			Description: "指定 URL を取得し、本文テキストの抜粋を返す（要約は呼び手が行う）。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "取得する http/https URL"},
				},
				"required": []string{"url"},
			},
			Handler: getHandler(fetcher),
		})
		log.Print("web_get 有効（SSRF 防御・サイズ/時間/リダイレクト制限つき）")
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", srv)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	auth := "認証なし"
	if token != "" {
		auth = "Bearer 認証"
	}
	log.Printf("websearch-mcp listening on %s (provider=%s, %s)", listen, kind, auth)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatal(err)
	}
}

func isOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

func getHandler(f *websearch.Fetcher) mcp.ToolHandler {
	return func(args json.RawMessage) (mcp.ToolResult, error) {
		var in struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return mcp.ToolResult{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		page, err := f.Fetch(ctx, in.URL)
		if err != nil {
			return mcp.ToolResult{}, err
		}
		payload, _ := json.Marshal(page)
		return mcp.ToolResult{
			Content: []mcp.ToolContent{{Type: "text", Text: string(payload)}},
		}, nil
	}
}

func searchHandler(prov websearch.Provider, defLimit int) mcp.ToolHandler {
	return func(args json.RawMessage) (mcp.ToolResult, error) {
		var in struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return mcp.ToolResult{}, err
		}
		if in.Limit <= 0 {
			in.Limit = defLimit
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hits, err := prov.Search(ctx, in.Query, in.Limit)
		if err != nil {
			return mcp.ToolResult{}, err
		}
		payload, _ := json.Marshal(map[string]any{"results": hits})
		return mcp.ToolResult{
			Content: []mcp.ToolContent{{Type: "text", Text: string(payload)}},
		}, nil
	}
}
