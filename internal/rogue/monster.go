package rogue

// spawnMonsters は depth 階にモンスターを配置する。
func (g *Game) spawnMonsters(depth int) {
	n := 3 + g.rnd(depth+2)
	if n > 12 {
		n = 12
	}
	for i := 0; i < n; i++ {
		p := g.emptyFloorForMonster()
		if p.X < 0 {
			return
		}
		which := g.pickMonster(depth)
		g.addMonster(which, p, true)
	}
}

func (g *Game) pickMonster(depth int) int {
	var cand []int
	for i, m := range monsters {
		if m.Depth <= depth && m.Depth >= depth-8 {
			cand = append(cand, i)
		}
	}
	if len(cand) == 0 {
		// 浅い階向けにフォールバック。
		for i, m := range monsters {
			if m.Depth <= max(depth, 1) {
				cand = append(cand, i)
			}
		}
	}
	if len(cand) == 0 {
		return g.rnd(len(monsters))
	}
	return cand[g.rnd(len(cand))]
}

func (g *Game) addMonster(which int, p Coord, asleep bool) *Monster {
	def := monsters[which]
	hp := g.d(def.HD, 8)
	if hp < 1 {
		hp = 1
	}
	m := &Monster{Which: which, Char: def.Char, Pos: p, HP: hp, MaxHP: hp}
	if asleep {
		m.add(msAsleep)
	} else {
		m.add(msWake)
	}
	// 品物を持つことがある（倒すと落とす）。
	if g.percent(def.Carry) {
		m.Carry = g.randomObject(g.Depth)
	}
	g.Monsters = append(g.Monsters, m)
	return m
}

func (g *Game) emptyFloorForMonster() Coord {
	for tries := 0; tries < 300; tries++ {
		s := g.rnd(9)
		if !g.Rooms[s].Exists {
			continue
		}
		p := g.randRoomFloor(s)
		c := g.at(p.X, p.Y)
		if c.Terr == tFloor && p != g.Player.Pos && g.monsterAt(p.X, p.Y) == nil {
			// プレイヤーに隣接しない位置を優先。
			if abs(p.X-g.Player.Pos.X) <= 1 && abs(p.Y-g.Player.Pos.Y) <= 1 {
				continue
			}
			return p
		}
	}
	return Coord{-1, -1}
}

func (g *Game) monsterAt(x, y int) *Monster {
	for _, m := range g.Monsters {
		if m.Pos.X == x && m.Pos.Y == y {
			return m
		}
	}
	return nil
}

func (g *Game) monsterIndex(m *Monster) int {
	for i, mm := range g.Monsters {
		if mm == m {
			return i
		}
	}
	return -1
}

func (g *Game) removeMonster(m *Monster) {
	i := g.monsterIndex(m)
	if i < 0 {
		return
	}
	g.Monsters = append(g.Monsters[:i], g.Monsters[i+1:]...)
}

// roomOf は座標を含む部屋の添字を返す（無ければ -1）。
func (g *Game) roomOf(x, y int) int {
	for i, r := range g.Rooms {
		if r.contains(x, y) {
			return i
		}
	}
	return -1
}

// canSeePlayer はモンスターがプレイヤーを認識できるか（同じ明るい部屋か隣接）。
func (g *Game) canSeePlayer(m *Monster) bool {
	px, py := g.Player.Pos.X, g.Player.Pos.Y
	if abs(m.Pos.X-px) <= 1 && abs(m.Pos.Y-py) <= 1 {
		return true
	}
	mr := g.roomOf(m.Pos.X, m.Pos.Y)
	pr := g.roomOf(px, py)
	if mr >= 0 && mr == pr && g.Rooms[mr].Lit {
		return true
	}
	return false
}

// moveMonsters は全モンスターに 1 手ずつ行動させる。
func (g *Game) moveMonsters() {
	// スライスを複製（行動中に消えることがある）。
	list := make([]*Monster, len(g.Monsters))
	copy(list, g.Monsters)
	for _, m := range list {
		if g.monsterIndex(m) < 0 {
			continue // すでに倒された・消えた
		}
		if g.dead || g.won {
			return
		}
		g.stepMonster(m)
	}
}

func (g *Game) stepMonster(m *Monster) {
	def := monsters[m.Which]
	// 凍結中は動けない。
	if m.has(msHeld) {
		// プレイヤーに押さえ込まれている（ice monster など）→ 何もしない確率。
	}
	// 起床判定。
	if m.has(msAsleep) {
		if g.canSeePlayer(m) && (def.Flags&mfMean != 0 || g.percent(40)) {
			m.rm(msAsleep)
			m.add(msWake)
		} else {
			return
		}
	}
	if !g.canSeePlayer(m) && !m.has(msWake) {
		return
	}

	px, py := g.Player.Pos.X, g.Player.Pos.Y
	// 隣接していれば攻撃。
	if abs(m.Pos.X-px) <= 1 && abs(m.Pos.Y-py) <= 1 && !(m.Pos.X == px && m.Pos.Y == py) {
		g.monsterAttack(m)
		return
	}

	// 移動方向を決める。
	dx, dy := 0, 0
	confused := m.has(msConfused) || def.Flags&mfFlit != 0 && g.coin()
	if confused {
		dx, dy = g.rnd(3)-1, g.rnd(3)-1
	} else {
		dx, dy = stepToward(m.Pos.X, m.Pos.Y, px, py)
	}
	nx, ny := m.Pos.X+dx, m.Pos.Y+dy
	if g.monsterCanEnter(nx, ny) {
		m.Pos.X, m.Pos.Y = nx, ny
	}
	if m.has(msConfused) {
		m.State &^= msConfused // 1 手で解ける簡易版
	}
}

func stepToward(x, y, tx, ty int) (int, int) {
	dx, dy := 0, 0
	if tx > x {
		dx = 1
	} else if tx < x {
		dx = -1
	}
	if ty > y {
		dy = 1
	} else if ty < y {
		dy = -1
	}
	return dx, dy
}

func (g *Game) monsterCanEnter(x, y int) bool {
	if !inMap(x, y) {
		return false
	}
	if x == g.Player.Pos.X && y == g.Player.Pos.Y {
		return false
	}
	if g.monsterAt(x, y) != nil {
		return false
	}
	switch g.at(x, y).Terr {
	case tFloor, tPassage, tDoor, tStairs:
		return true
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
