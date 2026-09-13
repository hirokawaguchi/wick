package agent

import (
	"testing"

	"github.com/hirokawaguchi/wick/internal/mcp"
)

func TestParseMCPHitsResultsObject(t *testing.T) {
	tr := mcp.ToolResult{Content: []mcp.ToolContent{{
		Type: "text",
		Text: `{"results":[{"title":"月の話","url":"https://example.com/moon","snippet":"満ちる"},{"title":"星","url":"https://example.org/star","snippet":"またたく"}]}`,
	}}}
	hits := parseMCPHits(tr, 5)
	if len(hits) != 2 {
		t.Fatalf("hits=%d", len(hits))
	}
	if hits[0].URL != "https://example.com/moon" || hits[0].Title != "月の話" {
		t.Fatalf("hit0=%+v", hits[0])
	}
}

func TestParseMCPHitsBareArrayAndLimit(t *testing.T) {
	tr := mcp.ToolResult{Content: []mcp.ToolContent{{
		Type: "text",
		Text: `[{"title":"a","url":"https://a","snippet":"1"},{"title":"b","url":"https://b","snippet":"2"},{"title":"c","url":"https://c","snippet":"3"}]`,
	}}}
	hits := parseMCPHits(tr, 2)
	if len(hits) != 2 {
		t.Fatalf("limit 未適用: hits=%d", len(hits))
	}
}

func TestParseMCPHitsPlainText(t *testing.T) {
	tr := mcp.ToolResult{Content: []mcp.ToolContent{{Type: "text", Text: "ただの説明文\n改行あり"}}}
	hits := parseMCPHits(tr, 5)
	if len(hits) != 1 || hits[0].URL != "" {
		t.Fatalf("plain text: %+v", hits)
	}
	if hits[0].Snippet == "" {
		t.Fatal("スニペットが空")
	}
}

func TestParseMCPPage(t *testing.T) {
	tr := mcp.ToolResult{Content: []mcp.ToolContent{{
		Type: "text",
		Text: `{"title":"月","url":"https://example.com/moon","text":"月は地球の衛星","truncated":true}`,
	}}}
	pg := parseMCPPage(tr)
	if pg.Title != "月" || pg.URL != "https://example.com/moon" || pg.Text != "月は地球の衛星" || !pg.Truncated {
		t.Fatalf("page=%+v", pg)
	}
}
