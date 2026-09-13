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

	// rogue（内部ゲーム。入口コマンドは Wick 追加）
	"rogue.too_small":  {JA: "** 画面が狭すぎます（80x24 以上が必要）。terminal で設定してください **", EN: "** screen too small (need at least 80x24). Set it with terminal **"},
	"rogue.score_fail": {JA: "スコアを読み出せませんでした。", EN: "Could not read the scores."},
	"rogue.no_scores":  {JA: "まだ記録がありません。", EN: "No scores yet."},
	"rogue.score_head": {JA: " 順  冒険者           金塊  最深階  結末", EN: " No  Adventurer        Gold  Depth  Result"},
	"rogue.depth_unit": {JA: "地下", EN: "L"},
	"rogue.result_won": {JA: "魔除けを持ち帰り生還！", EN: "escaped with the Amulet!"},

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

	// notes: 読み書き画面（internal/notes）
	"notes.guide":         {JA: "[ i=一覧  q=抜ける  ?=ヘルプ ]", EN: "[ i=list  q=quit  ?=help ]"},
	"notes.search_kw":     {JA: "検索語 : ", EN: "Search: "},
	"notes.search_none":   {JA: "-- \"%s\" は見つかりません --", EN: "-- \"%s\" not found --"},
	"notes.search_hits":   {JA: "-- \"%s\" : %d 件 --", EN: "-- \"%s\" : %d hit(s) --"},
	"notes.open_num":      {JA: "開く番号 (Enter で戻る) : ", EN: "Number to open (Enter to go back): "},
	"notes.seq_current":   {JA: "現在の未読基準 : %s", EN: "Current unread marker: %s"},
	"notes.seq_prompt":    {JA: "この時刻以降を未読に (YYYY/MM/DD HH:MM:SS, Enter で中止) : ", EN: "Mark unread since (YYYY/MM/DD HH:MM:SS, Enter to cancel): "},
	"notes.seq_set":       {JA: "-- %s 以降を未読とします（l/TAB で回収）--", EN: "-- marked unread since %s (l/TAB to fetch) --"},
	"notes.no_sign":       {JA: "-- 看板はありません --", EN: "-- no board sign --"},
	"notes.sign_head":     {JA: "=== %s の看板 ===", EN: "=== sign of %s ==="},
	"notes.deleted":       {JA: "** 削除されています **", EN: "** deleted **"},
	"notes.index_footer":  {JA: "-- %d..%d / %d  (Space=古 BS=新 =最新 *最古 f=検索 k=看板) --", EN: "-- %d..%d / %d  (Space=older BS=newer ==latest *=oldest f=find k=sign) --"},
	"notes.write_denied":  {JA: "** 書き込めません **", EN: "** cannot write here **"},
	"notes.write_done":    {JA: "-- 書き込み完了 --", EN: "-- posted --"},
	"notes.body_prompt":   {JA: "本文 (矢印で移動して修正可。送信は単独行の . / 中止は Ctrl-C):", EN: "Body (arrows to edit; '.' on its own line to submit; Ctrl-C to abort):"},
	"notes.del_num":       {JA: "削除する番号 [%d]: ", EN: "Number to delete [%d]: "},
	"notes.only_own_del":  {JA: "** 自分の書き込みだけ削除できます **", EN: "** you can only delete your own posts **"},
	"notes.changed":       {JA: "-- 変更しました --", EN: "-- changed --"},
	"notes.confirm_set":   {JA: "%s 設定しますか [Y/n]: ", EN: "Set %s? [Y/n]: "},
	"notes.confirm_clear": {JA: "%s 解除しますか [Y/n]: ", EN: "Clear %s? [Y/n]: "},
	"notes.change_label":  {JA: "変更", EN: "change"},
	"notes.query_end":     {JA: "-- 続きなし --  (Enter/Space/>) 次レス  (l/TAB) 未読  (BS) 戻る  (^D) 終了 : ", EN: "-- no more --  (Enter/Space/>) next res  (l/TAB) unread  (BS) back  (^D) quit : "},
	"notes.no_more_res":   {JA: "次のレスはありません", EN: "no more responses"},
	"notes.unread_normal": {JA: "-- 未読設定を通常に戻しました --", EN: "-- unread setting back to normal --"},
	"notes.all_unread":    {JA: "-- 全メッセージを未読とします --", EN: "-- marking all messages unread --"},
	"notes.note_all_unread": {JA: "-- このノートの全メッセージを未読とします --", EN: "-- marking all messages in this note unread --"},
	"notes.basenote_num":  {JA: "Basenote番号 : ", EN: "Basenote number: "},
	"notes.response_num":  {JA: "Response番号 : ", EN: "Response number: "},

	// notes: 一覧・設定・板編集（internal/command/notes.go）
	"notes.bbslist_head":   {JA: "掲示板一覧  ( R=読む  W=書き込み  B=新規話題を立てる )", EN: "Board list  ( R=read  W=write  B=new topic )"},
	"notes.no_readable":    {JA: "読めるボードがありません。", EN: "No readable boards."},
	"notes.seq_cur_val":    {JA: "現在の設定値 : %s", EN: "Current value: %s"},
	"notes.seq_input_hint": {JA: "変更値を入力してください。 (YYYY/MM/DD HH:MM:SS)", EN: "Enter new value. (YYYY/MM/DD HH:MM:SS)"},
	"notes.seq_input":      {JA: "変更値       : ", EN: "New value  : "},
	"notes.seq_changed":    {JA: "-- %s に変更しました --", EN: "-- changed to %s --"},
	"notes.board_item":     {JA: "変更する項目番号 : ", EN: "Item number to change: "},
	"notes.board_save_q":   {JA: "書き込みますか [Y/n]: ", EN: "Save? [Y/n]: "},
	"notes.discarded":      {JA: "-- 破棄しました --", EN: "-- discarded --"},
	"notes.save_failed":    {JA: "** 書き込みに失敗しました **", EN: "** save failed **"},
	"notes.sign_edit":      {JA: "看板を編集 (矢印で移動。送信は単独行の . / 中止は Ctrl-C / 空のまま . で消去):", EN: "Edit sign (arrows to move; '.' on its own line to submit; Ctrl-C to abort; empty then '.' to clear):"},

	// mail: メール（internal/command/mail.go）
	"mail.inbox":         {JA: "受信箱", EN: "Inbox"},
	"mail.sent":          {JA: "送信控え", EN: "Sent"},
	"mail.verb_delete":   {JA: "削除", EN: "Delete"},
	"mail.verb_withdraw": {JA: "撤回", EN: "Withdraw"},
	"mail.guide":         {JA: "[ 番号=読む  i=一覧  q=抜ける  ?=ヘルプ ]", EN: "[ number=read  i=list  q=quit  ?=help ]"},
	"mail.list_footer":   {JA: "番号で読む。dir 再表示。q で抜ける。", EN: "Read by number. dir to refresh. q to quit."},
	"mail.pick_prompt":   {JA: "番号 (%s)  [ i=一覧  q=終了 ]: ", EN: "Number (%s)  [ i=list  q=quit ]: "},
	"mail.no_user":       {JA: "** そのユーザーはいません **", EN: "** no such user **"},
	"mail.multi_dest":    {JA: "宛先 (空白区切り / @グループ) : ", EN: "To (space-separated / @group): "},
	"mail.deleted":       {JA: "-- 削除しました --", EN: "-- deleted --"},
	"mail.withdrawn":     {JA: "-- 撤回しました --", EN: "-- withdrawn --"},
	"mail.cant_withdraw": {JA: "** 既読のため撤回できません **", EN: "** already read; cannot withdraw **"},
	"mail.save_now":      {JA: "送信控え now = %s", EN: "Sent-copy now = %s"},
	"mail.onoff_prompt":  {JA: "on / off (Enter=そのまま): ", EN: "on / off (Enter=keep): "},
	"mail.save_on":       {JA: "送信控え = on", EN: "Sent-copy = on"},
	"mail.save_off":      {JA: "送信控え = off", EN: "Sent-copy = off"},
	"mail.groups_head":   {JA: "同報グループ", EN: "Broadcast groups"},
	"mail.groups_none":   {JA: "  (なし)", EN: "  (none)"},
	"mail.group_del":     {JA: "名前 (Enter=終了  -番号|-名前=削除): ", EN: "Name (Enter=quit  -num|-name=delete): "},
	"mail.group_cur":     {JA: "現在: %s", EN: "Current: %s"},
	"mail.group_members": {JA: "メンバー (空白区切り): ", EN: "Members (space-separated): "},
	"mail.group_max":     {JA: "** グループは 8 までです **", EN: "** up to 8 groups **"},
	"mail.group_missing": {JA: "** グループ @%s はありません **", EN: "** no such group @%s **"},
	"mail.dest":          {JA: "宛先 : ", EN: "To: "},
	"mail.compose_subj":  {JA: "題 (Ctrl-C 中止) : ", EN: "Subject (Ctrl-C to abort): "},
	"mail.untitled":      {JA: "(無題)", EN: "(untitled)"},
	"mail.no_such_user":  {JA: "** %s はいません **", EN: "** %s does not exist **"},
	"mail.inbox_full":    {JA: "** %s の受信箱がいっぱいです **", EN: "** %s's inbox is full **"},
	"mail.sent_to":       {JA: "-- %s へ送信しました --", EN: "-- sent to %s --"},

	// news: ニュース（internal/command/news.go）
	"news.unread_heads":    {JA: "[ 未読の見出し ]", EN: "[ Unread headlines ]"},
	"news.heads_fail":      {JA: "** 一覧を取得できません **", EN: "** cannot fetch list **"},
	"news.group_heads":     {JA: "[ %s の見出し ]", EN: "[ Headlines of %s ]"},
	"news.guide":           {JA: "[ Enter=次  i=一覧  q=抜ける(保存)  x=保存せず  ?=ヘルプ ]", EN: "[ Enter=next  i=list  q=quit(save)  x=quit(no save)  ?=help ]"},
	"news.guide2":          {JA: "[ Enter=次  i=一覧  q=抜ける  ?=ヘルプ ]", EN: "[ Enter=next  i=list  q=quit  ?=help ]"},
	"news.unsubscribed":    {JA: "-- %s を購読解除しました --", EN: "-- unsubscribed from %s --"},
	"news.no_dest":         {JA: "** 宛先がありません **", EN: "** no recipient **"},
	"news.no_author":       {JA: "** 著者はいません **", EN: "** author does not exist **"},
	"news.subj":            {JA: "題 (Ctrl-C 中止) : ", EN: "Subject (Ctrl-C to abort): "},
	"news.subj_re":         {JA: "題 [%s] (Ctrl-C 中止): ", EN: "Subject [%s] (Ctrl-C to abort): "},
	"news.group_prompt":    {JA: "グループ [%s]: ", EN: "Group [%s]: "},
	"news.too_many_groups": {JA: "** グループが多すぎます **", EN: "** too many groups **"},
	"news.untitled":        {JA: "(無題)", EN: "(untitled)"},
	"news.posted":          {JA: "-- %s:%d に投稿しました --", EN: "-- posted to %s:%d --"},

	// talk: リアルタイム会議室（internal/command/talk.go）
	"talk.no_unread":     {JA: "-- 未読なし --", EN: "-- no unread --"},
	"talk.scan_end":      {JA: "-- talk 終わり --", EN: "-- end of talk --"},
	"talk.no_room":       {JA: "** その部屋はありません **", EN: "** no such room **"},
	"talk.enter":         {JA: "-- 入室 --  %d: %s [%s]  (%s)", EN: "-- entered --  %d: %s [%s]  (%s)"},
	"talk.guide":         {JA: "[ /i=一覧  /q=退出  /?=ヘルプ ]  (/who /title /open /close /knock /ra)", EN: "[ /i=list  /q=leave  /?=help ]  (/who /title /open /close /knock /ra)"},
	"talk.leave":         {JA: "-- 退室 --", EN: "-- left --"},
	"talk.unknown":       {JA: "** 不明なコマンド **  [ /i=一覧  /q=退出  /?=ヘルプ ]", EN: "** unknown command **  [ /i=list  /q=leave  /?=help ]"},
	"talk.no_seat":       {JA: "** 座席がありません。/knock してください **", EN: "** no seat. use /knock **"},
	"talk.too_many":      {JA: "** これ以上書けません **", EN: "** cannot post more **"},
	"talk.send_fail":     {JA: "** 送れません **", EN: "** cannot send **"},
	"talk.already_seated": {JA: "-- すでに着席しています --", EN: "-- already seated --"},
	"talk.knock_fail":    {JA: "** ノックできません **", EN: "** cannot knock **"},
	"talk.knocked":       {JA: "-- ノックしました --", EN: "-- knocked --"},
	"talk.not_leader":    {JA: "** リーダーではありません **", EN: "** you are not the leader **"},
	"talk.no_person":     {JA: "** その人はいません **", EN: "** no such person **"},
	"talk.seated":        {JA: "-- %s を着席させました --", EN: "-- seated %s --"},
	"talk.no_self_kick":  {JA: "** 自分はキックできません **", EN: "** cannot kick yourself **"},
	"talk.kicked":        {JA: "-- %s を退席させました --", EN: "-- removed %s --"},
	"talk.title_set":     {JA: "-- 題を %s にしました --", EN: "-- title set to %s --"},
	"talk.status_set":    {JA: "-- %s にしました --", EN: "-- set to %s --"},
	"talk.ra_warn":       {JA: "** /ra は全件表示です (%d lines) **", EN: "** /ra shows all lines (%d lines) **"},
	"talk.continue_q":    {JA: "続行しますか (y/N): ", EN: "Continue? (y/N): "},
	"talk.no_log":        {JA: "-- ログなし --", EN: "-- no log --"},
	"talk.recent":        {JA: "-- 直近 --", EN: "-- recent --"},
	"talk.role_seat":     {JA: "座席", EN: "seat"},
	"talk.role_knock":    {JA: "ノック", EN: "knock"},
	"talk.role_watch":    {JA: "見学", EN: "watch"},
	"talk.usage":         {JA: "usage: talk [-n] [部屋番号]", EN: "usage: talk [-n] [room]"},

	// social: 電報・チャット（internal/command/social.go）
	"social.tg_usage":      {JA: "usage: ! <id|チャネル> <message>", EN: "usage: ! <id|channel> <message>"},
	"social.body_prompt":   {JA: "本文 : ", EN: "Body: "},
	"social.no_channel":    {JA: "** そのチャネルには誰もいません **", EN: "** nobody in that channel **"},
	"social.offline":       {JA: "** そのユーザーはオンラインではありません **", EN: "** that user is not online **"},
	"social.sent":          {JA: "-- 送信しました --", EN: "-- sent --"},
	"social.chat_usage":    {JA: "usage: chat [部屋番号]", EN: "usage: chat [room]"},
	"social.chat_enter":    {JA: "-- 入室 --  %d: %s  [ /i=一覧  /q=退出  /?=ヘルプ ]  (/who /title /! id 本文 /e)", EN: "-- entered --  %d: %s  [ /i=list  /q=leave  /?=help ]  (/who /title /! id body /e)"},
	"social.tg_room_usage": {JA: "usage: /! <id> <本文>", EN: "usage: /! <id> <body>"},
	"social.title_usage":   {JA: "usage: /title <題>", EN: "usage: /title <title>"},
	"social.cant_change":   {JA: "** 変更できません **", EN: "** cannot change **"},
	"social.room_prompt":   {JA: "部屋番号 (Enter=再表示  .=中止): ", EN: "Room number (Enter=refresh  .=abort): "},

	// signup / useredit: 新規登録・会員編集（internal/command/signup.go）
	"signup.head":         {JA: "== 新規登録 ==", EN: "== Sign up =="},
	"signup.intro1":       {JA: "ログイン ID は登録順に自動発行します（変更・指定はできません）。", EN: "Your login ID is issued automatically in order (you cannot choose it)."},
	"signup.intro2":       {JA: "画面に出る名前は「ハンドル」で自由に決められます。", EN: "The name shown on screen is your \"handle\", which you choose freely."},
	"signup.handle_prompt": {JA: "ハンドル : ", EN: "Handle: "},
	"signup.abort":        {JA: "-- 中止 --", EN: "-- aborted --"},
	"signup.busy":         {JA: "** 登録が混み合っています。少し待って再度お試しください **", EN: "** sign-up is busy; please wait and try again **"},
	"signup.done":         {JA: "-- 登録しました --", EN: "-- registered --"},
	"signup.your_id":      {JA: "あなたのログイン ID : %s   （ハンドル: %s）", EN: "Your login ID: %s   (handle: %s)"},
	"signup.note1":        {JA: "次回からはこの ID でログインしてください（メモをお願いします）。", EN: "Please log in with this ID next time (write it down)."},
	"signup.note2":        {JA: "いまはまだ見習い会員です。sysop が承認すると書き込めます。", EN: "You are still a probationary member; you can post once sysop approves you."},
	"signup.note3":        {JA: "いったん切断し、新しい ID で繋ぎ直してください。", EN: "Please disconnect and reconnect with your new ID."},
	"signup.pw_prompt":    {JA: "パスワード (4〜16 文字) : ", EN: "Password (4-16 chars): "},
	"signup.pw_again":     {JA: "もう一度 : ", EN: "Again: "},
	"signup.pw_short":     {JA: "** 短すぎます **", EN: "** too short **"},
	"signup.pw_mismatch":  {JA: "** 一致しません **", EN: "** does not match **"},
	"signup.id_prompt":    {JA: "ID : ", EN: "ID: "},
	"signup.edit_item":    {JA: "変更する項目番号 (Enter=保存して終了) : ", EN: "Item number to change (Enter=save and quit): "},
	"signup.tlimit_prompt": {JA: "time limit (分, 65535=無制限) : ", EN: "time limit (min, 65535=unlimited): "},
	"level.gen":           {JA: "一般", EN: "general"},
	"level.pro":           {JA: "見習い", EN: "probationary"},
	"level.gst":           {JA: "ゲスト", EN: "guest"},

	// pprof: 公開プロフィール（internal/command/pprof.go）
	"pprof.cur":          {JA: "現在の公開プロフィール :", EN: "Current public profile:"},
	"pprof.edit":         {JA: "公開文を編集 (矢印で移動。送信は単独行の . / 中止は Ctrl-C / 空のまま . で削除):", EN: "Edit profile (arrows to move; '.' on its own line to submit; Ctrl-C to abort; empty then '.' to clear):"},
	"pprof.deleted":      {JA: "-- プロフィールを消しました --", EN: "-- profile cleared --"},
	"pprof.none":         {JA: "(公開プロフィールはありません)", EN: "(no public profiles)"},
	"pprof.keyword":      {JA: "キーワード : ", EN: "Keyword: "},
	"pprof.search_usage": {JA: "usage: searchprof [語]", EN: "usage: searchprof [word]"},
	"pprof.no_hit":       {JA: "ヒットなし", EN: "no hits"},

	// agent: エージェント制御（internal/command/agent.go, sysop）
	"agent.disabled":     {JA: "** エージェント機能が無効です **", EN: "** agent feature is disabled **"},
	"agent.start_fail":   {JA: "起動できません: %v", EN: "cannot start: %v"},
	"agent.started":      {JA: "-- %s を起動しました --", EN: "-- started %s --"},
	"agent.stopped":      {JA: "-- %s を停止しました --", EN: "-- stopped %s --"},
	"agent.not_running":  {JA: "-- %s は動いていません --", EN: "-- %s is not running --"},
	"agent.usage_head":   {JA: "使い方:", EN: "usage:"},
	"agent.usage_list":   {JA: "  agent            一覧", EN: "  agent            list"},
	"agent.usage_start":  {JA: "  agent start <id> 起動", EN: "  agent start <id> start"},
	"agent.usage_stop":   {JA: "  agent stop <id>  停止", EN: "  agent stop <id>  stop"},
	"agent.none":         {JA: "-- エージェントは登録されていません --", EN: "-- no agents registered --"},
	"agent.col_state":    {JA: "状態", EN: "State"},
	"agent.state_stopped": {JA: "停止", EN: "stopped"},
	"agent.state_running": {JA: "稼働", EN: "running"},

	// 通知（internal/session/notice.go）。受信者の表示言語で出す。
	"notice.telegram":       {JA: "** 電報 from %s (%s) %s **", EN: "** telegram from %s (%s) %s **"},
	"notice.telegram_short": {JA: "電報 %s: %s", EN: "telegram %s: %s"},
	"notice.join":     {JA: "-- %s が入室 --", EN: "-- %s entered --"},
	"notice.leave":    {JA: "-- %s が退室 --", EN: "-- %s left --"},
	"notice.knock":    {JA: "-- %s がノック --", EN: "-- %s knocked --"},
	"notice.admit":    {JA: "-- %s が着席 --", EN: "-- %s seated --"},
	"notice.system":   {JA: "** システム: %s **", EN: "** system: %s **"},

	// ログイン直後・ゲスト口（internal/shell）
	"login.unread_mail": {JA: "メールが %d 通あります。", EN: "You have %d unread mail message(s)."},
	"login.unread_news": {JA: "ニュースが %d 本あります。", EN: "You have %d unread news article(s)."},
	"guest.menu":        {JA: "[1] 新規登録 (signup)\n[2] 終了     (off)\n", EN: "[1] Sign up (signup)\n[2] Quit     (off)\n"},
	"guest.only":        {JA: "使えるのは signup と off だけです。", EN: "Only signup and off are available."},

	// SSH（認証前バナー・重複ログイン・停止告知）
	"sshd.banner":   {JA: "Wick\n初めての方は ID に guest（パスワード guest）で入り、signup で登録してください。\n会員の方はご自分の ID でログインを。\n", EN: "Wick\nNew here? Log in as guest (password: guest) and run signup to register.\nMembers, please log in with your own ID.\n"},
	"sshd.dup_id":   {JA: "## 使用中ID ##", EN: "## ID in use ##"},
	"sshd.stopping": {JA: "局を停止します。切断します。", EN: "The station is shutting down. Disconnecting."},

	// who / power / kill / 外部リンク警告 / ? の別名行
	"who.kind_human":    {JA: "人", EN: "human"},
	"power.uptime_days": {JA: "%d日 %02d:%02d:%02d", EN: "%dd %02d:%02d:%02d"},
	"host.killed":       {JA: "sysop により切断されました。", EN: "Disconnected by sysop."},
	"warn.external":     {JA: "※ 外部サイトからのDLです。提供元を確認できないファイルは開かないでください。", EN: "* External download. Do not open files from an unverified source."},
	"help.alias":        {JA: "%s（%s の別名）", EN: "%s (alias of %s)"},
	"help.alias_only":   {JA: "%s の別名", EN: "alias of %s"},
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
