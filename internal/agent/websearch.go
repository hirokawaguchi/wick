package agent

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Web 検索は「エージェント専用ツール」。人間向けコマンドは無い（web-search.md）。
// モデルには生の実行を触らせず、構造化アクション {"action":"web","text":"検索語"} を
// 出させ、Pilot 側（modelBrain）が whitelist（能力 Spec.Web）・回数予算・監査を通してから
// この Provider を呼ぶ（橋渡し）。結果は次の思考でプロンプトに載せ、モデルが URL 付きで発言する。
//
// WebProvider は検索の実体。当面はスタブ（固定ヒット）。将来 MCP クライアントを
// この interface として差し込み、自前 web 検索 MCP サーバや既存のナレッジ検索 MCP を同じ経路で使う。
type WebHit struct {
	Title   string
	URL     string
	Snippet string
}

// WebProvider は検索プロバイダ（スタブ / MCP クライアント / …）。
type WebProvider interface {
	Search(ctx context.Context, query string, limit int) ([]WebHit, error)
}

// WebPage は web-get（取得）の結果。Text は本文の抜粋。
type WebPage struct {
	Title     string
	URL       string
	Text      string
	Truncated bool
}

// WebFetcher は URL 取得プロバイダ（web-get）。Provider が対応していれば
// webBroker がここへ委譲する。スタブ（オフライン）は非対応＝取得できない。
type WebFetcher interface {
	Fetch(ctx context.Context, rawURL string) (WebPage, error)
}

// noopProvider は実プロバイダ未接続時のフォールバック。常に空を返す（＝web 無効）。
// これにより web=on でも MCP 未接続なら検索は起きず、偽の URL を出さない。
type noopProvider struct{}

func (noopProvider) Search(context.Context, string, int) ([]WebHit, error) { return nil, nil }

// stubProvider は開発・テスト用の固定ヒット。外部ネットに出ない。
type stubProvider struct{}

func (stubProvider) Search(_ context.Context, query string, limit int) ([]WebHit, error) {
	if limit <= 0 {
		limit = 5
	}
	base := []WebHit{
		{Title: query + " の概要", URL: "https://example.com/" + urlSlug(query), Snippet: query + " について広く知られている事柄の要約（スタブ）。"},
		{Title: query + " に関する記事", URL: "https://example.org/article/" + urlSlug(query), Snippet: query + " の背景と経緯（スタブ）。"},
		{Title: query + " まとめ", URL: "https://example.net/wiki/" + urlSlug(query), Snippet: query + " の要点（スタブ）。"},
	}
	if len(base) > limit {
		base = base[:limit]
	}
	return base, nil
}

// urlSlug は日本語などをそのまま使わないよう、英数以外を "-" に潰した簡易スラグ。
func urlSlug(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "q"
	}
	return out
}

// webBroker は Provider の前段。回数上限（limit）・タイムアウト・監査を持ち、
// 検索の唯一の入口にする（能力チェックと回数予算は呼び手 modelBrain 側）。
type webBroker struct {
	provider WebProvider
	fetcher  WebFetcher // web-get 用（nil なら取得不可）
	live     bool       // 実プロバイダ（MCP 等）が接続されているか。false なら検索を有効化しない
	limit    int
	timeout  time.Duration
	audit    func(agentID, query string, hits int) // 誰が何を引いたか（監査）
}

// Search はプロバイダを呼び、監査を残す。エラー時は空を返す（切断はしない＝安全側）。
func (b *webBroker) Search(agentID, query string) []WebHit {
	query = strings.TrimSpace(query)
	if b == nil || b.provider == nil || query == "" {
		return nil
	}
	to := b.timeout
	if to <= 0 {
		to = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	hits, err := b.provider.Search(ctx, query, b.limit)
	if err != nil {
		hits = nil
	}
	if b.audit != nil {
		b.audit(agentID, query, len(hits))
	}
	return hits
}

// Get は URL を取得する（web-get）。fetcher 未対応・失敗時は nil を返す（切断はしない）。
// 監査は「取得: URL」として残す（hits=1 は成功、0 は失敗/未対応）。
func (b *webBroker) Get(agentID, rawURL string) *WebPage {
	rawURL = strings.TrimSpace(rawURL)
	if b == nil || b.fetcher == nil || rawURL == "" {
		return nil
	}
	to := b.timeout
	if to <= 0 {
		to = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	page, err := b.fetcher.Fetch(ctx, rawURL)
	if b.audit != nil {
		n := 1
		if err != nil {
			n = 0
		}
		b.audit(agentID, "取得 "+rawURL, n)
	}
	if err != nil {
		return nil
	}
	return &page
}

// formatWebPage は取得結果をモデルへ渡す文脈テキストにする（元 URL を必ず含める）。
func formatWebPage(p *WebPage) string {
	if p == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("【web取得: " + p.URL + "】\n")
	if t := strings.TrimSpace(p.Title); t != "" {
		sb.WriteString("題: " + oneLine(t) + "\n")
	}
	body := strings.TrimSpace(p.Text)
	if body == "" {
		body = "(本文を取得できませんでした)"
	}
	sb.WriteString("本文（抜粋）:\n" + body + "\n")
	if p.Truncated {
		sb.WriteString("(以降は省略)\n")
	}
	sb.WriteString("これを要約して say で答え、本文に元の URL を必ず含めること。出典の無い断定はしないこと。\n")
	return sb.String()
}

// formatWebHits は検索結果をモデルへ渡す文脈テキストにする（URL を必ず含める）。
func formatWebHits(query string, hits []WebHit) string {
	var sb strings.Builder
	sb.WriteString("【web検索結果: " + query + "】\n")
	if len(hits) == 0 {
		sb.WriteString("(ヒットなし)\n")
	}
	for i, h := range hits {
		sb.WriteString(strconv.Itoa(i+1) + ". " + h.Title + " — " + h.URL + "\n")
		if s := strings.TrimSpace(h.Snippet); s != "" {
			sb.WriteString("   " + oneLine(s) + "\n")
		}
	}
	sb.WriteString("これらを踏まえ、必要なら本文に URL を含めて発言してください。出典の無い断定はしないこと。\n")
	return sb.String()
}

// research は自由文生成型ブレイン（talker/responder/poster）向けの任意 web 前処理。
// モデルに「この文脈で web 検索すべきか？するなら検索語を1行、不要なら NONE」を尋ね、
// 必要なら 1 回だけ検索して文脈テキスト（formatWebHits）を返す。web 未接続（live=false）・
// 予算切れ・不要・ヒット無しなら "" を返す（＝従来どおりの生成にフォールバック）。
// budget は呼び手（ブレイン）の残回数へのポインタ。成功時に 1 減らす。
func research(cfg ModelConfig, web *webBroker, budget *int, selfID, context string) string {
	if web == nil || !web.live || budget == nil || *budget <= 0 || !cfg.enabled() {
		return ""
	}
	q := decideQuery(cfg, context)
	if q == "" {
		return ""
	}
	hits := web.Search(selfID, q)
	*budget--
	if len(hits) == 0 {
		return ""
	}
	return formatWebHits(q, hits)
}

// decideQuery は「検索すべきか」をモデルに 1 行で判断させる。検索語 or 空（NONE）。
func decideQuery(cfg ModelConfig, context string) string {
	sys := "次の文脈に web 検索で裏取りすべき事実（固有名詞・作品・人物・製品・ニュース・数値・日付・場所・評判・仕様など）が" +
		"含まれるか判断します。少しでも事実が絡むなら検索する方針で、検索語だけを1行で返す。" +
		"あいさつや感想・気持ちだけで事実が無いときのみ NONE とだけ返す。前置き・説明・記号は書かない。"
	out, err := chatOnce(cfg, sys, "文脈:\n"+context, 40, 0.3)
	if err != nil {
		return ""
	}
	q := strings.TrimSpace(firstLine(out))
	q = strings.Trim(q, "「」\"'` ")
	if q == "" || strings.EqualFold(q, "NONE") || strings.EqualFold(q, "なし") {
		return ""
	}
	return q
}
