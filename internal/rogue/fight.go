package rogue

import "strings"

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// playerAttack はプレイヤーが m を殴る。
func (g *Game) playerAttack(m *Monster) {
	def := monsters[m.Which]
	m.rm(msAsleep)
	m.add(msWake)

	toHit := g.Player.Level + strHit(g.Player.Str) + g.ringBonus("dexterity")
	if w := g.equippedWeapon(); w != nil {
		toHit += w.Enchant1
	}
	chance := clampi(45+4*toHit+4*def.AC, 5, 95)
	if g.rnd(100) >= chance {
		g.msg(g.tr(g.monName(m.Which)+"への攻撃は外れた。",
			"You miss the "+g.monName(m.Which)+"."))
		return
	}
	dmg := g.weaponDamage()
	m.HP -= dmg
	if m.HP <= 0 {
		g.killMonster(m)
		return
	}
	g.msg(g.tr(g.monName(m.Which)+"に当たった。", "You hit the "+g.monName(m.Which)+"."))
}

func (g *Game) weaponDamage() int {
	base := g.d(1, 4) // 素手
	if w := g.equippedWeapon(); w != nil {
		c, s := parseDice(weapons[w.Which].MDam)
		if c > 0 {
			base = g.d(c, s) + w.Enchant2
		}
	}
	dmg := base + strDam(g.Player.Str)
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}

// killMonster は m を倒し、経験値・持ち物を処理する。
func (g *Game) killMonster(m *Monster) {
	def := monsters[m.Which]
	g.msg(g.tr(g.monName(m.Which)+"を倒した。", "You defeated the "+g.monName(m.Which)+"."))
	// 持ち物を落とす。
	if m.Carry != nil {
		c := g.at(m.Pos.X, m.Pos.Y)
		if c.Obj == nil && (c.Terr == tFloor || c.Terr == tPassage || c.Terr == tDoor) {
			m.Carry.Pos = m.Pos
			c.Obj = m.Carry
		}
	}
	g.removeMonster(m)
	g.gainExp(def.Exp)
}

func (g *Game) gainExp(x int) {
	g.Player.Exp += x
	for g.Player.Level < len(expLevels) && g.Player.Exp >= expLevels[g.Player.Level] {
		g.Player.Level++
		inc := g.d(1, 10)
		g.Player.MaxHP += inc
		g.Player.HP += inc
		g.msg(g.tr("経験レベルが "+itoa(g.Player.Level)+" に上がった。",
			"Welcome to level "+itoa(g.Player.Level)+"."))
	}
}

// monsterAttack はモンスター m がプレイヤーを攻撃する。
func (g *Game) monsterAttack(m *Monster) {
	def := monsters[m.Which]
	chance := clampi(40+4*def.HD-4*g.armorClass(), 5, 95)
	if g.rnd(100) >= chance {
		g.msg(g.tr(g.monName(m.Which)+"の攻撃をかわした。",
			"The "+g.monName(m.Which)+" misses you."))
		return
	}

	// 盗む系は当たると盗んで消える。
	if def.Flags&mfStealGold != 0 {
		if g.Player.Gold > 0 {
			stolen := g.Player.Gold/2 + g.rnd(g.Player.Gold/2+1)
			g.Player.Gold -= stolen
			g.msg(g.tr(g.monName(m.Which)+"に金塊を奪われた！", "The "+g.monName(m.Which)+" steals your gold!"))
		}
		g.removeMonster(m)
		return
	}
	if def.Flags&mfStealItem != 0 {
		g.stealItem(m)
		return
	}

	// ダメージ（複数攻撃 "/" 区切り）。
	total := 0
	for _, part := range strings.Split(def.Dmg, "/") {
		total += g.rollDice(part)
	}
	g.Player.HP -= total
	g.msg(g.tr(g.monName(m.Which)+"に攻撃された。", "The "+g.monName(m.Which)+" hits you."))

	// 特殊効果。
	g.specialHit(m, def)

	if g.Player.HP <= 0 {
		g.setDeath(g.tr(g.monName(m.Which)+"に殺された", "killed by a "+g.monName(m.Which)))
	}
}

func (g *Game) specialHit(m *Monster, def monDef) {
	switch {
	case def.Flags&mfDrainStr != 0:
		if g.Player.Str > 3 && !g.percent(30) {
			g.Player.Str--
			g.msg(g.tr("力が抜けていく…", "You feel weaker."))
		}
	case def.Flags&mfRusts != 0:
		if a := g.equippedArmor(); a != nil && !a.has(ofProtected) && g.ringBonus("maintain armor") == 0 {
			a.Enchant1--
			g.msg(g.tr("よろいがさびた！", "Your armor is corroded!"))
		}
	case def.Flags&mfDrainLife != 0:
		if g.Player.MaxHP > 12 {
			g.Player.MaxHP -= g.d(1, 3)
			if g.Player.HP > g.Player.MaxHP {
				g.Player.HP = g.Player.MaxHP
			}
			g.msg(g.tr("生命力を吸い取られた！", "You feel your life draining away!"))
		}
	case def.Flags&mfDrainExp != 0:
		drain := g.d(1, 10)
		if g.Player.Exp >= drain {
			g.Player.Exp -= drain
			g.msg(g.tr("経験を奪われた！", "You feel less experienced."))
		}
	case def.Flags&mfConfuses != 0:
		g.Player.Confused += g.d(2, 4)
		g.msg(g.tr("頭がくらくらする…", "You feel confused."))
	case def.Flags&mfFreezes != 0:
		g.Player.Held += g.d(1, 3)
		g.msg(g.tr("凍りついて動けない！", "You are frozen!"))
	case def.Flags&mfHolds != 0:
		g.Player.Held += g.d(1, 3)
		g.msg(g.tr("からめ取られて動けない！", "You are held fast!"))
	}
}

func (g *Game) stealItem(m *Monster) {
	if len(g.Player.Pack) == 0 {
		g.removeMonster(m)
		return
	}
	// 装備していない品物を優先して盗む。
	idx := g.rnd(len(g.Player.Pack))
	o := g.Player.Pack[idx]
	g.removeFromPack(idx)
	_ = o
	g.msg(g.tr(g.monName(m.Which)+"に持ち物を盗まれた！", "The "+g.monName(m.Which)+" steals from you!"))
	g.removeMonster(m)
}

func (g *Game) setDeath(cause string) {
	g.dead = true
	g.cause = cause
}
