// Package warn は、メッセージ本文に外部リンクが含まれるときに添える注意書きを
// 提供する。この局ではファイルの送受信を行わないため、リンク先は必ずサービス外
// からのダウンロードになる。表示側がこの警告を一度だけ出す。
package warn

import "strings"

// External は外部リンク警告の日本語（カタログ warn.external の JA と同文）。
// 画面表示はセッション言語の i18n キー warn.external を使う。
const External = "※ 外部サイトからのDLです。提供元を確認できないファイルは開かないでください。"

// HasLink は本文に http(s):// のリンクが含まれるかを返す。
func HasLink(s string) bool {
	return strings.Contains(s, "http://") || strings.Contains(s, "https://")
}
