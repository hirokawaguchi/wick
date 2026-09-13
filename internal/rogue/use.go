package rogue

// getDirection は方向キーを 1 つ読む。
func (g *Game) getDirection(prompt string) (int, int, bool) {
	g.putMsg(prompt)
	for {
		c, err := g.io.ReadKey()
		if err != nil {
			g.quit = true
			return 0, 0, false
		}
		if c == 0x1b {
			return 0, 0, false
		}
		if dx, dy, ok := dirOf(c); ok {
			return dx, dy, true
		}
	}
}

func (g *Game) firstMonsterInDir(dx, dy int) *Monster {
	x, y := g.Player.Pos.X, g.Player.Pos.Y
	for i := 0; i < 40; i++ {
		x += dx
		y += dy
		if !inMap(x, y) {
			return nil
		}
		if m := g.monsterAt(x, y); m != nil {
			return m
		}
		switch g.at(x, y).Terr {
		case tHWall, tVWall, tNothing:
			return nil
		}
	}
	return nil
}

// --- 水薬 ---

func (g *Game) doQuaff() {
	idx, ok := g.pickItem(g.tr("どの水薬を飲む? ", "quaff which potion? "), func(o *Object) bool {
		return o.Kind == oPotion
	})
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	which := o.Which
	g.consumeOne(idx)
	g.potionEffect(which)
	g.learnKind(oPotion, which)
	g.endTurn()
}

func (g *Game) potionEffect(which int) {
	p := &g.Player
	switch potions[which].NameEN {
	case "increase strength":
		if p.Str < 31 {
			p.Str++
		}
		if p.Str > p.MaxStr {
			p.MaxStr = p.Str
		}
		g.msg(g.tr("力がみなぎってきた。", "You feel stronger."))
	case "restore strength":
		p.Str = p.MaxStr
		g.msg(g.tr("元の力が戻った。", "Your strength is restored."))
	case "healing":
		p.HP += g.d(p.Level, 4)
		if p.HP >= p.MaxHP {
			p.MaxHP++
			p.HP = p.MaxHP
		}
		g.msg(g.tr("傷が癒えていく。", "You feel better."))
	case "extra healing":
		p.HP += g.d(p.Level, 8)
		p.MaxHP += 2
		if p.HP > p.MaxHP {
			p.HP = p.MaxHP
		}
		g.msg(g.tr("見違えるほど元気になった。", "You feel much better."))
	case "poison":
		if g.ringBonus("add strength") == 0 {
			p.Str -= g.d(1, 3)
			if p.Str < 1 {
				p.Str = 1
			}
		}
		g.msg(g.tr("うっ、毒だ！体が弱っていく。", "You feel very sick now."))
	case "raise level":
		need := 0
		if p.Level < len(expLevels) {
			need = expLevels[p.Level]
		}
		g.gainExp(max(need-p.Exp, 1))
		g.msg(g.tr("経験を積んだ気がする。", "You suddenly feel more experienced."))
	case "blindness":
		p.Blind += g.d(200, 1)
		g.msg(g.tr("目の前が真っ暗になった。", "A cloak of darkness falls around you."))
	case "hallucination":
		g.msg(g.tr("ふらふらする…色がおかしい。", "Oh, wow! Everything looks so cosmic!"))
	case "detect food":
		g.msg(g.tr("食べ物の匂いを感じる。", "Your nose tingles with the smell of food."))
	case "detect magic":
		g.msg(g.tr("魔法の気配を感じる。", "You sense the presence of magic."))
	case "detect monster":
		for _, m := range g.Monsters {
			g.reveal(m.Pos.X, m.Pos.Y)
		}
		g.msg(g.tr("周囲の気配が伝わってくる。", "You sense the presence of monsters."))
	case "levitation":
		p.Levit += g.d(15, 3)
		g.msg(g.tr("体がふわりと浮いた。", "You start to float in the air."))
	case "haste self":
		p.Haste += g.d(4, 4)
		g.msg(g.tr("体が軽い！素早く動ける。", "You feel yourself moving faster."))
	case "see invisible":
		p.Sees = true
		g.msg(g.tr("見えないものが見えるようになった。", "Your eyes tingle."))
	}
}

// --- 巻物 ---

func (g *Game) doRead() {
	idx, ok := g.pickItem(g.tr("どの巻物を読む? ", "read which scroll? "), func(o *Object) bool {
		return o.Kind == oScroll
	})
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	which := o.Which
	g.consumeOne(idx)
	g.scrollEffect(which)
	g.learnKind(oScroll, which)
	g.endTurn()
}

func (g *Game) scrollEffect(which int) {
	p := &g.Player
	switch scrolls[which].NameEN {
	case "identify":
		g.msg(g.tr("鑑定の巻物だ。", "This is a scroll of identify."))
		i2, ok := g.pickItem(g.tr("どれを鑑定する? ", "identify which item? "), nil)
		if ok {
			t := g.Player.Pack[i2]
			t.set(ofIdentified)
			g.learnKind(t.Kind, t.Which)
			g.msg(g.objName(t))
		}
	case "teleportation":
		g.Player.Pos = g.randFloorAvoidStairs()
		g.markVisible()
		g.msg(g.tr("体が引きずられ、別の場所に飛ばされた。", "You feel a wrenching sensation."))
	case "sleep":
		p.Held += g.d(4, 3)
		g.msg(g.tr("眠気に襲われる…", "You fall asleep."))
	case "enchant armor":
		if a := g.equippedArmor(); a != nil {
			a.Enchant1++
			a.clear(ofCursed)
			g.msg(g.tr("よろいが青く輝いた。", "Your armor glows blue for a moment."))
		} else {
			g.msg(g.tr("何も起こらなかった。", "Nothing happens."))
		}
	case "enchant weapon":
		if w := g.equippedWeapon(); w != nil {
			if g.coin() {
				w.Enchant1++
			} else {
				w.Enchant2++
			}
			w.clear(ofCursed)
			g.msg(g.tr("武器が赤く輝いた。", "Your weapon glows red for a moment."))
		} else {
			g.msg(g.tr("何も起こらなかった。", "Nothing happens."))
		}
	case "remove curse":
		for _, o := range g.Player.Pack {
			o.clear(ofCursed)
			o.clear(ofKnownCurse)
		}
		g.msg(g.tr("呪いが解けた気がする。", "You feel as if somebody is watching over you."))
	case "create monster":
		g.createAdjacentMonster()
	case "magic mapping":
		for y := 0; y < MapH; y++ {
			for x := 0; x < MapW; x++ {
				if g.at(x, y).Terr != tNothing {
					g.at(x, y).Flags |= cfDiscovered
				}
			}
		}
		g.msg(g.tr("この階の地図が頭に浮かんだ。", "This level comes into view."))
	case "blank paper":
		g.msg(g.tr("何も書かれていない。", "This scroll seems to be blank."))
	case "protect armor":
		if a := g.equippedArmor(); a != nil {
			a.set(ofProtected)
			g.msg(g.tr("よろいが銀色に光った。", "Your armor is covered by a shimmering gold shield."))
		}
	case "aggravate monster":
		for _, m := range g.Monsters {
			m.rm(msAsleep)
			m.add(msWake)
		}
		g.msg(g.tr("けたたましい音が響いた。周りが殺気立つ。", "You hear a high pitched humming noise."))
	case "hold monster":
		for _, m := range g.Monsters {
			if abs(m.Pos.X-p.Pos.X) <= 2 && abs(m.Pos.Y-p.Pos.Y) <= 2 {
				m.add(msHeld)
			}
		}
		g.msg(g.tr("周囲のモンスターが凍りついた。", "The monsters around you freeze."))
	}
}

func (g *Game) createAdjacentMonster() {
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, -1}, {1, -1}, {-1, 1}} {
		x, y := g.Player.Pos.X+d[0], g.Player.Pos.Y+d[1]
		if g.walkable(x, y) && g.monsterAt(x, y) == nil {
			g.addMonster(g.pickMonster(g.Depth), Coord{x, y}, false)
			g.msg(g.tr("目の前に何かが現れた！", "A monster appears before you!"))
			return
		}
	}
}

// --- 杖 ---

func (g *Game) doZap() {
	idx, ok := g.pickItem(g.tr("どの杖を使う? ", "zap which wand? "), func(o *Object) bool {
		return o.Kind == oWand
	})
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	if o.Enchant1 <= 0 {
		g.msg(g.tr("その杖はもう魔力が尽きている。", "Nothing happens. The wand is drained."))
		g.endTurn()
		return
	}
	dx, dy, ok := g.getDirection(g.tr("どの方向へ? ", "in which direction? "))
	if !ok {
		return
	}
	o.Enchant1--
	m := g.firstMonsterInDir(dx, dy)
	g.wandEffect(o.Which, m)
	g.learnKind(oWand, o.Which)
	g.endTurn()
}

func (g *Game) wandEffect(which int, m *Monster) {
	if m == nil {
		g.msg(g.tr("光は空を切った。", "The bolt vanishes into the distance."))
		return
	}
	switch wands[which].NameEN {
	case "striking":
		g.zapDamage(m, g.d(2, 8))
	case "lightning":
		g.zapDamage(m, g.d(6, 6))
	case "fire":
		g.zapDamage(m, g.d(6, 6))
	case "cold":
		g.zapDamage(m, g.d(6, 6))
	case "slow monster":
		g.msg(g.tr(g.monName(m.Which)+"の動きが鈍った。", "The "+g.monName(m.Which)+" slows down."))
	case "sleep":
		m.add(msAsleep)
		m.rm(msWake)
		g.msg(g.tr(g.monName(m.Which)+"は眠りに落ちた。", "The "+g.monName(m.Which)+" falls asleep."))
	case "cancellation":
		g.msg(g.tr(g.monName(m.Which)+"の魔力が打ち消された。", "The "+g.monName(m.Which)+" is cancelled."))
	case "polymorph":
		m.Which = g.pickMonster(g.Depth)
		m.Char = monsters[m.Which].Char
		g.msg(g.tr("姿がぐにゃりと変わった！", "The monster changes shape!"))
	case "make invisible":
		g.msg(g.tr(g.monName(m.Which)+"が透明になった。", "The "+g.monName(m.Which)+" turns invisible."))
	case "confuse monster":
		m.add(msConfused)
		g.msg(g.tr(g.monName(m.Which)+"は混乱した。", "The "+g.monName(m.Which)+" looks confused."))
	}
}

func (g *Game) zapDamage(m *Monster, dmg int) {
	m.HP -= dmg
	m.rm(msAsleep)
	m.add(msWake)
	if m.HP <= 0 {
		g.killMonster(m)
		return
	}
	g.msg(g.tr(g.monName(m.Which)+"に魔力が命中した。", "The bolt strikes the "+g.monName(m.Which)+"."))
}

// --- 指輪 ---

func (g *Game) doPutRing() {
	if g.Player.LeftRing >= 0 && g.Player.RightRng >= 0 {
		g.msg(g.tr("両手とも指輪をしている。", "You already wear two rings."))
		return
	}
	idx, ok := g.pickItem(g.tr("どの指輪をはめる? ", "put on which ring? "), func(o *Object) bool {
		return o.Kind == oRing
	})
	if !ok {
		return
	}
	if idx == g.Player.LeftRing || idx == g.Player.RightRng {
		g.msg(g.tr("その指輪はもうはめている。", "You are already wearing that ring."))
		return
	}
	if g.Player.LeftRing < 0 {
		g.Player.LeftRing = idx
	} else {
		g.Player.RightRng = idx
	}
	o := g.Player.Pack[idx]
	o.set(ofIdentified)
	g.learnKind(oRing, o.Which)
	if o.has(ofCursed) {
		o.set(ofKnownCurse)
		g.msg(g.tr(g.objName(o)+"をはめた。呪われている！", "You put on "+g.objName(o)+". It is cursed!"))
	} else {
		g.msg(g.tr(g.objName(o)+"をはめた。", "You put on "+g.objName(o)+"."))
	}
	g.endTurn()
}

func (g *Game) doRemoveRing() {
	rings := []int{}
	if g.Player.LeftRing >= 0 {
		rings = append(rings, g.Player.LeftRing)
	}
	if g.Player.RightRng >= 0 {
		rings = append(rings, g.Player.RightRng)
	}
	if len(rings) == 0 {
		g.msg(g.tr("指輪をしていない。", "You aren't wearing any rings."))
		return
	}
	idx, ok := g.pickItem(g.tr("どの指輪を外す? ", "remove which ring? "), func(o *Object) bool {
		return o.Kind == oRing
	})
	if !ok {
		return
	}
	if idx != g.Player.LeftRing && idx != g.Player.RightRng {
		g.msg(g.tr("その指輪ははめていない。", "You aren't wearing that ring."))
		return
	}
	o := g.Player.Pack[idx]
	if o.has(ofCursed) {
		g.msg(g.tr("呪われていて外せない！", "You can't. It appears to be cursed."))
		return
	}
	if idx == g.Player.LeftRing {
		g.Player.LeftRing = -1
	} else {
		g.Player.RightRng = -1
	}
	g.msg(g.tr(g.objName(o)+"を外した。", "You remove "+g.objName(o)+"."))
	g.endTurn()
}

// --- 投げる ---

func (g *Game) doThrow() {
	dx, dy, ok := g.getDirection(g.tr("どの方向へ投げる? ", "throw in which direction? "))
	if !ok {
		return
	}
	idx, ok := g.pickItem(g.tr("何を投げる? ", "throw what? "), func(o *Object) bool {
		return o.Kind == oWeapon
	})
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	// 1 本消費。
	thrown := &Object{Kind: o.Kind, Which: o.Which, Count: 1, Enchant1: o.Enchant1, Enchant2: o.Enchant2, Flags: o.Flags}
	g.consumeOne(idx)

	m := g.firstMonsterInDir(dx, dy)
	if m == nil {
		// 落下点に置く（簡易）。
		g.msg(g.tr(g.weaponName(thrown.Which)+"を投げたが、何にも当たらなかった。",
			"The "+g.weaponName(thrown.Which)+" flies off and misses."))
		g.endTurn()
		return
	}
	toHit := g.Player.Level + strHit(g.Player.Str) + thrown.Enchant1
	chance := clampi(45+4*toHit+4*monsters[m.Which].AC, 5, 95)
	if g.rnd(100) >= chance {
		g.msg(g.tr(g.monName(m.Which)+"に投げたが外れた。", "You miss the "+g.monName(m.Which)+"."))
		g.endTurn()
		return
	}
	c, s := parseDice(weapons[thrown.Which].TDam)
	dmg := g.d(c, s) + thrown.Enchant2
	if dmg < 1 {
		dmg = 1
	}
	m.HP -= dmg
	m.rm(msAsleep)
	m.add(msWake)
	if m.HP <= 0 {
		g.killMonster(m)
	} else {
		g.msg(g.tr(g.monName(m.Which)+"に命中した。", "You hit the "+g.monName(m.Which)+"."))
	}
	g.endTurn()
}

// consumeOne は idx の品物を 1 個減らす（0 で袋から抜く）。
func (g *Game) consumeOne(idx int) {
	o := g.Player.Pack[idx]
	o.Count--
	if o.Count <= 0 {
		g.removeFromPack(idx)
	}
}

// --- わな ---

func (g *Game) springTrap(c *Cell) {
	switch c.Trap {
	case trapDoor:
		g.msg(g.tr("落とし穴だ！下の階へ落ちた。", "You fell through a trap door!"))
		g.Depth++
		if g.Depth > g.MaxDepth {
			g.MaxDepth = g.Depth
		}
		g.genLevel(g.Depth)
	case trapBear:
		g.Player.Held += g.d(2, 3)
		g.msg(g.tr("熊のわなに足を取られた！", "You are caught in a bear trap."))
	case trapTele:
		g.Player.Pos = g.randFloorAvoidStairs()
		g.markVisible()
		g.msg(g.tr("テレポートのわな！飛ばされた。", "You are teleported!"))
	case trapDart:
		dmg := g.d(1, 4)
		g.Player.HP -= dmg
		g.msg(g.tr("矢のわな！ちくりと刺さった。", "A small dart just hit you."))
		if g.Player.HP <= 0 {
			g.setDeath(g.tr("矢のわなにやられた", "killed by a dart trap"))
		}
	case trapSleep:
		g.Player.Held += g.d(2, 3)
		g.msg(g.tr("ガスを浴びて眠くなった…", "A strange white mist envelops you."))
	case trapRust:
		if a := g.equippedArmor(); a != nil && !a.has(ofProtected) {
			a.Enchant1--
			g.msg(g.tr("水がかかってよろいがさびた！", "A gush of water hits you. Your armor rusts."))
		} else {
			g.msg(g.tr("水がかかった。", "A gush of water hits you."))
		}
	}
}
