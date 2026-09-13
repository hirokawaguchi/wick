package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hirokawaguchi/wick/internal/store"
)

// mockModel は OpenAI 互換 /chat/completions を模したサーバを立て、
// content にそのまま返す文字列を仕込む。
func mockModel(t *testing.T, content string, capture *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.Error(w, "bad path", 404)
			return
		}
		if capture != nil {
			b, _ := io.ReadAll(r.Body)
			*capture = string(b)
		}
		resp := chatResp{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = content
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestModelBrain(t *testing.T, endpoint string) *modelBrain {
	t.Helper()
	return newModelBrain(ModelConfig{
		Endpoint: endpoint, Model: "test", Timeout: 3 * time.Second,
	}, Spec{Room: 1}, "poet", "Poet")
}

// TestModelBrainSay は say アクションがチャット発話（本文+改行）に翻訳されることを確かめる。
func TestModelBrainSay(t *testing.T) {
	var reqBody string
	srv := mockModel(t, `{"action":"say","text":"やあ、こんにちは"}`, &reqBody)
	b := newTestModelBrain(t, srv.URL)

	// 1手目は入室（決め打ち）。
	if s, _ := b.Next(Observation{Doing: "MAIN"}); s != "chat 1\n" {
		t.Fatalf("入室しない: %q", s)
	}
	// 2手目：部屋にいる状態で say（人が見ている＝Audience）。
	s, done := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> やあ\r\n", Audience: true})
	if done {
		t.Fatal("done になってしまった")
	}
	if s != "やあ、こんにちは\n" {
		t.Fatalf("say 翻訳が違う: %q", s)
	}
	// 画面（掲示板本文）がプロンプトに載っていること（> は JSON で \u003e に符号化）。
	if !strings.Contains(reqBody, "alice") {
		t.Fatalf("観測がプロンプトに含まれていない: %s", reqBody)
	}
}

// TestModelBrainSayOutsideRoom は部屋にいないと say を出さないことを確かめる。
func TestModelBrainSayOutsideRoom(t *testing.T) {
	srv := mockModel(t, `{"action":"say","text":"独り言"}`, nil)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true // 入室済みだが今は MAIN にいる想定
	if s, _ := b.Next(Observation{Doing: "MAIN", Screen: "\r\nalice> やあ\r\n"}); s != "" {
		t.Fatalf("部屋外で発話した: %q", s)
	}
}

// TestModelBrainWebThenCite は、モデルが action=web で検索を要求し、返ってきた
// 検索結果（URL付き）を踏まえて say で URL を含めて発言することを確かめる（橋渡し）。
func TestModelBrainWebThenCite(t *testing.T) {
	var calls int
	var lastReq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastReq = string(b)
		calls++
		content := `{"action":"web","text":"満月"}`
		if calls >= 2 { // 2 回目は検索結果を使って URL 付きで発言
			content = `{"action":"say","text":"満月の記事: https://example.com/moon"}`
		}
		resp := chatResp{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = content
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.web = &webBroker{provider: stubProvider{}, limit: 3, timeout: time.Second, audit: func(string, string, int) {}}
	b.webSearch = true
	b.webBudget = 5

	s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> 満月について教えて\r\n", Audience: true})
	if s != "満月の記事: https://example.com/moon\n" {
		t.Fatalf("URL 付きの発言になっていない: %q (calls=%d)", s, calls)
	}
	if calls < 2 {
		t.Fatalf("web の一手が飛ばされた: calls=%d", calls)
	}
	if b.webUsed != 1 {
		t.Fatalf("検索回数の集計が違う: %d", b.webUsed)
	}
	// 2 回目のリクエストには検索結果（URL）が文脈として載っているはず。
	if !strings.Contains(lastReq, "http") {
		t.Fatalf("検索結果がプロンプトに載っていない: %s", lastReq)
	}
}

// fakeFetcher はテスト用の WebFetcher。
type fakeFetcher struct {
	page WebPage
	err  error
	got  string
}

func (f *fakeFetcher) Fetch(_ context.Context, rawURL string) (WebPage, error) {
	f.got = rawURL
	return f.page, f.err
}

// TestModelBrainGetThenSummarize は、action=get で URL 取得を要求し、返ってきた
// 本文を踏まえて say で元 URL を含めて要約発言することを確かめる（橋渡し）。
func TestModelBrainGetThenSummarize(t *testing.T) {
	var calls int
	var lastReq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastReq = string(b)
		calls++
		content := `{"action":"get","text":"https://example.com/moon"}`
		if calls >= 2 {
			content = `{"action":"say","text":"要約: 月の記事 https://example.com/moon"}`
		}
		resp := chatResp{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = content
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ff := &fakeFetcher{page: WebPage{Title: "月", URL: "https://example.com/moon", Text: "月は地球の衛星。"}}
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.web = &webBroker{provider: stubProvider{}, fetcher: ff, limit: 3, timeout: time.Second, audit: func(string, string, int) {}}
	b.webGet = true
	b.webBudget = 5

	s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> この URL 読んで\r\n", Audience: true})
	if s != "要約: 月の記事 https://example.com/moon\n" {
		t.Fatalf("要約発言になっていない: %q (calls=%d)", s, calls)
	}
	if ff.got != "https://example.com/moon" {
		t.Fatalf("取得先が違う: %q", ff.got)
	}
	if b.webUsed != 1 {
		t.Fatalf("取得回数の集計が違う: %d", b.webUsed)
	}
	if !strings.Contains(lastReq, "月は地球の衛星") {
		t.Fatalf("取得本文がプロンプトに載っていない: %s", lastReq)
	}
}

// TestResearchDecidesAndSearches は、research が「検索語」を得たとき検索して文脈を返し、
// live=false や NONE のときは何もしないことを確かめる。
func TestResearchDecidesAndSearches(t *testing.T) {
	// モデルは検索語 "ちいかわ 映画" を返す。
	srv := mockModel(t, "ちいかわ 映画", nil)
	cfg := ModelConfig{Endpoint: srv.URL, Model: "test", Timeout: 3 * time.Second}
	web := &webBroker{provider: stubProvider{}, live: true, limit: 3, timeout: time.Second, audit: func(string, string, int) {}}
	budget := 5
	got := research(cfg, web, &budget, "kai", "ちいかわの映画の話")
	if got == "" || !strings.Contains(got, "http") {
		t.Fatalf("検索結果の文脈が返らない: %q", got)
	}
	if budget != 4 {
		t.Fatalf("予算が減っていない: %d", budget)
	}
}

func TestResearchSkipsWhenNotLive(t *testing.T) {
	srv := mockModel(t, "何か", nil)
	cfg := ModelConfig{Endpoint: srv.URL, Model: "test", Timeout: 3 * time.Second}
	web := &webBroker{provider: stubProvider{}, live: false, limit: 3, timeout: time.Second}
	budget := 5
	if got := research(cfg, web, &budget, "kai", "話題"); got != "" {
		t.Fatalf("未接続なのに検索した: %q", got)
	}
	if budget != 5 {
		t.Fatalf("未接続なのに予算が減った: %d", budget)
	}
}

func TestResearchSkipsOnNONE(t *testing.T) {
	srv := mockModel(t, "NONE", nil)
	cfg := ModelConfig{Endpoint: srv.URL, Model: "test", Timeout: 3 * time.Second}
	web := &webBroker{provider: stubProvider{}, live: true, limit: 3, timeout: time.Second, audit: func(string, string, int) {}}
	budget := 5
	if got := research(cfg, web, &budget, "kai", "特に事実確認の要らない雑談"); got != "" {
		t.Fatalf("NONE なのに検索した: %q", got)
	}
	if budget != 5 {
		t.Fatalf("NONE なのに予算が減った: %d", budget)
	}
}

// TestModelBrainGetDisabled は、webGet が無効なら get を要求されても黙ることを確かめる。
func TestModelBrainGetDisabled(t *testing.T) {
	srv := mockModel(t, `{"action":"get","text":"https://example.com/x"}`, nil)
	ff := &fakeFetcher{page: WebPage{Text: "x"}}
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.web = &webBroker{provider: stubProvider{}, fetcher: ff, limit: 3, timeout: time.Second, audit: func(string, string, int) {}}
	b.webGet = false // 取得能力なし
	b.webBudget = 5
	if s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> 読んで\r\n", Audience: true}); s != "" {
		t.Fatalf("get 能力なしで出力した: %q", s)
	}
	if ff.got != "" {
		t.Fatal("能力なしなのに取得してしまった")
	}
}

// TestModelBrainWebDisabled は、web 能力が無い（b.web==nil）ときに web を要求されても
// 黙る（コマンドを出さない）ことを確かめる。
func TestModelBrainWebDisabled(t *testing.T) {
	srv := mockModel(t, `{"action":"web","text":"何か"}`, nil)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true // web 未設定（nil）
	if s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> 調べて\r\n", Audience: true}); s != "" {
		t.Fatalf("web 能力なしで何か出力した: %q", s)
	}
}

// TestModelBrainTelegram は telegram アクションが ! 宛先 本文 に翻訳されることを確かめる。
func TestModelBrainTelegram(t *testing.T) {
	srv := mockModel(t, `ここは説明。{"action":"telegram","target":"sysop","text":"報告です"} 以上。`, nil)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	s, _ := b.Next(Observation{Doing: "MAIN", Screen: "\r\nsysop> 状況は？\r\n"})
	if s != "! sysop 報告です\n" {
		t.Fatalf("telegram 翻訳が違う: %q", s)
	}
}

// TestModelBrainIdleAndError は idle と HTTP エラーがともに「黙る」ことを確かめる。
func TestModelBrainIdle(t *testing.T) {
	srv := mockModel(t, `{"action":"idle"}`, nil)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	if s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> どう？\r\n", Audience: true}); s != "" {
		t.Fatalf("idle なのに発話: %q", s)
	}
}

// TestModelBrainNoAudienceIdle は、部屋に人（観客）がいないときは他の AI が
// 喋っていても自発発話せず、LLM も呼ばないことを確かめる（無人時の浪費防止）。
func TestModelBrainNoAudienceIdle(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should not be called", 500)
	}))
	defer srv.Close()
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.silenceLimit = 1 // 本来なら毎心拍で考える設定でも…
	// 別の AI が喋っているが Audience=false（人がいない）。
	if s, _ := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nmuse> やあ\r\n"}); s != "" {
		t.Fatalf("無人の部屋で発話した: %q", s)
	}
	if calls != 0 {
		t.Fatalf("無人の部屋で LLM を呼んだ: calls=%d", calls)
	}
}

func TestModelBrainHTTPErrorSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	if s, done := b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> やあ\r\n", Audience: true}); s != "" || done {
		t.Fatalf("エラー時に黙らない: script=%q done=%v", s, done)
	}
}

// TestModelBrainSilenceGate は、誰も喋っていない間は LLM を呼ばず、
// 沈黙が silenceLimit 続いたときだけ 1 回考えることを確かめる（コスト/テンポ配慮）。
func TestModelBrainSilenceGate(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		resp := chatResp{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = `{"action":"idle"}`
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	b.silenceLimit = 3
	// 沈黙（他者発言なし）。最初の 2 回は呼ばない。人は在室（Audience）。
	b.Next(Observation{Doing: "CHAT1", Audience: true})
	b.Next(Observation{Doing: "CHAT1", Audience: true})
	if calls != 0 {
		t.Fatalf("沈黙中に LLM を呼んだ: calls=%d", calls)
	}
	// 3 回目で 1 度だけ考える。
	b.Next(Observation{Doing: "CHAT1", Audience: true})
	if calls != 1 {
		t.Fatalf("沈黙上限で呼ばれない/呼びすぎ: calls=%d", calls)
	}
}

// TestModelBrainTelegramReply は、受信した個人電報を検知して、宛先未指定でも
// 差出人へ telegram で返信することを確かめる（電報に返事が来る）。
func TestModelBrainTelegramReply(t *testing.T) {
	var reqBody string
	// 宛先を空にしても、差出人(sysop)へ返るはず。
	srv := mockModel(t, `{"action":"telegram","text":"了解、あとで送ります"}`, &reqBody)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	// 部屋にいる最中に電報が届いた画面（notice.go の書式）。
	screen := "\r\n** 電報 from sysop (Sysop) 12:00:00 **\r\n例の件どうなった？\r\n"
	// 部屋の中(CHAT1)なので、部屋内コマンドの /! で返すはず。
	s, _ := b.Next(Observation{Doing: "CHAT1", Screen: screen})
	if s != "/! sysop 了解、あとで送ります\n" {
		t.Fatalf("電報返信になっていない: %q", s)
	}
	if !strings.Contains(reqBody, "例の件どうなった？") {
		t.Fatalf("電報本文が文脈に載っていない: %s", reqBody)
	}
}

// TestLastTelegram は電報通知のパースを確かめる（自分宛の差出人と本文）。
func TestLastTelegram(t *testing.T) {
	screen := "\r\npoet> やあ\r\n** 電報 from alice (Alice) 09:30:00 **\r\nおはよう\r\n"
	tm := lastTelegram(screen, "poet")
	if tm.from != "alice" || tm.body != "おはよう" {
		t.Fatalf("パース失敗: %+v", tm)
	}
	// 自分が差出人の電報は拾わない。
	if got := lastTelegram("** 電報 from poet (Poet) 09:31:00 **\n返信\n", "poet"); got.from != "" {
		t.Fatalf("自分の電報を拾ってしまった: %+v", got)
	}
	en := lastTelegram("** telegram from alice (Alice) 09:30:00 **\nhello\n", "poet")
	if en.from != "alice" || en.body != "hello" {
		t.Fatalf("en parse: %+v", en)
	}
}

// TestGenerateNote は、モデルが返す JSON からノートの題・本文を組み立て、
// 単独の "." 行が投稿終端と衝突しないよう無害化されることを確かめる。
func TestGenerateNote(t *testing.T) {
	srv := mockModel(t, `{"title":"月夜のこと","body":"昨夜は月がきれいでした。\n.\n続きを書いてください。"}`, nil)
	cfg := ModelConfig{Endpoint: srv.URL, Model: "test", Timeout: 3 * time.Second}
	title, body, err := generateNote(cfg, "Columnist", "月", "")
	if err != nil {
		t.Fatalf("generateNote 失敗: %v", err)
	}
	if title != "月夜のこと" {
		t.Fatalf("題が違う: %q", title)
	}
	if strings.Contains(body, "\n.\n") {
		t.Fatalf("単独の . 行が残っている: %q", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatalf("本文が改行で終わっていない: %q", body)
	}
}

// TestPosterUsesGen は、gen があればモデル生成の題・本文で投稿スクリプトを作ることを確かめる。
func TestPosterUsesGen(t *testing.T) {
	pb := newPosterBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist")
	pb.gen = func(topic, webCtx string) (string, string, error) {
		return "生成タイトル", "生成本文です。\n", nil
	}
	base := time.Now()
	pb.lastPost = base.Add(-2 * time.Minute) // 間隔を満たす
	s, _ := pb.Next(Observation{Now: base, Doing: "MAIN"})
	if !strings.Contains(s, "生成タイトル") || !strings.Contains(s, "生成本文です。") {
		t.Fatalf("gen の内容で投稿していない: %q", s)
	}
	if !strings.HasPrefix(s, "open junk.test\nw生成タイトル\n") {
		t.Fatalf("投稿スクリプトの形が違う: %q", s)
	}
}

// TestPosterGenFallback は、gen が失敗したら定型文へフォールバックすることを確かめる。
func TestPosterGenFallback(t *testing.T) {
	pb := newPosterBrain(Spec{Board: "junk.test", Interval: time.Minute}, "columnist", "Columnist")
	pb.gen = func(topic, webCtx string) (string, string, error) {
		return "", "", io.EOF
	}
	base := time.Now()
	pb.lastPost = base.Add(-2 * time.Minute)
	s, _ := pb.Next(Observation{Now: base, Doing: "MAIN"})
	if !strings.Contains(s, "Columnist") { // 定型文はハンドル名を含む
		t.Fatalf("フォールバックしていない: %q", s)
	}
}

// TestModelBrainUsesHistory は、過去の発話が文脈としてプロンプトに載ること
// （断片だけで迷子にならない）を確かめる。
func TestModelBrainUsesHistory(t *testing.T) {
	var reqBody string
	srv := mockModel(t, `{"action":"say","text":"はい"}`, &reqBody)
	b := newTestModelBrain(t, srv.URL)
	b.entered = true
	// 1 ターン目（詩の話）。人は在室（Audience）。
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> どんな詩？\r\npoet> 抒情詩が多いよ\r\n", Audience: true})
	// 2 ターン目（短い follow-up）。ここで過去の文脈が載っていること。
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nsysop> 見せて\r\n", Audience: true})
	if !strings.Contains(reqBody, "抒情詩が多いよ") {
		t.Fatalf("過去の発話が文脈に含まれていない: %s", reqBody)
	}
	if !strings.Contains(reqBody, "見せて") {
		t.Fatalf("直近の発言が含まれていない: %s", reqBody)
	}
}

// TestModelBrainTokenBudget は、usage を集計し、上限に達したら以後 LLM を
// 呼ばず黙る（在室のまま）ことを確かめる。
func TestModelBrainTokenBudget(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		resp := chatResp{}
		resp.Choices = append(resp.Choices, struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{})
		resp.Choices[0].Message.Content = `{"action":"idle"}`
		resp.Usage.TotalTokens = 40 // 1 回 40 トークン
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := newModelBrain(ModelConfig{
		Endpoint: srv.URL, Model: "test", Timeout: 3 * time.Second, TokenBudget: 100,
	}, Spec{Room: 1}, "poet", "Poet")
	b.entered = true
	b.silenceLimit = 1 // 毎心拍で考える設定にする

	// 40, 80 と積み上がる（<100 なので 2 回は呼ぶ）。人は在室（Audience）。
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> a\r\n", Audience: true})
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> b\r\n", Audience: true})
	if calls != 2 {
		t.Fatalf("予算内で呼ばれない: calls=%d used=%d", calls, b.TokensUsed())
	}
	if b.TokensUsed() != 80 {
		t.Fatalf("トークン集計が違う: %d (want 80)", b.TokensUsed())
	}
	// 3 回目で 120>=100 になり以後は呼ばない。
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> c\r\n", Audience: true}) // これは呼ばれる（呼ぶ前の判定は 80<100）
	if calls != 3 {
		t.Fatalf("3 回目が呼ばれない: calls=%d", calls)
	}
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> d\r\n", Audience: true}) // 120>=100 で以後は静観
	b.Next(Observation{Doing: "CHAT1", Screen: "\r\nalice> e\r\n", Audience: true})
	if calls != 3 {
		t.Fatalf("予算超過後も呼んでいる: calls=%d used=%d", calls, b.TokensUsed())
	}
}

// TestParseAction は前後に散文があっても最初の JSON を拾えることを確かめる。
func TestParseAction(t *testing.T) {
	a, err := parseAction("よし。{\"action\":\"SAY\",\"text\":\"hi\"}\nおわり")
	if err != nil {
		t.Fatal(err)
	}
	if a.Action != "say" || a.Text != "hi" {
		t.Fatalf("parse 失敗: %+v", a)
	}
	// JSON が無ければ idle。
	if a2, _ := parseAction("説明だけ"); a2.Action != "idle" {
		t.Fatalf("JSON 無しで idle にならない: %+v", a2)
	}
}

// TestManagerModelFallback は接続未設定なら model→conversant にフォールバックすることを確かめる。
func TestManagerModelFallback(t *testing.T) {
	m := &Manager{specs: map[string]Spec{}, active: map[string]*pilot{}}
	u := store.User{ID: "poet", Handle: "Poet"}
	// modelCfg 未設定（Endpoint 空）。
	brain := m.buildBrainFor(Spec{ID: "poet", Behavior: "model", Room: 1}, u)
	if _, ok := brain.(*conversantBrain); !ok {
		t.Fatalf("フォールバックが conversant でない: %T", brain)
	}
	// 接続設定を与えると modelBrain。
	m.SetModel(ModelConfig{Endpoint: "http://x/v1", Model: "test"})
	brain2 := m.buildBrainFor(Spec{ID: "poet", Behavior: "model", Room: 1}, u)
	if _, ok := brain2.(*modelBrain); !ok {
		t.Fatalf("model 設定時に modelBrain でない: %T", brain2)
	}
}
