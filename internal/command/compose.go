package command

import (
	"strings"

	"github.com/hirokawaguchi/wick/internal/session"
)

// composeBody は本文をスクリーンエディタ（複数行・矢印移動可）で読む。
// 送信は単独行の "."、中止は Ctrl-C。空本文・中止は ok=false（"-- 中止 --" を表示）。
// 戻り値の本文は末尾に改行を 1 つ付ける（従来の join+"\n" と同じ）。
func composeBody(e *Env) (string, bool, error) {
	e.Sess.Print("本文 (矢印で移動して修正可。送信は単独行の . / 中止は Ctrl-C):\n")
	text, submitted, err := e.Sess.EditText("", 500)
	if err != nil {
		return "", false, err
	}
	if !submitted || strings.TrimRight(text, "\n") == "" {
		e.Sess.Print("-- 中止 --\n")
		return "", false, nil
	}
	return text + "\n", true, nil
}

// editField はプロフィール・署名・看板など複数行フィールドを現在値付きで編集する。
// init は現在値。ok=false は Ctrl-C 中止（呼び手は現状維持）。
// maxLines を超えた行は切り捨て、各行は maxLineLen 文字に丸める。
// 戻り値は末尾改行を落としたテキスト（空文字＝削除の意）。
func editField(e *Env, init string, maxLines, maxLineLen int) (string, bool, error) {
	text, submitted, err := e.Sess.EditText(strings.TrimRight(init, "\n"), maxLineLen)
	if err != nil {
		return "", false, err
	}
	if !submitted {
		return "", false, nil
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = session.ClipWidth(lines[i], maxLineLen)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n"), true, nil
}
