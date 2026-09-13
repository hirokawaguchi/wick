package rogue

import (
	"io"
	"math/rand"
	"testing"
)

// nullIO は入力なし・出力捨てのテスト用 IO。キーが尽きたら EOF で終わる。
type nullIO struct {
	keys []byte
	i    int
	out  []byte
}

func (n *nullIO) ReadKey() (byte, error) {
	if n.i >= len(n.keys) {
		return 0, io.EOF
	}
	c := n.keys[n.i]
	n.i++
	return c, nil
}
func (n *nullIO) Print(s string)    { n.out = append(n.out, s...) }
func (n *nullIO) Pending() []string { return nil }
func (n *nullIO) Size() (int, int)  { return 80, 24 }

// memPersist はテスト用のセーブ／スコア置き場。
type memPersist struct {
	blob    []byte
	deleted bool
	scores  []Score
}

func (p *memPersist) Load() ([]byte, bool, error) {
	return p.blob, p.blob != nil, nil
}
func (p *memPersist) Save(b []byte) error { p.blob = b; return nil }
func (p *memPersist) Delete() error       { p.blob = nil; p.deleted = true; return nil }
func (p *memPersist) AddScore(s Score) error {
	p.scores = append(p.scores, s)
	return nil
}
func (p *memPersist) Top(limit int) ([]Score, error) { return p.scores, nil }

func newTestGame(seed int64) *Game {
	g := newGame(&nullIO{})
	g.rng = rand.New(rand.NewSource(seed))
	g.PotionLabel = g.shuffleLabels(len(potions), len(potionColors))
	g.ScrollLabel = g.shuffleLabels(len(scrolls), len(scrollTitles))
	g.RingLabel = g.shuffleLabels(len(rings), len(ringStones))
	g.WandLabel = g.shuffleLabels(len(wands), len(wandMaterials))
	return g
}

func monByChar(c byte) int {
	for i, m := range monsters {
		if m.Char == c {
			return i
		}
	}
	return -1
}

// TestGenerateConnected は多数のシードで生成が破綻せず、階段がプレイヤーから
// 歩いて到達できることを確かめる。
func TestGenerateConnected(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		g := newTestGame(seed)
		g.Depth = 1
		g.initPlayer()
		g.genLevel(1)

		// 階段が 1 つ以上ある。
		stairs := Coord{-1, -1}
		count := 0
		for y := 0; y < MapH; y++ {
			for x := 0; x < MapW; x++ {
				if g.at(x, y).Terr == tStairs {
					stairs = Coord{x, y}
					count++
				}
			}
		}
		if count == 0 {
			t.Fatalf("seed %d: 階段が生成されなかった", seed)
		}
		// プレイヤーは床の上にいる。
		if !g.walkable(g.Player.Pos.X, g.Player.Pos.Y) {
			t.Fatalf("seed %d: プレイヤーが歩けないマスにいる", seed)
		}
		// BFS で階段へ到達できる。
		if !g.reachable(g.Player.Pos, stairs) {
			t.Fatalf("seed %d: 階段へ到達できない（連結でない）", seed)
		}
	}
}

func (g *Game) reachable(from, to Coord) bool {
	seen := make([][]bool, MapH)
	for y := range seen {
		seen[y] = make([]bool, MapW)
	}
	q := []Coord{from}
	seen[from.Y][from.X] = true
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		if c == to {
			return true
		}
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := c.X+d[0], c.Y+d[1]
			if inMap(nx, ny) && !seen[ny][nx] && g.walkable(nx, ny) {
				seen[ny][nx] = true
				q = append(q, Coord{nx, ny})
			}
		}
	}
	return false
}

// TestIdentifyShuffle はラベル割当が範囲内で、learnKind が効くこと。
func TestIdentifyShuffle(t *testing.T) {
	g := newTestGame(7)
	if len(g.PotionLabel) != len(potions) {
		t.Fatalf("水薬ラベル数が合わない")
	}
	for _, l := range g.PotionLabel {
		if l < 0 || l >= len(potionColors) {
			t.Fatalf("ラベル添字が範囲外: %d", l)
		}
	}
	if g.knownKind(oPotion, 0) {
		t.Fatalf("最初は未識別のはず")
	}
	g.learnKind(oPotion, 0)
	if !g.knownKind(oPotion, 0) {
		t.Fatalf("learnKind で識別済みにならない")
	}
}

// TestHungerDeath は空腹が尽きると餓死すること。
func TestHungerDeath(t *testing.T) {
	g := newTestGame(3)
	g.initPlayer()
	g.genLevel(1)
	g.Monsters = nil
	g.Player.Hunger = -149
	g.endTurn()
	if !g.dead {
		t.Fatalf("餓死するはずが生きている（Hunger=%d）", g.Player.Hunger)
	}
}

// TestCombatKills は十分強いプレイヤーが弱いモンスターを倒せること。
func TestCombatKills(t *testing.T) {
	g := newTestGame(5)
	g.initPlayer()
	g.genLevel(1)
	g.Player.Level = 20
	g.Player.Str = 20

	which := monByChar('S') // へび
	m := g.addMonster(which, Coord{g.Player.Pos.X + 2, g.Player.Pos.Y}, false)
	killed := false
	for i := 0; i < 50; i++ {
		g.playerAttack(m)
		if g.monsterIndex(m) < 0 {
			killed = true
			break
		}
	}
	if !killed {
		t.Fatalf("弱いモンスターを倒せなかった")
	}
	if g.Player.Exp == 0 {
		t.Fatalf("倒しても経験値が入っていない")
	}
}

// TestSaveRoundTrip はセーブ→復元で主要な状態が保たれること。
func TestSaveRoundTrip(t *testing.T) {
	g := newTestGame(9)
	g.initPlayer()
	g.genLevel(1)
	g.Depth = 4
	g.MaxDepth = 5
	g.Player.Gold = 123
	g.Player.HP = 7
	g.HasAmulet = true

	blob, err := g.encode()
	if err != nil {
		t.Fatalf("encode 失敗: %v", err)
	}
	g2, err := decodeGame(blob, &nullIO{})
	if err != nil {
		t.Fatalf("decode 失敗: %v", err)
	}
	if g2.Depth != 4 || g2.MaxDepth != 5 {
		t.Fatalf("階が保たれない: %d/%d", g2.Depth, g2.MaxDepth)
	}
	if g2.Player.Gold != 123 || g2.Player.HP != 7 {
		t.Fatalf("プレイヤー状態が保たれない")
	}
	if !g2.HasAmulet {
		t.Fatalf("魔除け所持が保たれない")
	}
	if len(g2.Player.Pack) != len(g.Player.Pack) {
		t.Fatalf("持ち物数が保たれない: %d != %d", len(g2.Player.Pack), len(g.Player.Pack))
	}
	for i := range g.PotionLabel {
		if g2.PotionLabel[i] != g.PotionLabel[i] {
			t.Fatalf("識別ラベルが保たれない")
		}
	}
}

// TestPlayLoopSave は数手動いて S でセーブ中断する一連が破綻しないこと。
func TestPlayLoopSave(t *testing.T) {
	// 移動キーを一通り叩き、最後に S→y でセーブして中断。
	keys := []byte("hjklyubn.s")
	keys = append(keys, 'S', 'y')
	io := &nullIO{keys: keys}
	pr := &memPersist{}
	out, err := Play(io, pr, Options{Lang: "ja", Name: "テスト"})
	if err != nil {
		t.Fatalf("Play がエラー: %v", err)
	}
	if out.Ended {
		t.Fatalf("S 中断は Ended=false のはず")
	}
	if pr.blob == nil {
		t.Fatalf("セーブが書かれていない")
	}
	if len(io.out) == 0 {
		t.Fatalf("画面出力が無い")
	}
}

// TestPlayLoopQuit は Q→y で投了し、スコアが記録されセーブが消えること。
func TestPlayLoopQuit(t *testing.T) {
	keys := []byte{'Q', 'y'}
	io := &nullIO{keys: keys}
	pr := &memPersist{}
	out, err := Play(io, pr, Options{Lang: "en", Name: "tester"})
	if err != nil {
		t.Fatalf("Play がエラー: %v", err)
	}
	if !out.Ended {
		t.Fatalf("投了は Ended=true のはず")
	}
	if len(pr.scores) != 1 {
		t.Fatalf("スコアが 1 件記録されるはず: %d", len(pr.scores))
	}
	if !pr.deleted {
		t.Fatalf("投了後にセーブが消えるはず")
	}
}

// TestObjectNames は識別状態で名前表示が変わること。
func TestObjectNames(t *testing.T) {
	g := newTestGame(1)
	o := &Object{Kind: oPotion, Which: 0, Count: 1}
	before := g.objName(o)
	g.learnKind(oPotion, 0)
	after := g.objName(o)
	if before == after {
		t.Fatalf("識別前後で水薬の名前が変わらない: %q", before)
	}
}
