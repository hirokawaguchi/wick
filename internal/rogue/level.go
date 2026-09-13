package rogue

// 3x3 のセクタに部屋を置き、隣接する部屋を通路でつなぐ。原典の「各階に部屋と
// それを結ぶ通路、階段が 1 つ」を素直に再現する。

var xEdges = []int{0, 26, 53, 80}
var yEdges = []int{0, 7, 14, 22}

const amuletDepth = 26

// genLevel は depth 階を生成し、Cells/Rooms/Monsters を作り直す。
func (g *Game) genLevel(depth int) {
	g.Cells = make([][]Cell, MapH)
	for y := range g.Cells {
		g.Cells[y] = make([]Cell, MapW)
	}
	g.Rooms = make([]Room, 9)
	g.Monsters = nil

	for s := 0; s < 9; s++ {
		g.makeRoom(s, depth)
	}
	g.connectRooms()

	// 階段。魔除けを取って戻る途中の階でも下り階段は要る（原典どおり常設）。
	g.placeStairs()

	// プレイヤー配置（最初の階／降りたとき）。階段と重ならない床へ。
	g.Player.Pos = g.randFloorAvoidStairs()

	g.placeObjects(depth)
	g.placeTraps(depth)
	g.spawnMonsters(depth)

	if depth == amuletDepth && !g.HasAmulet {
		g.placeAmulet()
	}

	g.markVisible()
}

func (g *Game) at(x, y int) *Cell {
	return &g.Cells[y][x]
}

func inMap(x, y int) bool { return x >= 0 && x < MapW && y >= 0 && y < MapH }

func (g *Game) makeRoom(s, depth int) {
	c, r := s%3, s/3
	x0, x1 := xEdges[c], xEdges[c+1]-1
	y0, y1 := yEdges[r], yEdges[r+1]-1
	sw, sh := x1-x0+1, y1-y0+1

	rw := 4 + g.rnd(min(sw-2, 12))
	rh := 4 + g.rnd(min(sh-2, 8))
	if rw > sw-1 {
		rw = sw - 1
	}
	if rh > sh-1 {
		rh = sh - 1
	}
	left := x0 + g.rnd(sw-rw)
	top := y0 + g.rnd(sh-rh)
	room := Room{
		Exists: true,
		Left:   left,
		Top:    top,
		Right:  left + rw - 1,
		Bottom: top + rh - 1,
	}
	// 深い階ほど暗い部屋が増える。
	darkChance := depth * 3
	if darkChance > 65 {
		darkChance = 65
	}
	room.Lit = depth <= 1 || !g.percent(darkChance)
	g.Rooms[s] = room
	g.carveRoom(room)
}

func (g *Game) carveRoom(room Room) {
	for y := room.Top; y <= room.Bottom; y++ {
		for x := room.Left; x <= room.Right; x++ {
			cell := g.at(x, y)
			switch {
			case y == room.Top || y == room.Bottom:
				cell.Terr = tHWall
			case x == room.Left || x == room.Right:
				cell.Terr = tVWall
			default:
				cell.Terr = tFloor
			}
			if room.Lit {
				cell.Flags |= cfLit
			}
		}
	}
}

// connectRooms は 3x3 の隣接グラフに全域木＋数本の追加辺を張り、通路でつなぐ。
func (g *Game) connectRooms() {
	// 隣接辺（右・下）
	type edge struct{ a, b int }
	var edges []edge
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			s := r*3 + c
			if c < 2 {
				edges = append(edges, edge{s, s + 1})
			}
			if r < 2 {
				edges = append(edges, edge{s, s + 3})
			}
		}
	}
	g.rng.Shuffle(len(edges), func(i, j int) { edges[i], edges[j] = edges[j], edges[i] })

	// Union-Find で全域木を作る。
	parent := make([]int, 9)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	used := make(map[[2]int]bool)
	connect := func(a, b int) {
		g.digCorridor(a, b)
		used[[2]int{a, b}] = true
	}
	for _, e := range edges {
		ra, rb := find(e.a), find(e.b)
		if ra != rb {
			parent[ra] = rb
			connect(e.a, e.b)
		}
	}
	// 追加辺を少し（ループを作って行き止まりを減らす）。
	extra := 2 + g.rnd(3)
	for _, e := range edges {
		if extra <= 0 {
			break
		}
		if !used[[2]int{e.a, e.b}] {
			connect(e.a, e.b)
			extra--
		}
	}
}

// digCorridor は隣接する 2 部屋を L 字通路でつなぐ（セクタ間の隙間を通す）。
func (g *Game) digCorridor(a, b int) {
	ra, rb := g.Rooms[a], g.Rooms[b]
	if !ra.Exists || !rb.Exists {
		return
	}
	horizontal := (a%3 != b%3) // 左右隣接
	if horizontal {
		// a が左、b が右になるよう並べ替え。
		if ra.Left > rb.Left {
			ra, rb = rb, ra
		}
		ay := ra.Top + 1 + g.rnd(ra.Bottom-ra.Top-1)
		by := rb.Top + 1 + g.rnd(rb.Bottom-rb.Top-1)
		g.putDoor(ra.Right, ay)
		g.putDoor(rb.Left, by)
		midX := (ra.Right + rb.Left) / 2
		g.digH(ra.Right+1, midX, ay)
		g.digV(ay, by, midX)
		g.digH(midX, rb.Left-1, by)
	} else {
		// a が上、b が下になるよう並べ替え。
		if ra.Top > rb.Top {
			ra, rb = rb, ra
		}
		ax := ra.Left + 1 + g.rnd(ra.Right-ra.Left-1)
		bx := rb.Left + 1 + g.rnd(rb.Right-rb.Left-1)
		g.putDoor(ax, ra.Bottom)
		g.putDoor(bx, rb.Top)
		midY := (ra.Bottom + rb.Top) / 2
		g.digV(ra.Bottom+1, midY, ax)
		g.digH(ax, bx, midY)
		g.digV(midY, rb.Top-1, bx)
	}
}

func (g *Game) putDoor(x, y int) {
	if inMap(x, y) {
		g.at(x, y).Terr = tDoor
	}
}

func (g *Game) digH(x0, x1, y int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := x0; x <= x1; x++ {
		g.digPassage(x, y)
	}
}

func (g *Game) digV(y0, y1, x int) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		g.digPassage(x, y)
	}
}

func (g *Game) digPassage(x, y int) {
	if !inMap(x, y) {
		return
	}
	c := g.at(x, y)
	if c.Terr == tNothing {
		c.Terr = tPassage
	}
}

// --- 配置 ---

func (g *Game) placeStairs() {
	for tries := 0; tries < 200; tries++ {
		s := g.rnd(9)
		if !g.Rooms[s].Exists {
			continue
		}
		p := g.randRoomFloor(s)
		if g.at(p.X, p.Y).Terr == tFloor {
			g.at(p.X, p.Y).Terr = tStairs
			return
		}
	}
}

func (g *Game) randRoomFloor(s int) Coord {
	room := g.Rooms[s]
	for tries := 0; tries < 100; tries++ {
		x := room.Left + 1 + g.rnd(room.Right-room.Left-1)
		y := room.Top + 1 + g.rnd(room.Bottom-room.Top-1)
		if g.at(x, y).Terr == tFloor {
			return Coord{x, y}
		}
	}
	return Coord{room.Left + 1, room.Top + 1}
}

func (g *Game) randFloorAvoidStairs() Coord {
	for tries := 0; tries < 500; tries++ {
		s := g.rnd(9)
		if !g.Rooms[s].Exists {
			continue
		}
		p := g.randRoomFloor(s)
		c := g.at(p.X, p.Y)
		if c.Terr == tFloor && c.Obj == nil {
			return p
		}
	}
	// どこかの部屋内へ。
	for s := 0; s < 9; s++ {
		if g.Rooms[s].Exists {
			return g.randRoomFloor(s)
		}
	}
	return Coord{1, 1}
}

func (g *Game) placeObjects(depth int) {
	n := 2 + g.rnd(4)
	for i := 0; i < n; i++ {
		p := g.emptyFloor()
		if p.X < 0 {
			return
		}
		g.at(p.X, p.Y).Obj = g.randomObject(depth)
		g.at(p.X, p.Y).Obj.Pos = p
	}
	// 金塊。いくつかの部屋に。
	for s := 0; s < 9; s++ {
		if g.Rooms[s].Exists && g.percent(45) {
			p := g.randRoomFloor(s)
			if g.at(p.X, p.Y).Terr == tFloor && g.at(p.X, p.Y).Obj == nil {
				amt := g.d(2, 50+depth*10)
				g.at(p.X, p.Y).Obj = &Object{Kind: oGold, Count: amt, Pos: p}
			}
		}
	}
}

func (g *Game) emptyFloor() Coord {
	for tries := 0; tries < 300; tries++ {
		s := g.rnd(9)
		if !g.Rooms[s].Exists {
			continue
		}
		p := g.randRoomFloor(s)
		c := g.at(p.X, p.Y)
		if c.Terr == tFloor && c.Obj == nil && !(p == g.Player.Pos) {
			return p
		}
	}
	return Coord{-1, -1}
}

func (g *Game) placeTraps(depth int) {
	n := g.rnd(min(depth, 4) + 1)
	for i := 0; i < n; i++ {
		p := g.emptyFloor()
		if p.X < 0 {
			return
		}
		kind := trapType(1 + g.rnd(int(trapKindCount)-1))
		g.at(p.X, p.Y).Trap = kind
	}
}

func (g *Game) placeAmulet() {
	p := g.emptyFloor()
	if p.X < 0 {
		p = g.randFloorAvoidStairs()
	}
	g.at(p.X, p.Y).Obj = &Object{Kind: oAmulet, Count: 1, Flags: ofIdentified, Pos: p}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
