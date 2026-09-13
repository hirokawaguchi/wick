package rogue

import "fmt"

// markVisible は現在見えているマスを計算し、見えたマスを既知にする。
// 明るい部屋にいれば部屋全体、そうでなければ周囲 1 マスだけ見える（原典どおり）。
func (g *Game) markVisible() {
	if g.vis == nil || len(g.vis) != MapH {
		g.vis = make([][]bool, MapH)
		for y := range g.vis {
			g.vis[y] = make([]bool, MapW)
		}
	}
	for y := 0; y < MapH; y++ {
		for x := 0; x < MapW; x++ {
			g.vis[y][x] = false
		}
	}
	px, py := g.Player.Pos.X, g.Player.Pos.Y

	// 明るい部屋にいれば部屋全体を見せる。
	if pr := g.roomOf(px, py); pr >= 0 && g.Rooms[pr].Lit {
		r := g.Rooms[pr]
		for y := r.Top; y <= r.Bottom; y++ {
			for x := r.Left; x <= r.Right; x++ {
				g.reveal(x, y)
			}
		}
	}
	// 周囲 8 マス＋自分（暗い部屋・通路用）。
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			g.reveal(px+dx, py+dy)
		}
	}
}

func (g *Game) reveal(x, y int) {
	if !inMap(x, y) {
		return
	}
	g.vis[y][x] = true
	g.at(x, y).Flags |= cfDiscovered
}

func (g *Game) visible(x, y int) bool {
	return inMap(x, y) && g.vis != nil && g.vis[y][x]
}

// render は全画面を 1 度に描く（メッセージ行は flushMsg が別に扱う）。
func (g *Game) render() {
	var b []byte
	b = append(b, ansiHideCur...)
	for y := 0; y < MapH; y++ {
		b = append(b, []byte(gotoRC(MapTop+y, 0))...)
		for x := 0; x < MapW; x++ {
			b = append(b, g.glyph(x, y))
		}
		b = append(b, []byte(ansiClearEOL)...)
	}
	// ステータス行。
	b = append(b, []byte(gotoRC(StatusRow, 0))...)
	b = append(b, []byte(ansiClearEOL+g.statusLine())...)
	// カーソルをプレイヤーに置いて表示。
	b = append(b, []byte(gotoRC(MapTop+g.Player.Pos.Y, g.Player.Pos.X))...)
	b = append(b, []byte(ansiShowCur)...)
	g.io.Print(string(b))
}

func (g *Game) glyph(x, y int) byte {
	if x == g.Player.Pos.X && y == g.Player.Pos.Y {
		return '@'
	}
	c := g.at(x, y)
	// 見えているモンスター。
	if g.visible(x, y) {
		if m := g.monsterAt(x, y); m != nil {
			if monsters[m.Which].Flags&mfInvis == 0 || g.Player.Sees {
				return m.Char
			}
		}
	}
	if !c.has(cfDiscovered) {
		return ' '
	}
	// 既知のわな。
	if c.Trap != tNoTrap && c.has(cfTrapSeen) {
		return '^'
	}
	// 既知の床の品物。
	if c.Obj != nil {
		return symbolOf(c.Obj)
	}
	return terrainChar(c.Terr)
}

func terrainChar(t terrain) byte {
	switch t {
	case tFloor:
		return '.'
	case tHWall:
		return '-'
	case tVWall:
		return '|'
	case tDoor:
		return '+'
	case tPassage:
		return '#'
	case tStairs:
		return '%'
	}
	return ' '
}

// statusLine は最下行のステータス。
func (g *Game) statusLine() string {
	p := &g.Player
	hunger := ""
	switch {
	case p.Hunger <= 0:
		hunger = g.tr("  餓死寸前", "  Fainting")
	case p.Hunger <= 20:
		hunger = g.tr("  衰弱", "  Weak")
	case p.Hunger <= 300:
		hunger = g.tr("  空腹", "  Hungry")
	}
	if g.Lang == "en" {
		return fmt.Sprintf("Level:%d  Gold:%d  Hp:%d(%d)  Str:%d(%d)  Arm:%d  Exp:%d/%d%s",
			g.Depth, p.Gold, p.HP, p.MaxHP, p.Str, p.MaxStr, g.armorClass(), p.Level, p.Exp, hunger)
	}
	return fmt.Sprintf("階:%d  金塊:%d  体力:%d(%d)  強さ:%d(%d)  守備:%d  経験:%d/%d%s",
		g.Depth, p.Gold, p.HP, p.MaxHP, p.Str, p.MaxStr, g.armorClass(), p.Level, p.Exp, hunger)
}
