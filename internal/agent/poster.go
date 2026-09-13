package agent

import (
	"fmt"
	"strings"
	"time"
)

// posterBrain は「自発的にベースノートを立てる」頭脳（UC の自走ノート）。
// 2 種類のきっかけで動く:
//   - 時刻起点: 前回投稿から interval 以上経ったら、話題リストから 1 本立てる。
//   - 興味起点: 画面に interests のキーワードが現れたら、その話題で 1 本立てる
//     （ただし cooldown 中は連投しない）。
//
// 部屋には入らず MAIN に居続け、open→w でボードに書く（人間と同じコマンド経路）。
type posterBrain struct {
	selfID    string
	handle    string
	board     string
	interval  time.Duration // 時刻起点の最小間隔
	interests []string      // 興味キーワード（興味起点のトリガ）

	cfg       ModelConfig // web 研究の要否判断に使う（web 有効時のみ設定）
	web       *webBroker  // web 検索（任意。nil または未接続なら使わない）
	webBudget int         // 残り検索回数

	lastPost time.Time // 直近に投稿した時刻（cooldown/間隔の基準）
	topicN   int       // 話題ローテーションの位置

	// gen が非 nil なら、題・本文をモデルに書かせる（意味のある面白い内容）。
	// webCtx が非空なら web 検索結果を渡す。失敗時は定型文へフォールバックする。
	gen func(topic, webCtx string) (title, body string, err error)
}

func newPosterBrain(sp Spec, id, handle string) *posterBrain {
	board := sp.Board
	if board == "" {
		board = "junk.test"
	}
	iv := sp.Interval
	if iv <= 0 {
		iv = 10 * time.Minute // 既定: 10 分に 1 本まで
	}
	return &posterBrain{
		selfID:    id,
		handle:    handle,
		board:     board,
		interval:  iv,
		interests: sp.Interests,
	}
}

func (b *posterBrain) Next(obs Observation) (string, bool) {
	now := obs.now()
	if b.lastPost.IsZero() {
		b.lastPost = now // 起動直後は基準時刻をセット（すぐには投稿しない）
		return "", false
	}
	// cooldown 中は何もしない（時刻・興味とも投稿間隔を守る）。
	if now.Sub(b.lastPost) < b.interval {
		return "", false
	}
	// 興味起点: 画面に興味キーワードがあれば、その話題で立てる。
	if kw := firstInterest(obs.Screen, b.interests); kw != "" {
		b.lastPost = now
		title, body := b.composeInterest(kw)
		return b.postScript(title, body), false
	}
	// 時刻起点: 話題リストから 1 本立てる。
	title, body := b.composeTopic()
	b.lastPost = now
	return b.postScript(title, body), false
}

// composeInterest は興味キーワードからノートの題・本文を作る（gen 優先、失敗で定型）。
func (b *posterBrain) composeInterest(kw string) (string, string) {
	if b.gen != nil {
		topic := "「" + kw + "」について"
		webCtx := research(b.cfg, b.web, &b.webBudget, b.selfID, topic)
		if t, body, err := b.gen(topic, webCtx); err == nil {
			return t, body
		}
	}
	return interestTitle(b.handle, kw), interestBody(b.handle, kw)
}

// composeTopic は時刻起点のノートの題・本文を作る（gen 優先、失敗で定型ローテ）。
func (b *posterBrain) composeTopic() (string, string) {
	if b.gen != nil {
		seed := posterSeeds()[b.topicN%len(posterSeeds())]
		b.topicN++
		webCtx := research(b.cfg, b.web, &b.webBudget, b.selfID, seed)
		if t, body, err := b.gen(seed, webCtx); err == nil {
			return t, body
		}
		// 失敗したので定型のローテ位置は進めた状態のまま定型へ。
	}
	return b.nextTopic()
}

// posterSeeds はモデルに振る話題の種（ローテーション）。
func posterSeeds() []string {
	return []string{
		"最近のちょっとした発見",
		"季節の移ろいや空模様",
		"おすすめしたい一冊や一曲",
		"暮らしの小さな工夫",
		"ふと浮かんだ問いかけ",
	}
}

// postScript は open→w→題→本文→単独 . →q のキー列を返す。
func (b *posterBrain) postScript(title, body string) string {
	return fmt.Sprintf("open %s\nw%s\n%s\n.\nq", b.board, title, body)
}

func (b *posterBrain) nextTopic() (title, body string) {
	topics := posterTopics(b.handle)
	t := topics[b.topicN%len(topics)]
	b.topicN++
	return t.title, t.body
}

type topic struct{ title, body string }

func posterTopics(handle string) []topic {
	return []topic{
		{handle + " の覚え書き: 今日の話題", "ふと気になったことを置いておきます。よかったらレスで続けてください。\n"},
		{handle + " のメモ: 最近読んだもの", "面白かった一節を共有します。感想があれば教えてください。\n"},
		{handle + " の問いかけ: みなさんはどう？", "ひとつ問いを立ててみます。気軽に意見をどうぞ。\n"},
	}
}

// firstInterest は画面テキストに含まれる最初の興味キーワードを返す。
func firstInterest(screen string, interests []string) string {
	if screen == "" || len(interests) == 0 {
		return ""
	}
	low := strings.ToLower(screen)
	for _, kw := range interests {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(kw)) {
			return kw
		}
	}
	return ""
}

func interestTitle(handle, kw string) string {
	return fmt.Sprintf("%s の反応: 「%s」について", handle, clipRunes(kw, 24))
}

func interestBody(handle, kw string) string {
	return fmt.Sprintf("「%s」の話題が出ていたので、一本立てておきます。詳しい人はレスをください。\n", clipRunes(kw, 40))
}
