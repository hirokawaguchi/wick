package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStubProvider(t *testing.T) {
	p := &StubProvider{}
	hits, err := p.Search(context.Background(), "月と詩", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits=%d", len(hits))
	}
	for _, h := range hits {
		if h.URL == "" || h.Title == "" {
			t.Fatalf("空フィールド: %+v", h)
		}
	}
}

func TestSearxngProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("format=%s", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"A","url":"https://a.example/1","content":"one"},
			{"title":"B","url":"https://b.example/2","content":"two"},
			{"title":"NoURL","url":"","content":"skip"}
		]}`))
	}))
	defer srv.Close()

	p, err := New("searxng", srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	hits, err := p.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 { // URL 空はスキップ
		t.Fatalf("hits=%d", len(hits))
	}
	if hits[0].URL != "https://a.example/1" || hits[0].Snippet != "one" {
		t.Fatalf("hit0=%+v", hits[0])
	}
}

func TestSearxngLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"title":"1","url":"https://x/1","content":"a"},
			{"title":"2","url":"https://x/2","content":"b"},
			{"title":"3","url":"https://x/3","content":"c"}
		]}`))
	}))
	defer srv.Close()
	p, _ := New("searxng", srv.URL, srv.Client())
	hits, _ := p.Search(context.Background(), "q", 2)
	if len(hits) != 2 {
		t.Fatalf("limit 未適用: %d", len(hits))
	}
}

func TestDDGProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Heading":"月",
			"AbstractText":"月は地球の衛星",
			"AbstractURL":"https://ja.wikipedia.org/wiki/月",
			"RelatedTopics":[
				{"Text":"満月 - 満ちた月","FirstURL":"https://example.com/full"},
				{"Topics":[{"Text":"新月 - 見えない月","FirstURL":"https://example.com/new"}]}
			]
		}`))
	}))
	defer srv.Close()

	// New では DDG は実 API を指すので、ここでは直接 baseURL を差し替えてテストする。
	p := &DDGProvider{HC: srv.Client(), base: srv.URL}
	hits, err := p.Search(context.Background(), "月", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 3 {
		t.Fatalf("hits=%d (>=3 期待)", len(hits))
	}
	if hits[0].URL != "https://ja.wikipedia.org/wiki/月" {
		t.Fatalf("abstract が先頭でない: %+v", hits[0])
	}
	// 入れ子 RelatedTopics も展開されること。
	found := false
	for _, h := range hits {
		if h.URL == "https://example.com/new" {
			found = true
		}
	}
	if !found {
		t.Fatal("入れ子 topic が展開されていない")
	}
}
