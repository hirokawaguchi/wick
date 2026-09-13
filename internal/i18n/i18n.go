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
	// 共通
	"saved":      {JA: "-- 保存しました --", EN: "-- saved --"},
	"unchanged":  {JA: "-- 変更しません --", EN: "-- unchanged --"},
	"invalid":    {JA: "invalid", EN: "invalid"},
	"aborted":    {JA: "-- 中止しました --", EN: "-- aborted --"},
	"none_paren": {JA: "(なし)", EN: "(none)"},

	// language（lang コマンド）
	"lang.current": {JA: "表示言語 : %s", EN: "Language : %s"},
	"lang.opt.ja":  {JA: "  [1] 日本語 (ja)", EN: "  [1] 日本語 (ja)"},
	"lang.opt.en":  {JA: "  [2] English (en)", EN: "  [2] English (en)"},
	"lang.prompt":  {JA: "言語を選んでください (Enter=変更しない) : ", EN: "Choose a language (Enter=keep) : "},
	"lang.saved":   {JA: "-- 表示言語を %s にしました --", EN: "-- language set to %s --"},

	// コマンド共通（command パッケージのヘルパ）
	"cmd.unimplemented": {JA: "%s: まだ実装されていません", EN: "%s: not implemented yet"},
	"cmd.denied":        {JA: "%s: permission denied", EN: "%s: permission denied"},
	"cmd.no_usage":      {JA: "%s: no usage", EN: "%s: no usage"},

	// メニュー操作（menu パッケージ）
	"menu.too_deep":    {JA: "** メニュー階層が深すぎます **", EN: "** menu nesting too deep **"},
	"menu.at_top":      {JA: "-- トップメニューです --", EN: "-- already at the top menu --"},
	"menu.no_item":     {JA: "no such item", EN: "no such item"},
	"menu.unknown_cmd": {JA: "%s: unknown command", EN: "%s: unknown command"},
	"menu.aborted":     {JA: "-- 中止 --", EN: "-- aborted --"},
	"help.header":      {JA: "コマンド一覧（詳しい使い方は  ? 名前   または  名前 -?  ）", EN: "Commands (for details:  ? name   or   name -? )"},
	"help.none":        {JA: "%s: 該当コマンドなし", EN: "%s: no matching command"},

	// NOTES 動的メニューの固定ラベル
	"notes.unread_scan": {JA: "未読スキャン", EN: "unread scan"},
	"notes.board_list":  {JA: "ボード一覧", EN: "board list"},

	// power / kill / shutdown
	"power.started":     {JA: "起動時刻 : %s", EN: "Started : %s"},
	"power.uptime":      {JA: "稼働時間 : %s", EN: "Uptime  : %s"},
	"power.online":      {JA: "在室     : %d / %d", EN: "Online  : %d / %d"},
	"kill.usage":        {JA: "使い方: kill <回線番号|ID>（回線番号は who / ps で確認）", EN: "usage: kill <channel|ID>  (use who / ps to find the channel)"},
	"kill.confirm":      {JA: "%s を切断します。よろしいですか? (y/N) : ", EN: "Disconnect %s? (y/N) : "},
	"kill.fail":         {JA: "切断できません: %v", EN: "cannot disconnect: %v"},
	"kill.done":         {JA: "-- %s を切断しました --", EN: "-- disconnected %s --"},
	"shutdown.confirm":  {JA: "システムを停止します。よろしいですか? (y/N) : ", EN: "Shut down the system? (y/N) : "},
	"shutdown.default":  {JA: "まもなくシステムを停止します。", EN: "The system will shut down shortly."},
	"shutdown.reqd":     {JA: "-- 停止を要求しました --", EN: "-- shutdown requested --"},

	// expert / handle / password / terminal
	"expert.now":     {JA: "expert now = %d (0=beginner, 1=normal, 2=expert)", EN: "expert now = %d (0=beginner, 1=normal, 2=expert)"},
	"prompt.newval":  {JA: "new value: ", EN: "new value: "},
	"expert.set":     {JA: "expert = %d", EN: "expert = %d"},
	"handle.now":     {JA: "handle now = %s", EN: "handle now = %s"},
	"handle.new":     {JA: "new handle: ", EN: "new handle: "},
	"handle.toolong": {JA: "** 長すぎます（全角%d文字/半角%d文字まで） **", EN: "** too long (up to %d full-width / %d half-width chars) **"},
	"handle.set":     {JA: "handle = %s", EN: "handle = %s"},
	"pw.old":         {JA: "old password: ", EN: "old password: "},
	"pw.mismatch":    {JA: "mismatch", EN: "mismatch"},
	"pw.new":         {JA: "new password: ", EN: "new password: "},
	"pw.retype":      {JA: "retype: ", EN: "retype: "},
	"pw.notmatch":    {JA: "not match", EN: "not match"},
	"pw.updated":     {JA: "password updated", EN: "password updated"},
	"term.status":    {JA: "width=%d height=%d color=%v prompt=%q", EN: "width=%d height=%d color=%v prompt=%q"},
	"term.width":     {JA: "width [enter=keep]: ", EN: "width [enter=keep]: "},
	"term.height":    {JA: "height [enter=keep]: ", EN: "height [enter=keep]: "},
	"term.color":     {JA: "color 0/1 [enter=keep]: ", EN: "color 0/1 [enter=keep]: "},
	"term.prompt":    {JA: "prompt [enter=keep]: ", EN: "prompt [enter=keep]: "},
	"term.saved":     {JA: "saved", EN: "saved"},
	"whoami.fallback": {JA: "You are logged in as %s (%s).", EN: "You are logged in as %s (%s)."},

	// SETUP: private / scanlist / regsign
	"priv.header":    {JA: "  会員票（他の利用者には見えません）", EN: "  Member card (private; not shown to others)"},
	"priv.f.name":    {JA: "本名", EN: "Real name"},
	"priv.f.birth":   {JA: "生年月日", EN: "Birthday"},
	"priv.f.addr":    {JA: "住所", EN: "Address"},
	"priv.f.phone":   {JA: "電話", EN: "Phone"},
	"priv.f.comment": {JA: "備考", EN: "Note"},
	"priv.pick":      {JA: "変更する項目番号 (Enter=保存): ", EN: "Item number to edit (Enter=save): "},
	"priv.birthfmt":  {JA: "生年月日 (YYYY/MM/DD) : ", EN: "Birthday (YYYY/MM/DD): "},
	"scan.title":     {JA: "巡回するボード (空なら全ボード)", EN: "Boards to scan (empty = all boards)"},
	"scan.none":      {JA: "  (なし = new は全ボード)", EN: "  (none = new scans all boards)"},
	"scan.add":       {JA: "追加する名前 (Enter=保存  -番号=削除): ", EN: "Name to add (Enter=save  -N=delete): "},
	"scan.full":      {JA: "** これ以上追加できません **", EN: "** cannot add any more **"},
	"sign.cur_none":  {JA: "現在の署名 : (なし)", EN: "Current signature: (none)"},
	"sign.cur":       {JA: "現在の署名 :", EN: "Current signature:"},
	"sign.edit":      {JA: "署名を編集 (矢印で移動。送信は単独行の . / 中止は Ctrl-C / 空のまま . で削除):", EN: "Edit signature (arrows to move; '.' on its own line to submit; Ctrl-C to abort; empty '.' to delete):"},
	"sign.deleted":   {JA: "-- 署名を消しました --", EN: "-- signature cleared --"},
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
