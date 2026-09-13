package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
)

// noteOut はモデルに書かせるベースノートの構造化出力（題と本文）。
type noteOut struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// generateNote は与えられた話題で、掲示板に載せる短いベースノートを
// モデルに書かせて (題, 本文) を返す。JSON 以外や失敗時はエラー。
// poster から使い、失敗時は定型文へフォールバックする（デプロイを壊さない）。
func generateNote(cfg ModelConfig, handle, topic, webCtx string, langs ...i18n.Lang) (string, string, error) {
	lang := langOf(langs...)
	if !cfg.enabled() {
		return "", "", errors.New("model disabled")
	}
	temp := cfg.Temperature
	if temp == 0 {
		temp = 0.9 // ノートは少し発想を広げたいので高め
	}
	maxTok := cfg.MaxTokens
	if maxTok < 300 {
		maxTok = 300
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	sys := i18n.T(lang, "agent.llm.note_sys", handle)
	if webCtx != "" {
		sys += i18n.T(lang, "agent.llm.note_web")
	} else {
		sys += i18n.T(lang, "agent.llm.note_noweb")
	}
	user := i18n.T(lang, "agent.llm.note_user", topic)
	if webCtx != "" {
		user += "\n\n" + webCtx
	}

	reqBody := chatReq{
		Model:       cfg.Model,
		Temperature: temp,
		MaxTokens:   maxTok,
		Messages: []chatMsg{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := strings.TrimRight(cfg.Endpoint, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("model http %d", resp.StatusCode)
	}
	var cr chatResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", "", err
	}
	if len(cr.Choices) == 0 {
		return "", "", errors.New("no choices")
	}
	return parseNote(cr.Choices[0].Message.Content)
}

// parseNote はモデル応答から最初の JSON を取り出し、題と本文を整える。
func parseNote(s string) (string, string, error) {
	i := strings.IndexByte(s, '{')
	j := strings.LastIndexByte(s, '}')
	if i < 0 || j <= i {
		return "", "", errors.New("no json")
	}
	var n noteOut
	if err := json.Unmarshal([]byte(s[i:j+1]), &n); err != nil {
		return "", "", err
	}
	title := clipRunes(oneLine(n.Title), 60)
	body := sanitizeBody(n.Body)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return "", "", errors.New("empty note")
	}
	return title, body, nil
}

// sanitizeBody は本文を掲示板の投稿に安全な形へ整える。
//   - CR を除去、行頭/行末の空白を整える
//   - 単独の "." 行は投稿終端と衝突するので "・" に置換
//   - 最大 8 行・500 文字程度にクリップし、末尾に改行を足す（> 引用＋コメントが収まる程度）
func sanitizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r", "")
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		if strings.TrimSpace(ln) == "." {
			ln = "・"
		}
		out = append(out, ln)
		if len(out) >= 8 {
			break
		}
	}
	res := strings.TrimRight(strings.Join(out, "\n"), "\n")
	res = clipRunes(res, 500)
	return res + "\n"
}
