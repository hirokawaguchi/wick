package rogue

// --- 装備・能力値のヘルパ ---

func (g *Game) equippedWeapon() *Object {
	if g.Player.Weapon >= 0 && g.Player.Weapon < len(g.Player.Pack) {
		return g.Player.Pack[g.Player.Weapon]
	}
	return nil
}

func (g *Game) equippedArmor() *Object {
	if g.Player.Armor >= 0 && g.Player.Armor < len(g.Player.Pack) {
		return g.Player.Pack[g.Player.Armor]
	}
	return nil
}

func (g *Game) ringObjs() []*Object {
	var out []*Object
	if g.Player.LeftRing >= 0 && g.Player.LeftRing < len(g.Player.Pack) {
		out = append(out, g.Player.Pack[g.Player.LeftRing])
	}
	if g.Player.RightRng >= 0 && g.Player.RightRng < len(g.Player.Pack) {
		out = append(out, g.Player.Pack[g.Player.RightRng])
	}
	return out
}

// armorClass は守備力（大きいほど攻撃を受けにくい）。
func (g *Game) armorClass() int {
	ac := 0
	if a := g.equippedArmor(); a != nil {
		ac = armors[a.Which].Class + a.Enchant1
	}
	for _, r := range g.ringObjs() {
		if rings[r.Which].NameEN == "protection" {
			ac += r.Enchant1
		}
	}
	return ac
}

func (g *Game) ringBonus(name string) int {
	total := 0
	for _, r := range g.ringObjs() {
		if rings[r.Which].NameEN == name {
			total += r.Enchant1
		}
	}
	return total
}

func strHit(str int) int {
	switch {
	case str < 8:
		return -1
	case str < 17:
		return 0
	case str < 19:
		return 1
	case str < 21:
		return 2
	default:
		return 3
	}
}

func strDam(str int) int {
	switch {
	case str < 8:
		return -1
	case str < 16:
		return 0
	case str < 18:
		return 1
	case str < 20:
		return 2
	case str < 22:
		return 3
	default:
		return 4
	}
}

// --- 方角 ---

func dirOf(key byte) (int, int, bool) {
	switch key {
	case 'h', 'H':
		return -1, 0, true
	case 'l', 'L':
		return 1, 0, true
	case 'k', 'K':
		return 0, -1, true
	case 'j', 'J':
		return 0, 1, true
	case 'y', 'Y':
		return -1, -1, true
	case 'u', 'U':
		return 1, -1, true
	case 'b', 'B':
		return -1, 1, true
	case 'n', 'N':
		return 1, 1, true
	}
	return 0, 0, false
}

func (g *Game) walkable(x, y int) bool {
	if !inMap(x, y) {
		return false
	}
	switch g.at(x, y).Terr {
	case tFloor, tPassage, tDoor, tStairs:
		return true
	}
	return false
}

// movePlayer は 1 マス移動（またはその先のモンスターへ攻撃）。pickup=false は
// m コマンド（拾わない移動）。turn を消費したら endTurn する。
func (g *Game) movePlayer(dx, dy int, pickup bool) {
	if g.Player.Confused > 0 {
		dx, dy = g.rnd(3)-1, g.rnd(3)-1
	}
	if dx == 0 && dy == 0 {
		return
	}
	nx, ny := g.Player.Pos.X+dx, g.Player.Pos.Y+dy
	if m := g.monsterAt(nx, ny); m != nil {
		g.playerAttack(m)
		g.endTurn()
		return
	}
	if !g.walkable(nx, ny) {
		return // 壁にぶつかった。ターンは消費しない。
	}
	g.Player.Pos.X, g.Player.Pos.Y = nx, ny
	g.enterCell(pickup)
	g.markVisible()
	g.endTurn()
}

// enterCell は移動先で拾う・わな・階段の処理。
func (g *Game) enterCell(pickup bool) {
	c := g.at(g.Player.Pos.X, g.Player.Pos.Y)
	if c.Trap != tNoTrap {
		c.Flags |= cfTrapSeen
		g.springTrap(c)
	}
	if c.Obj != nil && pickup {
		g.pickUp()
	} else if c.Obj != nil {
		g.msg(g.tr("足元に "+g.objName(c.Obj)+" がある。", "You see "+g.objName(c.Obj)+" here."))
	}
	if c.Terr == tStairs {
		g.msg(g.tr("階段がある。", "There is a staircase here."))
	}
}

// pickUp は足元の品物を拾う。
func (g *Game) pickUp() {
	c := g.at(g.Player.Pos.X, g.Player.Pos.Y)
	o := c.Obj
	if o == nil {
		g.msg(g.tr("ここには何もない。", "There is nothing here."))
		return
	}
	if o.Kind == oGold {
		g.Player.Gold += o.Count
		c.Obj = nil
		g.msg(g.tr(itoa(o.Count)+" 枚の金塊を拾った。", "You found "+itoa(o.Count)+" gold pieces."))
		return
	}
	if o.Kind == oAmulet {
		g.HasAmulet = true
	}
	if len(g.Player.Pack) >= 26 {
		g.msg(g.tr("持ち物がいっぱいで拾えない。", "Your pack is too full."))
		return
	}
	c.Obj = nil
	letter := g.addToPack(o)
	g.msg(g.tr(string(letter)+") "+g.objName(o)+" を拾った。",
		string(letter)+") "+g.objName(o)))
}

// addToPack は品物を袋に入れ、割り当てた文字（a-z）を返す。矢などは束ねる。
func (g *Game) addToPack(o *Object) byte {
	// 同種の束ねられる物はまとめる。
	if o.Kind == oWeapon && (weapons[o.Which].Group == grpArrow || weapons[o.Which].Group == grpDart) {
		for i, e := range g.Player.Pack {
			if e.Kind == oWeapon && e.Which == o.Which && e.Enchant1 == o.Enchant1 && e.Enchant2 == o.Enchant2 {
				e.Count += o.Count
				return byte('a' + i)
			}
		}
	}
	if o.Kind == oFood {
		for i, e := range g.Player.Pack {
			if e.Kind == oFood && e.Which == o.Which {
				e.Count += o.Count
				return byte('a' + i)
			}
		}
	}
	if o.Kind == oPotion || o.Kind == oScroll {
		for i, e := range g.Player.Pack {
			if e.Kind == o.Kind && e.Which == o.Which && e.CallName == o.CallName {
				e.Count += o.Count
				return byte('a' + i)
			}
		}
	}
	g.Player.Pack = append(g.Player.Pack, o)
	return byte('a' + len(g.Player.Pack) - 1)
}

// runPlayer は指定方向に「何か面白い物・壁」まで走る（大文字移動）。
func (g *Game) runPlayer(dx, dy int) {
	for steps := 0; steps < 100; steps++ {
		nx, ny := g.Player.Pos.X+dx, g.Player.Pos.Y+dy
		if !g.walkable(nx, ny) {
			return
		}
		if g.monsterAt(nx, ny) != nil {
			return
		}
		g.Player.Pos.X, g.Player.Pos.Y = nx, ny
		c := g.at(nx, ny)
		trap := c.Trap != tNoTrap
		if trap {
			c.Flags |= cfTrapSeen
			g.springTrap(c)
		}
		if c.Obj != nil {
			g.pickUp()
			g.markVisible()
			g.endTurn()
			return
		}
		g.markVisible()
		g.endTurn()
		if g.dead || g.won || g.quit {
			return
		}
		// 近くにモンスターが見えたら止まる。
		if g.monsterNearVisible() {
			return
		}
		// 分岐（ドア・通路の交差）で止まる。
		if c.Terr == tDoor || c.Terr == tStairs || g.junction(nx, ny) {
			return
		}
	}
}

func (g *Game) monsterNearVisible() bool {
	for _, m := range g.Monsters {
		if g.visible(m.Pos.X, m.Pos.Y) && g.canSeePlayer(m) {
			return true
		}
	}
	return false
}

// junction は通路の分岐かどうか（走行停止用の簡易判定）。
func (g *Game) junction(x, y int) bool {
	if g.at(x, y).Terr != tPassage {
		return false
	}
	open := 0
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if g.walkable(x+d[0], y+d[1]) {
			open++
		}
	}
	return open >= 3
}

// search は隣接するわな・隠しドアを探す。
func (g *Game) search() {
	found := false
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := g.Player.Pos.X+dx, g.Player.Pos.Y+dy
			if !inMap(x, y) {
				continue
			}
			c := g.at(x, y)
			if c.Trap != tNoTrap && !c.has(cfTrapSeen) && g.percent(35) {
				c.Flags |= cfTrapSeen
				found = true
			}
		}
	}
	if found {
		g.msg(g.tr("わなを見つけた！", "You found a trap!"))
	}
	g.endTurn()
}

// goDown は階段を降りる。
func (g *Game) goDown() {
	if g.at(g.Player.Pos.X, g.Player.Pos.Y).Terr != tStairs {
		g.msg(g.tr("ここには階段がない。", "There is no staircase here."))
		return
	}
	g.Depth++
	if g.Depth > g.MaxDepth {
		g.MaxDepth = g.Depth
	}
	g.genLevel(g.Depth)
	g.msg(g.tr("地下 "+itoa(g.Depth)+" 階に降りた。", "You descend to level "+itoa(g.Depth)+"."))
}

// goUp は階段を上る。魔除けが要る。26 階から上り切れば勝利。
func (g *Game) goUp() {
	if g.at(g.Player.Pos.X, g.Player.Pos.Y).Terr != tStairs {
		g.msg(g.tr("ここには階段がない。", "There is no staircase here."))
		return
	}
	if !g.HasAmulet {
		g.msg(g.tr("イェンダーの魔除けを持っていない。上れない。",
			"You need the Amulet of Yendor to climb up."))
		return
	}
	if g.Depth <= 1 {
		g.won = true
		return
	}
	g.Depth--
	g.genLevel(g.Depth)
	g.msg(g.tr("地下 "+itoa(g.Depth)+" 階に戻った。", "You climb up to level "+itoa(g.Depth)+"."))
}

// rest は 1 ターン休む。
func (g *Game) rest() {
	g.endTurn()
}
