package rogue

import (
	"math/rand"
	"strings"
	"time"
)

// Game はゲーム 1 局の状態。大文字始まりのフィールドはセーブ（gob）に載る。
// io / rng / msgs は一時状態でセーブしない（ロード時に付け直す）。
type Game struct {
	Depth    int // 現在の階
	MaxDepth int // 到達した最深階
	Player   Player
	Cells    [][]Cell
	Rooms    []Room
	Monsters []*Monster

	// 識別状態（分類ごとの種別数だけ用意）。
	PotionKnown []bool
	ScrollKnown []bool
	RingKnown   []bool
	WandKnown   []bool
	// でたらめラベルの割り当て（種別→ラベル添字の並び替え）。
	PotionLabel []int
	ScrollLabel []int
	RingLabel   []int
	WandLabel   []int

	HasAmulet bool
	Turns     int
	Lang      string // "ja" / "en"

	io       IO
	rng      *rand.Rand
	vis      [][]bool // 現在見えているマス（一時状態）
	msgs     []string // このターンにためたメッセージ（--More-- でつなぐ）
	quit     bool     // ループ終了要求（切断など）
	saveReq  bool     // S でセーブして中断
	quitGame bool     // Q で投了（金塊はスコアに残る）
	dead     bool
	won      bool
	cause    string // 死因
	name     string // 墓標・スコアに出す名前（ハンドル）
}

// Outcome は 1 局の結果（command 側でスコア記録・セーブ削除に使う）。
type Outcome struct {
	Ended    bool // 局が終わった（死亡・勝利・投了）。false=セーブして中断
	Won      bool
	Gold     int
	Depth    int
	MaxDepth int
	Cause    string
}

func newGame(io IO) *Game {
	seed := time.Now().UnixNano()
	g := &Game{
		io:  io,
		rng: rand.New(rand.NewSource(seed)),
	}
	g.PotionKnown = make([]bool, len(potions))
	g.ScrollKnown = make([]bool, len(scrolls))
	g.RingKnown = make([]bool, len(rings))
	g.WandKnown = make([]bool, len(wands))
	g.PotionLabel = g.shuffleLabels(len(potions), len(potionColors))
	g.ScrollLabel = g.shuffleLabels(len(scrolls), len(scrollTitles))
	g.RingLabel = g.shuffleLabels(len(rings), len(ringStones))
	g.WandLabel = g.shuffleLabels(len(wands), len(wandMaterials))
	return g
}

func (g *Game) shuffleLabels(kinds, pool int) []int {
	perm := g.rng.Perm(pool)
	out := make([]int, kinds)
	for i := 0; i < kinds; i++ {
		out[i] = perm[i%pool]
	}
	return out
}

// initPlayer は開始時の主人公と初期装備を作る（原典どおり ほこ・弓・矢・革の
// よろい・食料を持ってスタート）。
func (g *Game) initPlayer() {
	p := &g.Player
	p.MaxHP, p.HP = 12, 12
	p.Str, p.MaxStr = 16, 16
	p.Exp, p.Level = 0, 1
	p.Gold = 0
	p.Hunger = 1300
	p.Weapon, p.Armor, p.LeftRing, p.RightRng = -1, -1, -1, -1
	p.Sees = true

	mace := &Object{Kind: oWeapon, Which: 0, Count: 1, Enchant1: 1, Enchant2: 1, Flags: ofIdentified}
	bow := &Object{Kind: oWeapon, Which: 2, Count: 1, Enchant1: 1, Flags: ofIdentified}
	arrows := &Object{Kind: oWeapon, Which: 3, Count: g.rng.Intn(15) + 25, Flags: ofIdentified}
	leather := &Object{Kind: oArmor, Which: 0, Count: 1, Enchant1: 0, Flags: ofIdentified}
	food := &Object{Kind: oFood, Which: 0, Count: 1}
	p.Pack = []*Object{mace, bow, arrows, leather, food}
	p.Weapon = 0 // ほこを装備
	p.Armor = 3  // 革のよろいを着る
}

// --- ダイス・乱数 ---

func (g *Game) rnd(n int) int {
	if n <= 0 {
		return 0
	}
	return g.rng.Intn(n)
}

// d(count, sides) は count 個の sides 面ダイスの合計。
func (g *Game) d(count, sides int) int {
	t := 0
	for i := 0; i < count; i++ {
		t += g.rng.Intn(sides) + 1
	}
	return t
}

// rollDice は "2d4" 形式を振る。"0d0" は 0。"/" 区切りの複数攻撃は roundDice で。
func (g *Game) rollDice(spec string) int {
	c, s := parseDice(spec)
	return g.d(c, s)
}

func parseDice(spec string) (count, sides int) {
	i := strings.IndexByte(spec, 'd')
	if i < 0 {
		return 0, 0
	}
	count = atoiSafe(spec[:i])
	sides = atoiSafe(spec[i+1:])
	return
}

func atoiSafe(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func (g *Game) percent(p int) bool { return g.rng.Intn(100) < p }

// coin は五分五分。
func (g *Game) coin() bool { return g.rng.Intn(2) == 0 }

// --- メッセージ ---

// msg はこのターンのメッセージ行に足す。複数たまったら --More-- でつなぐ。
func (g *Game) msg(s string) {
	if s == "" {
		return
	}
	g.msgs = append(g.msgs, s)
}

// flushMsg はためたメッセージを最上行に順に出す。複数なら --More-- で待つ。
func (g *Game) flushMsg() {
	if len(g.msgs) == 0 {
		g.putMsg("")
		return
	}
	for i, m := range g.msgs {
		if i < len(g.msgs)-1 {
			g.putMsg(m + " " + g.tr("--続く--", "--More--"))
			g.waitMore()
		} else {
			g.putMsg(m)
		}
	}
	g.msgs = g.msgs[:0]
}

func (g *Game) putMsg(s string) {
	g.io.Print(gotoRC(MsgRow, 0) + ansiClearEOL + clip(s, Cols))
}

func (g *Game) waitMore() {
	for {
		c, err := g.io.ReadKey()
		if err != nil {
			g.quit = true
			return
		}
		if c == ' ' || c == '\n' {
			return
		}
	}
}

// tr は表示言語で日本語／英語を選ぶ。
func (g *Game) tr(ja, en string) string {
	if g.Lang == "en" {
		return en
	}
	return ja
}

func clip(s string, w int) string {
	// 表示セル幅で切る（全角=2）。ANSI は含めない前提。
	cells := 0
	for i, r := range s {
		cw := 1
		if r >= 0x1100 && (r < 0x2000 || r >= 0x2E80) {
			cw = 2
		}
		if cells+cw > w {
			return s[:i]
		}
		cells += cw
	}
	return s
}
