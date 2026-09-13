package websearch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Page は web-get（取得）の結果。Text は整形済み本文の抜粋。
type Page struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// Fetcher は URL を取得して本文テキストにする。取得は検索より重く危険なので
// 強い制限をかける:
//   - スキームは http/https のみ（file: 等は拒否）
//   - 接続先 IP を解決後に検査し、ループバック・私有・リンクローカル等を拒否（SSRF/DNSリバインド対策）
//   - サイズ上限・時間上限・リダイレクト回数上限
//   - Content-Type は text/html・text/plain 系のみ
type Fetcher struct {
	HC           *http.Client
	MaxBytes     int64
	MaxRunes     int
	MaxRedirects int
	// AllowPrivate はテスト専用（httptest の 127.0.0.1 を許すため）。本番は false。
	AllowPrivate bool
}

var (
	errBlockedIP        = errors.New("websearch: 取得先が許可されない IP（内部/私有アドレス）")
	errBadScheme        = errors.New("websearch: http/https 以外のスキームは取得しない")
	errTooManyRedirects = errors.New("websearch: リダイレクトが多すぎます")
	errBadContentType   = errors.New("websearch: テキスト系以外の Content-Type は取得しない")
)

// NewFetcher は制限付きの取得器を作る。allowPrivate はテスト用（本番 false）。
func NewFetcher(allowPrivate bool) *Fetcher {
	f := &Fetcher{
		MaxBytes:     2 << 20, // 2 MiB
		MaxRunes:     2000,
		MaxRedirects: 3,
		AllowPrivate: allowPrivate,
	}
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control は DNS 解決後・接続直前に、実際に繋ぐ IP で呼ばれる（リバインド対策）。
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return errBlockedIP
			}
			if !f.AllowPrivate && !isPublicIP(ip) {
				return errBlockedIP
			}
			return nil
		},
	}
	tr := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		DisableKeepAlives:     true,
	}
	f.HC = &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= f.MaxRedirects {
				return errTooManyRedirects
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errBadScheme
			}
			return nil
		},
	}
	return f
}

// Fetch は URL を取得して本文テキストの抜粋を返す。
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Page{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Page{}, errBadScheme
	}
	if u.Host == "" {
		return Page{}, errors.New("websearch: ホストが空です")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", "wick-websearch/0.9 (+bot)")
	req.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.1")

	resp, err := f.HC.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Page{}, fmt.Errorf("websearch: http %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !okContentType(ct) {
		return Page{}, errBadContentType
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		return Page{}, err
	}
	truncatedBytes := int64(len(body)) > f.MaxBytes
	if truncatedBytes {
		body = body[:f.MaxBytes]
	}

	title := ""
	text := ""
	if strings.Contains(strings.ToLower(ct), "html") {
		title = extractTitle(string(body))
		text = htmlToText(string(body))
	} else {
		text = collapseSpaces(string(body))
	}

	truncatedText := false
	if r := []rune(text); f.MaxRunes > 0 && len(r) > f.MaxRunes {
		text = strings.TrimSpace(string(r[:f.MaxRunes]))
		truncatedText = true
	}
	return Page{
		Title:     title,
		URL:       resp.Request.URL.String(),
		Text:      text,
		Truncated: truncatedBytes || truncatedText,
	}, nil
}

func okContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return true // 未指定は許容（本文を軽く整形して返す）
	}
	return strings.HasPrefix(ct, "text/html") ||
		strings.HasPrefix(ct, "text/plain") ||
		strings.HasPrefix(ct, "application/xhtml")
}

// isPublicIP は「外に出してよい」公開 IP かを判定する（それ以外は SSRF として拒否）。
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// CGNAT 100.64.0.0/10。
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
	}
	return true
}

// --- HTML → テキスト（外部依存なしの簡易版） ---

func extractTitle(s string) string {
	lower := strings.ToLower(s)
	i := strings.Index(lower, "<title")
	if i < 0 {
		return ""
	}
	j := strings.IndexByte(s[i:], '>')
	if j < 0 {
		return ""
	}
	start := i + j + 1
	end := strings.Index(strings.ToLower(s[start:]), "</title>")
	if end < 0 {
		return ""
	}
	return collapseSpaces(html.UnescapeString(s[start : start+end]))
}

func htmlToText(s string) string {
	s = removeBlock(s, "script")
	s = removeBlock(s, "style")
	s = removeBlock(s, "noscript")

	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte(' ') // タグ境界は空白に
		case !inTag:
			b.WriteRune(r)
		}
	}
	return collapseSpaces(html.UnescapeString(b.String()))
}

// removeBlock は <tag ...>...</tag> ブロックを丸ごと除去する（大文字小文字無視）。
func removeBlock(s, tag string) string {
	open := "<" + tag
	close := "</" + tag + ">"
	for {
		lower := strings.ToLower(s)
		i := strings.Index(lower, open)
		if i < 0 {
			return s
		}
		j := strings.Index(lower[i:], close)
		if j < 0 {
			return s[:i] // 閉じが無ければ以降を捨てる
		}
		s = s[:i] + " " + s[i+j+len(close):]
	}
}

func collapseSpaces(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
