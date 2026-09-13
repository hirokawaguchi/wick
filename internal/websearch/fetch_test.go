package websearch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchHTMLToText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>月の話 &amp; 星</title></head>
			<body><script>var x=1;</script><h1>見出し</h1><p>本文です。<b>強調</b>。</p>
			<style>.a{}</style></body></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(true) // テスト: ループバック許可
	page, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "月の話 & 星" {
		t.Fatalf("title=%q", page.Title)
	}
	if !strings.Contains(page.Text, "本文です") || !strings.Contains(page.Text, "見出し") {
		t.Fatalf("text=%q", page.Text)
	}
	if strings.Contains(page.Text, "var x") || strings.Contains(page.Text, ".a{}") {
		t.Fatalf("script/style が残っている: %q", page.Text)
	}
}

func TestFetchBlocksLoopbackSSRF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	defer srv.Close()

	f := NewFetcher(false) // 本番相当: 私有/ループバック禁止
	if _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("ループバックへの取得がブロックされていない（SSRF）")
	}
}

func TestFetchRejectsFileScheme(t *testing.T) {
	f := NewFetcher(true)
	if _, err := f.Fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("file スキームが拒否されていない")
	}
}

func TestFetchRejectsNonText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG"))
	}))
	defer srv.Close()
	f := NewFetcher(true)
	if _, err := f.Fetch(context.Background(), srv.URL); err != errBadContentType {
		t.Fatalf("非テキストが拒否されていない: %v", err)
	}
}

func TestFetchTruncatesLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("あ", 5000)))
	}))
	defer srv.Close()
	f := NewFetcher(true)
	f.MaxRunes = 100
	page, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Truncated {
		t.Fatal("truncated が立っていない")
	}
	if r := []rune(page.Text); len(r) > 100 {
		t.Fatalf("MaxRunes 超過: %d", len(r))
	}
}

func TestFetchTooManyRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()
	f := NewFetcher(true)
	f.MaxRedirects = 2
	if _, err := f.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("リダイレクト過多が拒否されていない")
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":              true,
		"1.1.1.1":              true,
		"127.0.0.1":            false,
		"10.0.0.1":             false,
		"192.168.1.1":          false,
		"172.16.5.4":           false,
		"169.254.1.1":          false,
		"100.64.0.1":           false, // CGNAT
		"::1":                  false,
		"fc00::1":              false, // ULA
		"2001:4860:4860::8888": true,
	}
	for s, want := range cases {
		if got := isPublicIP(net.ParseIP(s)); got != want {
			t.Errorf("isPublicIP(%s)=%v want %v", s, got, want)
		}
	}
}
