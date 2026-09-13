package rogue

// --- 袋（pack）管理 ---

func packLetter(idx int) byte { return byte('a' + idx) }

func (g *Game) findPack(letter byte) int {
	idx := int(letter - 'a')
	if idx >= 0 && idx < len(g.Player.Pack) {
		return idx
	}
	return -1
}

// removeFromPack は idx の品物を袋から抜き、装備添字を補正する。
func (g *Game) removeFromPack(idx int) *Object {
	if idx < 0 || idx >= len(g.Player.Pack) {
		return nil
	}
	o := g.Player.Pack[idx]
	g.Player.Pack = append(g.Player.Pack[:idx], g.Player.Pack[idx+1:]...)
	fix := func(e *int) {
		switch {
		case *e == idx:
			*e = -1
		case *e > idx:
			*e = *e - 1
		}
	}
	fix(&g.Player.Weapon)
	fix(&g.Player.Armor)
	fix(&g.Player.LeftRing)
	fix(&g.Player.RightRng)
	return o
}

// inventoryLines は袋の中身を「a) 名前 (装備印)」の行にする。
func (g *Game) inventoryLines() []string {
	if len(g.Player.Pack) == 0 {
		return []string{g.tr("何も持っていない。", "You are empty-handed.")}
	}
	var out []string
	for i, o := range g.Player.Pack {
		mark := ""
		switch i {
		case g.Player.Weapon:
			mark = g.tr("（手に持っている）", " (weapon in hand)")
		case g.Player.Armor:
			mark = g.tr("（着ている）", " (being worn)")
		case g.Player.LeftRing:
			mark = g.tr("（左手）", " (on left hand)")
		case g.Player.RightRng:
			mark = g.tr("（右手）", " (on right hand)")
		}
		out = append(out, string(packLetter(i))+") "+g.objName(o)+mark)
	}
	return out
}

// showList は一覧を画面上部に出して 1 キー待つ（i 持ち物・D 発見など）。
func (g *Game) showList(lines []string) {
	var b []byte
	b = append(b, ansiHome...)
	for i, ln := range lines {
		if i >= Rows-1 {
			break
		}
		b = append(b, []byte(gotoRC(i, 0))...)
		b = append(b, []byte(ansiClearEOL+clip(ln, Cols))...)
	}
	b = append(b, []byte(gotoRC(min(len(lines), Rows-1), 0))...)
	b = append(b, []byte(ansiClearEOL+g.tr("--キーを押すと戻る--", "--press a key--"))...)
	g.io.Print(string(b))
	g.io.ReadKey()
}

// pickItem は品物 1 個を選ばせる。filter が nil なら全部。'*' で一覧。
// ok=false は中止。
func (g *Game) pickItem(prompt string, filter func(*Object) bool) (int, bool) {
	g.putMsg(prompt + g.tr("（* で一覧, ESC で中止）", " (* to list, ESC to cancel)"))
	for {
		c, err := g.io.ReadKey()
		if err != nil {
			g.quit = true
			return -1, false
		}
		switch {
		case c == 0x1b:
			return -1, false
		case c == '*':
			var lines []string
			for i, o := range g.Player.Pack {
				if filter == nil || filter(o) {
					lines = append(lines, string(packLetter(i))+") "+g.objName(o))
				}
			}
			if len(lines) == 0 {
				lines = []string{g.tr("該当する品物がない。", "No such item.")}
			}
			g.showList(lines)
			g.render()
			g.putMsg(prompt + g.tr("（* で一覧, ESC で中止）", " (* to list, ESC to cancel)"))
		case c >= 'a' && c <= 'z':
			idx := g.findPack(c)
			if idx < 0 {
				g.putMsg(g.tr("そんな品物はない。もう一度。", "No such item. Try again."))
				continue
			}
			if filter != nil && !filter(g.Player.Pack[idx]) {
				g.putMsg(g.tr("それは選べない。もう一度。", "You can't select that. Try again."))
				continue
			}
			return idx, true
		}
	}
}

// --- 装備・消費コマンド ---

func (g *Game) doInventory() {
	g.showList(g.inventoryLines())
	g.render()
}

func (g *Game) doWield() {
	idx, ok := g.pickItem(g.tr("どれを手に持つ? ", "wield which weapon? "), func(o *Object) bool {
		return o.Kind == oWeapon
	})
	if !ok {
		return
	}
	if g.Player.Weapon >= 0 {
		w := g.Player.Pack[g.Player.Weapon]
		if w.has(ofCursed) {
			g.msg(g.tr("今持っている武器は呪われていて放せない。",
				"You can't. The weapon in your hand is cursed."))
			return
		}
	}
	g.Player.Weapon = idx
	o := g.Player.Pack[idx]
	if o.has(ofCursed) {
		o.set(ofKnownCurse)
		g.msg(g.tr(g.objName(o)+"を手にした。呪われている！", "You wield "+g.objName(o)+". It is cursed!"))
	} else {
		g.msg(g.tr(g.objName(o)+"を手にした。", "You are now wielding "+g.objName(o)+"."))
	}
	g.endTurn()
}

func (g *Game) doWear() {
	if g.Player.Armor >= 0 {
		g.msg(g.tr("先に今のよろいを脱ぐ必要がある。", "You are already wearing armor. Take it off first."))
		return
	}
	idx, ok := g.pickItem(g.tr("どのよろいを着る? ", "wear which armor? "), func(o *Object) bool {
		return o.Kind == oArmor
	})
	if !ok {
		return
	}
	g.Player.Armor = idx
	o := g.Player.Pack[idx]
	o.set(ofIdentified)
	if o.has(ofCursed) {
		o.set(ofKnownCurse)
		g.msg(g.tr(g.objName(o)+"を着た。呪われている！", "You wear "+g.objName(o)+". It is cursed!"))
	} else {
		g.msg(g.tr(g.objName(o)+"を着た。", "You are now wearing "+g.objName(o)+"."))
	}
	g.endTurn()
}

func (g *Game) doTakeOff() {
	if g.Player.Armor < 0 {
		g.msg(g.tr("よろいを着ていない。", "You aren't wearing any armor."))
		return
	}
	o := g.Player.Pack[g.Player.Armor]
	if o.has(ofCursed) {
		g.msg(g.tr("よろいが呪われていて脱げない！", "You can't. It appears to be cursed."))
		return
	}
	g.msg(g.tr(g.objName(o)+"を脱いだ。", "You take off "+g.objName(o)+"."))
	g.Player.Armor = -1
	g.endTurn()
}

func (g *Game) doDrop() {
	if g.at(g.Player.Pos.X, g.Player.Pos.Y).Obj != nil {
		g.msg(g.tr("足元にすでに品物がある。", "There is already something here."))
		return
	}
	idx, ok := g.pickItem(g.tr("どれを落とす? ", "drop which item? "), nil)
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	if (idx == g.Player.Weapon || idx == g.Player.Armor) && o.has(ofCursed) {
		g.msg(g.tr("呪われていて手放せない。", "You can't. It is cursed."))
		return
	}
	g.removeFromPack(idx)
	o.Pos = g.Player.Pos
	g.at(g.Player.Pos.X, g.Player.Pos.Y).Obj = o
	g.msg(g.tr(g.objName(o)+"を置いた。", "You dropped "+g.objName(o)+"."))
	g.endTurn()
}

func (g *Game) doCall() {
	idx, ok := g.pickItem(g.tr("どれに呼び名を付ける? ", "call which item? "), func(o *Object) bool {
		return o.Kind == oPotion || o.Kind == oScroll || o.Kind == oRing || o.Kind == oWand
	})
	if !ok {
		return
	}
	g.putMsg(g.tr("呼び名: ", "call it: "))
	name, ok, err := readLine(g.io, 24)
	if err != nil {
		g.quit = true
		return
	}
	if !ok {
		return
	}
	g.Player.Pack[idx].CallName = name
}

func (g *Game) doEat() {
	idx, ok := g.pickItem(g.tr("何を食べる? ", "eat what? "), func(o *Object) bool {
		return o.Kind == oFood
	})
	if !ok {
		return
	}
	o := g.Player.Pack[idx]
	o.Count--
	if o.Count <= 0 {
		g.removeFromPack(idx)
	}
	g.Player.Hunger += 1100 + g.rnd(400)
	if g.Player.Hunger > 2000 {
		g.Player.Hunger = 2000
	}
	g.msg(g.tr("食事をとった。ふう、満腹だ。", "You eat the food. Yum!"))
	g.endTurn()
}
