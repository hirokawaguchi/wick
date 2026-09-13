package rogue

import "fmt"

// randomObject は depth 階に落ちている品物を 1 つ作る。
func (g *Game) randomObject(depth int) *Object {
	switch r := g.rnd(100); {
	case r < 26:
		return g.makePotion()
	case r < 52:
		return g.makeScroll()
	case r < 62:
		return g.makeWeapon()
	case r < 72:
		return g.makeArmor()
	case r < 82:
		return &Object{Kind: oFood, Which: 0, Count: 1}
	case r < 90:
		return g.makeRing()
	default:
		return g.makeWand()
	}
}

func (g *Game) makePotion() *Object {
	return &Object{Kind: oPotion, Which: g.rnd(len(potions)), Count: 1}
}

func (g *Game) makeScroll() *Object {
	return &Object{Kind: oScroll, Which: g.rnd(len(scrolls)), Count: 1}
}

func (g *Game) makeWeapon() *Object {
	w := g.rnd(len(weapons))
	o := &Object{Kind: oWeapon, Which: w, Count: 1}
	if weapons[w].Group == grpArrow || weapons[w].Group == grpDart {
		o.Count = g.rnd(8) + 3
	}
	// 祝福／呪い。
	switch {
	case g.percent(10):
		o.Enchant1 = -(g.rnd(3) + 1)
		o.set(ofCursed)
	case g.percent(15):
		o.Enchant1 = g.rnd(3) + 1
		o.Enchant2 = g.rnd(3)
	}
	return o
}

func (g *Game) makeArmor() *Object {
	a := g.rnd(len(armors))
	o := &Object{Kind: oArmor, Which: a, Count: 1}
	switch {
	case g.percent(12):
		o.Enchant1 = -(g.rnd(3) + 1)
		o.set(ofCursed)
	case g.percent(18):
		o.Enchant1 = g.rnd(3) + 1
	}
	return o
}

func (g *Game) makeRing() *Object {
	o := &Object{Kind: oRing, Which: g.rnd(len(rings)), Count: 1}
	// 効きの強さ。呪いの指輪はマイナス。
	if g.percent(20) {
		o.Enchant1 = -(g.rnd(2) + 1)
		o.set(ofCursed)
	} else {
		o.Enchant1 = g.rnd(2) + 1
	}
	return o
}

func (g *Game) makeWand() *Object {
	return &Object{Kind: oWand, Which: g.rnd(len(wands)), Count: 1, Enchant1: g.rnd(5) + 3}
}

// --- 名前・ラベル表示 ---

func (g *Game) potionColor(which int) string  { return potionColors[g.PotionLabel[which]] }
func (g *Game) scrollTitle(which int) string  { return scrollTitles[g.ScrollLabel[which]] }
func (g *Game) ringStone(which int) string    { return ringStones[g.RingLabel[which]] }
func (g *Game) wandMaterial(which int) string { return wandMaterials[g.WandLabel[which]] }

func (g *Game) potionName(which int) string {
	if g.Lang == "en" {
		return potions[which].NameEN
	}
	return potions[which].NameJA
}
func (g *Game) scrollName(which int) string {
	if g.Lang == "en" {
		return scrolls[which].NameEN
	}
	return scrolls[which].NameJA
}
func (g *Game) ringName(which int) string {
	if g.Lang == "en" {
		return rings[which].NameEN
	}
	return rings[which].NameJA
}
func (g *Game) wandName(which int) string {
	if g.Lang == "en" {
		return wands[which].NameEN
	}
	return wands[which].NameJA
}
func (g *Game) weaponName(which int) string {
	if g.Lang == "en" {
		return weapons[which].NameEN
	}
	return weapons[which].NameJA
}
func (g *Game) armorName(which int) string {
	if g.Lang == "en" {
		return armors[which].NameEN
	}
	return armors[which].NameJA
}

func (g *Game) monName(which int) string {
	if g.Lang == "en" {
		return monsters[which].NameEN
	}
	return monsters[which].NameJA
}

// objName は品物 1 個の表示名を返す。識別状態と呼び名を反映する。
func (g *Game) objName(o *Object) string {
	switch o.Kind {
	case oGold:
		return g.tr(fmt.Sprintf("%d 枚の金塊", o.Count), fmt.Sprintf("%d gold", o.Count))
	case oAmulet:
		return g.tr("イェンダーの魔除け", "the Amulet of Yendor")
	case oFood:
		if o.Count > 1 {
			return g.tr(fmt.Sprintf("食料 %d 個", o.Count), fmt.Sprintf("%d rations of food", o.Count))
		}
		return g.tr("食料", "some food")
	case oWeapon:
		return g.weaponDisplay(o)
	case oArmor:
		return g.armorDisplay(o)
	case oPotion:
		return g.classDisplay(o, o.Kind)
	case oScroll:
		return g.classDisplay(o, o.Kind)
	case oRing:
		return g.classDisplay(o, o.Kind)
	case oWand:
		return g.classDisplay(o, o.Kind)
	}
	return "?"
}

func (g *Game) weaponDisplay(o *Object) string {
	name := g.weaponName(o.Which)
	q := ""
	if o.Count > 1 {
		q = fmt.Sprintf("%d 本の", o.Count)
		if g.Lang == "en" {
			q = fmt.Sprintf("%d ", o.Count)
		}
	}
	if o.identified() || o.Enchant1 != 0 || o.Enchant2 != 0 {
		if o.identified() {
			return fmt.Sprintf("%s%s (%+d,%+d)", q, name, o.Enchant1, o.Enchant2)
		}
	}
	return q + name
}

func (g *Game) armorDisplay(o *Object) string {
	name := g.armorName(o.Which)
	base := armors[o.Which].Class
	if o.identified() {
		return fmt.Sprintf("%s [%d]", name, base+o.Enchant1)
	}
	return name
}

// classDisplay は水薬・巻物・指輪・杖の表示。未識別ならラベル、識別済みなら真名。
func (g *Game) classDisplay(o *Object, kind objKind) string {
	known := g.knownKind(kind, o.Which)
	if o.CallName != "" && !known {
		return g.labelOnly(o) + " 「" + o.CallName + "」"
	}
	if known || o.identified() {
		switch kind {
		case oPotion:
			return g.tr("水薬（"+g.potionName(o.Which)+"）", "potion of "+g.potionName(o.Which))
		case oScroll:
			return g.tr("巻物（"+g.scrollName(o.Which)+"）", "scroll of "+g.scrollName(o.Which))
		case oRing:
			return g.ringName(o.Which)
		case oWand:
			n := g.wandName(o.Which)
			if o.identified() {
				return fmt.Sprintf("%s [%d]", n, o.Enchant1)
			}
			return n
		}
	}
	return g.labelOnly(o)
}

func (g *Game) labelOnly(o *Object) string {
	switch o.Kind {
	case oPotion:
		return g.tr(g.potionColor(o.Which)+"水薬", g.potionColor(o.Which)+"potion")
	case oScroll:
		return g.tr("「"+g.scrollTitle(o.Which)+"」と書かれた巻物", "scroll titled "+g.scrollTitle(o.Which))
	case oRing:
		return g.tr(g.ringStone(o.Which)+"指輪", g.ringStone(o.Which)+"ring")
	case oWand:
		return g.tr(g.wandMaterial(o.Which)+"杖", g.wandMaterial(o.Which)+"wand")
	}
	return "?"
}

// knownKind はその種別が識別済みか。
func (g *Game) knownKind(kind objKind, which int) bool {
	switch kind {
	case oPotion:
		return g.PotionKnown[which]
	case oScroll:
		return g.ScrollKnown[which]
	case oRing:
		return g.RingKnown[which]
	case oWand:
		return g.WandKnown[which]
	}
	return false
}

// learnKind はその種別を識別済みにする（使って効果が明らかになったとき）。
func (g *Game) learnKind(kind objKind, which int) {
	switch kind {
	case oPotion:
		g.PotionKnown[which] = true
	case oScroll:
		g.ScrollKnown[which] = true
	case oRing:
		g.RingKnown[which] = true
	case oWand:
		g.WandKnown[which] = true
	}
}

// symbolOf は品物のマップ記号。
func symbolOf(o *Object) byte {
	switch o.Kind {
	case oGold:
		return '*'
	case oWeapon:
		return ')'
	case oArmor:
		return ']'
	case oPotion:
		return '!'
	case oScroll:
		return '?'
	case oRing:
		return '='
	case oWand:
		return '/'
	case oFood:
		return ':'
	case oAmulet:
		return ','
	}
	return '?'
}
