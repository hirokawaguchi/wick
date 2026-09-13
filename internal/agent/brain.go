// Package agent は AgentIO（ホスト内エージェント）の常駐フレームワーク。
// エージェントは人間と同じコマンドループ（mail/chat/notes/電報）を内部パイプ越しに
// 回して自律参加する。頭脳（Brain）は差し替え可能で、当面はスクリプトの偽頭脳。
package agent

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
)

// langOf は可変長引数の先頭言語。空なら既定(ja)へフォールバックする。
func langOf(langs ...i18n.Lang) i18n.Lang {
	if len(langs) > 0 {
		return langs[0]
	}
	return ""
}

// Observation は 1 回の心拍でエージェントが観測する情報。
type Observation struct {
	Screen string    // 直近の画面出力（Drain 済み）
	Tick   int       // 何回目の心拍か（1 始まり）
	Doing  string    // 現在地（MAIN / CHATn など）
	Jobs   []Job     // sys.jobs の未クローズ求人（worker のみ Pilot が観測して渡す）
	Now    time.Time // この心拍の時刻（テスト差し替え用。ゼロなら time.Now()）
	// Audience は「人が見ているか」。チャット部屋なら人間（非エージェント）の
	// 在室者がいるとき true。無人の部屋では AI 同士の会話を止めるために使う
	//（誰も見られず記録も残らないので、トークンを浪費しない）。
	Audience bool
}

// now は観測時刻。ゼロ値なら現在時刻。
func (o Observation) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// Job は sys.jobs 上の 1 件の依頼（未クローズの基文）。
type Job struct {
	Num        int       // ボード内のノート番号（open で選ぶ番号）
	Title      string    // 題
	Author     string    // 依頼者
	Responders []string  // これまでにレスした著者（引受け済み判定に使う）
	Updated    time.Time // 最終更新（最後のレス時刻。再割り当て判定に使う）
}

// Brain は「次に何を打つか」を決める頭脳。実モデルはこの実装を差し替える。
// script は人間と同じキー列（コマンド文字列）。done=true で離脱。
type Brain interface {
	Next(obs Observation) (script string, done bool)
}

// scriptedBrain は決め打ちの手順を 1 心拍 1 手ずつ返す偽頭脳。
type scriptedBrain struct {
	steps []string
	i     int
}

func (b *scriptedBrain) Next(Observation) (string, bool) {
	if b.i >= len(b.steps) {
		return "", true // 手順を打ち切ったら離脱（在室のまま idle にはしない）
	}
	s := b.steps[b.i]
	b.i++
	return s, false
}

// conversantBrain は画面を観測して分岐する反応型の偽頭脳（UC13 の土台）。
// 部屋に入って居続け、他者の発言に反応し、沈黙が続けば控えめに一言振る。
// 「観測→判断」の最小形。実モデルはこの Next を差し替えるだけでよい。
type conversantBrain struct {
	selfID  string
	room    int
	lang    i18n.Lang
	isAgent func(id string) bool // 相手がエージェントか（nil なら全員人間扱い）

	entered        bool
	silence        int // 連続沈黙の心拍数
	silenceLimit   int // これを超えたら一言振る
	fillerStreak   int // 誰も居ない/黙っている間に自分から振った連続回数
	maxFillers     int // 空き部屋で自分だけ喋り続けないための上限
	agentStreak    int // エージェント相手に連続で相づちを打った回数
	maxAgentReacts int // エージェント相手の相づち上限（無限ループ防止）
}

func (b *conversantBrain) Next(obs Observation) (string, bool) {
	if !b.entered {
		b.entered = true
		return fmt.Sprintf("chat %d\n", b.room), false
	}
	// 他者の発言があれば反応する。ただし相手もエージェントのときは数回で打ち止め、
	// 以降は人間の発言があるまで黙る（AI 同士が「了解です」を延々往復して、引用が
	// 入れ子に肥大するのを防ぐ）。
	if who, text := lastOtherUtterance(obs.Screen, b.selfID); who != "" {
		human := b.isAgent == nil || !b.isAgent(who)
		if human {
			b.silence, b.fillerStreak, b.agentStreak = 0, 0, 0
			return reactLine(who, text, b.lang) + "\n", false
		}
		if b.agentStreak < b.maxAgentReacts {
			b.agentStreak++
			b.silence = 0
			return reactLine(who, text, b.lang) + "\n", false
		}
		// 打ち止め。反応せず、下の沈黙処理（控えめな話題振り）に任せる。
	}
	// 沈黙が続けば控えめに一言。ただし連投で埋め尽くさない。
	b.silence++
	if b.silence >= b.silenceLimit && b.fillerStreak < b.maxFillers {
		b.silence = 0
		b.fillerStreak++
		return fillerLine(b.fillerStreak, b.lang) + "\n", false
	}
	return "", false // まだ黙っている（在室のまま。心拍は進む）
}

// lastOtherUtterance は画面から「<id>> 本文」形式の、自分以外の最新発言を拾う。
func lastOtherUtterance(screen, selfID string) (who, text string) {
	for _, ln := range strings.Split(screen, "\n") {
		ln = strings.TrimRight(ln, "\r")
		i := strings.Index(ln, "> ")
		if i <= 0 {
			continue
		}
		id := ln[:i]
		if !isIDToken(id) || strings.EqualFold(id, selfID) {
			continue
		}
		who, text = id, strings.TrimSpace(ln[i+2:])
	}
	return who, text
}

// screenHasHumanSpeech は画面に「人間（＝エージェント以外）の <id>> 発言」があるか。
// テンポ監督が「人が来たら AI は譲る」を判断するために使う。
func screenHasHumanSpeech(screen string, isAgent func(id string) bool) bool {
	for _, ln := range strings.Split(screen, "\n") {
		ln = strings.TrimRight(ln, "\r")
		i := strings.Index(ln, "> ")
		if i <= 0 {
			continue
		}
		id := ln[:i]
		if !isIDToken(id) {
			continue
		}
		if isAgent == nil || !isAgent(id) {
			return true
		}
	}
	return false
}

func isIDToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func reactLine(who, text string, langs ...i18n.Lang) string {
	// 相手の発言が相づち（「…」了解です。）だった場合に引用が入れ子に肥大しないよう、
	// 鉤括弧・引用符を落としてから短く引用する。
	t := strings.NewReplacer(
		"「", "", "」", "", "『", "", "』", "",
		"\"", "", "“", "", "”", "",
	).Replace(strings.TrimSpace(text))
	t = clipRunes(strings.TrimSpace(t), 16)
	return i18n.T(langOf(langs...), "agent.react", who, t)
}

func fillerLine(streak int, langs ...i18n.Lang) string {
	keys := []string{"agent.fill.1", "agent.fill.2", "agent.fill.3"}
	lang := langOf(langs...)
	return i18n.T(lang, keys[(streak-1)%len(keys)])
}

// workerBrain は sys.jobs を観測し、未着手の依頼をコマンド経路で引き受ける（UC11/UC7）。
// 観測は Store 読取（Pilot が obs.Jobs に載せる）、書き込みは open→w の人間コマンド。
type workerBrain struct {
	selfID       string
	handle       string
	lang         i18n.Lang
	acted        map[int]bool  // このセッションで着手済みのノート番号（多重着手防止）
	reclaimAfter time.Duration // 他者が引き受けたまま滞留したら再割り当てするまでの時間（0 で無効）
}

func (b *workerBrain) Next(obs Observation) (string, bool) {
	if b.acted == nil {
		b.acted = map[int]bool{}
	}
	now := obs.now()
	for _, j := range obs.Jobs {
		if b.acted[j.Num] {
			continue
		}
		if containsFold(j.Responders, b.selfID) {
			b.acted[j.Num] = true // 既に自分が応答済み
			continue
		}
		if len(j.Responders) > 0 {
			// 誰かが引き受けている。滞留していれば引き取る（タイムアウト再割り当て）。
			if b.reclaimAfter <= 0 || now.Sub(j.Updated) < b.reclaimAfter {
				continue // まだ動いている（新鮮）。触らず次の求人へ
			}
			b.acted[j.Num] = true
			body := workerReclaim(b.selfID, j.Title, b.lang)
			return fmt.Sprintf("open sys.jobs\n%d\nw\n%s\n.\nq", j.Num, body), false
		}
		// 未着手の依頼を引き受ける。
		b.acted[j.Num] = true
		body := workerReply(b.selfID, j.Title, b.lang)
		// open sys.jobs → 番号選択 → w(レス) → 本文 → 単独 . → q
		return fmt.Sprintf("open sys.jobs\n%d\nw\n%s\n.\nq", j.Num, body), false
	}
	return "", false // 求人待ち（在室のまま idle）
}

// workerReclaim は滞留した依頼を引き取るときのレス文。
func workerReclaim(id, title string, langs ...i18n.Lang) string {
	return i18n.T(langOf(langs...), "agent.worker.reclaim", id)
}

// workerReply は役割（ID）に応じた引受けレスを返す。実モデルはここで実際に調べて書く。
func workerReply(id, title string, langs ...i18n.Lang) string {
	lang := langOf(langs...)
	switch id {
	case "scout":
		return i18n.T(lang, "agent.worker.scout")
	case "critic":
		return i18n.T(lang, "agent.worker.critic")
	case "writer":
		return i18n.T(lang, "agent.worker.writer")
	default:
		return i18n.T(lang, "agent.worker.take", id)
	}
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// buildBrain は Spec の behavior から偽頭脳を組み立てる。
func buildBrain(sp Spec, id, handle string, langs ...i18n.Lang) Brain {
	lang := langOf(langs...)
	switch sp.Behavior {
	case "worker":
		ra := sp.Interval
		if ra <= 0 {
			ra = 90 * time.Second // 既定: 90 秒滞留したら再割り当て
		}
		return &workerBrain{selfID: id, handle: handle, lang: lang, acted: map[int]bool{}, reclaimAfter: ra}
	case "poster":
		return newPosterBrain(sp, id, handle, lang)
	case "conversant":
		room := sp.Room
		if room <= 0 {
			room = 1
		}
		return &conversantBrain{
			selfID:         id,
			room:           room,
			lang:           lang,
			silenceLimit:   3,
			maxFillers:     3,
			maxAgentReacts: 2,
		}
	case "chatter":
		room := sp.Room
		if room <= 0 {
			room = 1
		}
		rs := strconv.Itoa(room)
		return &scriptedBrain{steps: []string{
			"chat " + rs + "\n", // 入室（以後は部屋にとどまる）
			i18n.T(lang, "agent.chatter.hi", handle) + "\n",
			i18n.T(lang, "agent.chatter.topic") + "\n",
			".\n", // 退室
		}}
	default: // "greeter": 電報→メール→ノートの順に全チャネルを触る
		target := sp.Target
		if target == "" {
			target = "sysop"
		}
		board := sp.Board
		if board == "" {
			board = "junk.test"
		}
		return &scriptedBrain{steps: []string{
			// 電報（MAIN で 1 行）
			fmt.Sprintf("! %s %s\n", target, i18n.T(lang, "agent.greeter.tg", handle)),
			// メール（題→本文→単独 . で送信）
			fmt.Sprintf("postmail %s\n%s\n%s\n.\n", target, i18n.T(lang, "agent.greeter.msubj"), i18n.T(lang, "agent.greeter.mbody", handle)),
			// ノート（open→INDEX で w＝新規ベース→題→本文→単独 . →q で退出）
			fmt.Sprintf("open %s\nw%s\n%s\n.\nq", board, i18n.T(lang, "agent.greeter.ntitle", handle), i18n.T(lang, "agent.greeter.nbody")),
		}}
	}
}

// splitKV は "k=v k=v" を map に。AGENTS.txt のパラメータ用。
func splitKV(fields []string) map[string]string {
	m := map[string]string{}
	for _, f := range fields {
		if i := strings.IndexByte(f, '='); i > 0 {
			m[strings.ToLower(f[:i])] = f[i+1:]
		}
	}
	return m
}
