// Package websearch は web 検索 MCP サーバの検索 Provider を提供する。
// 無課金を既定にする: stub（オフライン）／SearXNG（自ホスト・キー不要）／
// DuckDuckGo Instant Answer（キー不要・簡易フォールバック）。
//
// ここでは固定の（sysop 設定の）プロバイダ URL にしかアクセスしない。
// 任意 URL の取得（SSRF 対象）は行わない。取得系は web-get（増分4）で
// SSRF 防御込みで実装する。
package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result は検索 1 件。
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Provider は検索の実体。
type Provider interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}

// New は種別からプロバイダを作る。
//   - "stub"    : オフラインの固定結果（既定・開発用）
//   - "searxng" : SearXNG の JSON API（baseURL 必須）
//   - "ddg"     : DuckDuckGo Instant Answer（キー不要フォールバック）
func New(kind, baseURL string, hc *http.Client) (Provider, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "stub":
		return &StubProvider{}, nil
	case "searxng":
		if baseURL == "" {
			return nil, errors.New("searxng: baseURL が必要です")
		}
		return &SearxngProvider{Base: strings.TrimRight(baseURL, "/"), HC: hc}, nil
	case "ddg", "duckduckgo":
		return &DDGProvider{HC: hc}, nil
	default:
		return nil, fmt.Errorf("未知の provider: %q", kind)
	}
}

func capLimit(limit int) int {
	if limit <= 0 {
		return 5
	}
	if limit > 20 {
		return 20
	}
	return limit
}

// --- stub ---

// StubProvider は外部ネットに出ない固定結果を返す。
type StubProvider struct{}

func (p *StubProvider) Search(_ context.Context, query string, limit int) ([]Result, error) {
	limit = capLimit(limit)
	q := strings.TrimSpace(query)
	hosts := []string{"example.com", "example.org", "example.net"}
	out := make([]Result, 0, limit)
	for i := 0; i < limit && i < len(hosts); i++ {
		out = append(out, Result{
			Title:   fmt.Sprintf("%s に関する情報 (%d)", q, i+1),
			URL:     fmt.Sprintf("https://%s/%s", hosts[i], slug(q)),
			Snippet: fmt.Sprintf("「%s」の検索スタブ結果 %d 件目。開発用のダミーです。", q, i+1),
		})
	}
	return out, nil
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "q"
	}
	return b.String()
}

// --- SearXNG ---

// SearxngProvider は SearXNG の JSON API を叩く。
type SearxngProvider struct {
	Base string
	HC   *http.Client
}

func (p *SearxngProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	limit = capLimit(limit)
	u := p.Base + "/search?" + url.Values{
		"q":          {query},
		"format":     {"json"},
		"safesearch": {"1"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.HC.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("searxng: http %d", resp.StatusCode)
	}
	var body struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, limit)
	for _, r := range body.Results {
		if r.URL == "" {
			continue
		}
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// --- DuckDuckGo Instant Answer ---

// DDGProvider は DuckDuckGo Instant Answer API（キー不要）を使う。
// 結果は限定的（RelatedTopics 中心）なので、あくまでフォールバック。
type DDGProvider struct {
	HC   *http.Client
	base string // テスト用の差し替え。空なら本番エンドポイント。
}

func (p *DDGProvider) endpoint() string {
	if p.base != "" {
		return p.base
	}
	return "https://api.duckduckgo.com/"
}

func (p *DDGProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	limit = capLimit(limit)
	u := p.endpoint() + "?" + url.Values{
		"q":             {query},
		"format":        {"json"},
		"no_html":       {"1"},
		"no_redirect":   {"1"},
		"skip_disambig": {"1"},
		"t":             {"wick"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.HC.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("ddg: http %d", resp.StatusCode)
	}
	var body ddgResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, limit)
	// まず AbstractText（要約）があれば 1 件目に。
	if body.AbstractText != "" && body.AbstractURL != "" {
		out = append(out, Result{
			Title:   firstNonEmpty(body.Heading, query),
			URL:     body.AbstractURL,
			Snippet: body.AbstractText,
		})
	}
	// RelatedTopics（入れ子も展開）。
	for _, t := range flattenTopics(body.RelatedTopics) {
		if t.FirstURL == "" {
			continue
		}
		out = append(out, Result{Title: topicTitle(t.Text), URL: t.FirstURL, Snippet: t.Text})
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type ddgResponse struct {
	Heading       string     `json:"Heading"`
	AbstractText  string     `json:"AbstractText"`
	AbstractURL   string     `json:"AbstractURL"`
	RelatedTopics []ddgTopic `json:"RelatedTopics"`
}

type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Topics   []ddgTopic `json:"Topics"` // グループの入れ子
}

func flattenTopics(ts []ddgTopic) []ddgTopic {
	var out []ddgTopic
	for _, t := range ts {
		if len(t.Topics) > 0 {
			out = append(out, flattenTopics(t.Topics)...)
			continue
		}
		out = append(out, t)
	}
	return out
}

func topicTitle(text string) string {
	if i := strings.IndexByte(text, '-'); i > 0 {
		return strings.TrimSpace(text[:i])
	}
	if len(text) > 40 {
		return text[:40]
	}
	return text
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
