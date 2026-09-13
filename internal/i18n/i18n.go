// Package i18n は Wick の多言語対応の土台（スキャフォールド）。
//
// 方針: 日本語(ja)を基準言語とし、まず ja を本番出荷する。英語(en)は骨組みで、
// 未翻訳のキーは自動的に ja へフォールバックする（欠落しても壊れない）。
// メッセージは「キー」で引き、翻訳が無ければ Default(=ja)、それも無ければキー自身を返す。
//
// 使い方:
//
//	i18n.T(lang, "saved")             // "-- 保存しました --"
//	i18n.T(lang, "lang.saved", label) // 書式引数つき（fmt.Sprintf）
//
// セッションからは session.Session.T(...) を使うと、そのユーザの言語で引ける。
package i18n

import (
	"fmt"
	"strings"
)

// Lang は表示言語のコード（ISO 639-1 の小文字）。
type Lang string

const (
	JA Lang = "ja"
	EN Lang = "en"

	// Default は基準言語。翻訳が無いときのフォールバック先。
	Default = JA
)

// Supported は対応言語を表示順で返す。
func Supported() []Lang { return []Lang{JA, EN} }

// Valid は対応言語かどうか。
func Valid(l Lang) bool { return l == JA || l == EN }

// Normalize は環境変数やユーザ設定の文字列を Lang に正規化する。
// 未知・空は Default(ja) に寄せる（壊れた値でも安全に既定へ倒す）。
func Normalize(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "en", "en-us", "en_us", "english", "eng":
		return EN
	case "ja", "jp", "ja-jp", "ja_jp", "japanese", "jpn":
		return JA
	}
	return Default
}

// Label は言語の人間向け表示名（その言語の自称）。
func Label(l Lang) string {
	switch l {
	case EN:
		return "English"
	default:
		return "日本語"
	}
}

// catalog はメッセージ辞書。key -> lang -> 文字列。
// ここは最初の一式（スキャフォールド）。UI 文字列の移行は段階的に増やす。
var catalog = map[string]map[Lang]string{
	"saved":     {JA: "-- 保存しました --", EN: "-- saved --"},
	"unchanged": {JA: "-- 変更しません --", EN: "-- unchanged --"},
	"invalid":   {JA: "invalid", EN: "invalid"},
	"aborted":   {JA: "-- 中止しました --", EN: "-- aborted --"},

	// language（lang コマンド）
	"lang.current": {JA: "表示言語 : %s", EN: "Language : %s"},
	"lang.opt.ja":  {JA: "  [1] 日本語 (ja)", EN: "  [1] 日本語 (ja)"},
	"lang.opt.en":  {JA: "  [2] English (en)", EN: "  [2] English (en)"},
	"lang.prompt":  {JA: "言語を選んでください (Enter=変更しない) : ", EN: "Choose a language (Enter=keep) : "},
	"lang.saved":   {JA: "-- 表示言語を %s にしました --", EN: "-- language set to %s --"},
}

// T は lang のメッセージを引く。args があれば fmt.Sprintf にかける。
// 見つからなければ Default(ja)、それも無ければキー自身を返す。
func T(lang Lang, key string, args ...any) string {
	s := lookup(lang, key)
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

func lookup(lang Lang, key string) string {
	m, ok := catalog[key]
	if !ok {
		return key
	}
	if s := m[lang]; s != "" {
		return s
	}
	if s := m[Default]; s != "" {
		return s
	}
	return key
}
