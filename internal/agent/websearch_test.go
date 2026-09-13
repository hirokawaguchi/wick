package agent

import (
	"strings"
	"testing"
	"time"
)

func TestStubProviderAndBroker(t *testing.T) {
	var gotID, gotQ string
	var gotN int
	b := &webBroker{
		provider: stubProvider{},
		limit:    2,
		timeout:  time.Second,
		audit:    func(id, q string, n int) { gotID, gotQ, gotN = id, q, n },
	}
	hits := b.Search("poet", "月の話")
	if len(hits) == 0 {
		t.Fatal("ヒットが空")
	}
	if len(hits) > 2 {
		t.Fatalf("limit が効いていない: %d", len(hits))
	}
	for _, h := range hits {
		if h.Title == "" || h.URL == "" {
			t.Fatalf("不正なヒット: %+v", h)
		}
	}
	if gotID != "poet" || gotQ != "月の話" || gotN != len(hits) {
		t.Fatalf("監査が違う: id=%q q=%q n=%d", gotID, gotQ, gotN)
	}
	// 空クエリ・provider 無しは安全に空。
	if got := b.Search("poet", "   "); got != nil {
		t.Fatalf("空クエリで検索した: %+v", got)
	}
	if got := (&webBroker{}).Search("x", "y"); got != nil {
		t.Fatalf("provider 無しで検索した: %+v", got)
	}
}

func TestFormatWebHitsHasURL(t *testing.T) {
	out := formatWebHits("満月", []WebHit{{Title: "満月", URL: "https://example.com/moon", Snippet: "説明"}})
	if !strings.Contains(out, "https://example.com/moon") {
		t.Fatalf("URL が出力に無い: %q", out)
	}
	if !strings.Contains(out, "満月") {
		t.Fatalf("クエリ/題が無い: %q", out)
	}
}
