package session

import "strings"

// 昔の BBS は等幅・固定桁を前提に画面を組んでいた。日本語端末では
// 全角＝2 セル・半角＝1 セルの「二倍幅」等幅なので、桁を揃えるには
// 文字数（rune）ではなく表示セル数で数える必要がある（等幅フォントだけでは揃わない）。
// ここではその「セル数え」を提供する。全角判定は wideRune（session.go）と共有。

// DisplayWidth は文字列の表示桁数（全角=2, 半角=1）を返す。
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if wideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// ClipWidth は表示桁数が cols を超えないよう末尾を切る（全角の途中では割らない）。
func ClipWidth(s string, cols int) string {
	if cols <= 0 {
		return ""
	}
	w := 0
	var b strings.Builder
	for _, r := range s {
		rw := 1
		if wideRune(r) {
			rw = 2
		}
		if w+rw > cols {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

// PadRight は左寄せで cols 桁に空白詰めする（fmt の %-Ns の全角対応版）。
// cols を超える分はクリップする。表組みの整列に使う。
func PadRight(s string, cols int) string {
	s = ClipWidth(s, cols)
	if pad := cols - DisplayWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// WrapWidth は文字列を各行 cols 桁以内に折り返す（切り捨てずに全文を残す）。
// 既存の改行は段落境界として尊重する。日本語は語間の空白が無いので桁で
// ハードに折る（会話文の途中で切れてしまうのを防ぐ＝そのままつなげて表示）。
func WrapWidth(s string, cols int) []string {
	if cols <= 0 {
		return []string{s}
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		for {
			chunk := ClipWidth(para, cols)
			if chunk == "" { // cols が極端に小さいときの保険
				lines = append(lines, para)
				break
			}
			lines = append(lines, chunk)
			if len(chunk) >= len(para) {
				break
			}
			para = para[len(chunk):]
		}
	}
	return lines
}
