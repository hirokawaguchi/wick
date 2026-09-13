package rogue

// endTurn は 1 ターン経過の共通処理。プレイヤーの行動後に必ず呼ぶ。
func (g *Game) endTurn() {
	g.Turns++

	// 状態タイマ。
	if g.Player.Confused > 0 {
		g.Player.Confused--
	}
	if g.Player.Blind > 0 {
		g.Player.Blind--
	}
	if g.Player.Haste > 0 {
		g.Player.Haste--
	}
	if g.Player.Levit > 0 {
		g.Player.Levit--
	}
	if g.Player.Held > 0 {
		g.Player.Held--
	}

	g.digest()
	if g.dead {
		return
	}

	// 俊足のときはモンスターを 1 ターンおきに動かす（相対的に速く動ける）。
	if g.Player.Haste > 0 && g.Turns%2 == 0 {
		// モンスターは休み。
	} else {
		g.moveMonsters()
	}

	g.regen()

	if g.Player.HP <= 0 && !g.dead {
		g.setDeath(g.cause)
	}
}

func (g *Game) digest() {
	cost := 1 + len(g.ringObjs())
	if g.ringBonus("slow digestion") != 0 {
		cost = 1
	}
	before := g.Player.Hunger
	g.Player.Hunger -= cost

	// しきい値をまたいだら知らせる。
	if before > 300 && g.Player.Hunger <= 300 {
		g.msg(g.tr("お腹が空いてきた。", "You are starting to get hungry."))
	} else if before > 20 && g.Player.Hunger <= 20 {
		g.msg(g.tr("かなり衰弱してきた。食べないと危ない。", "You are starting to feel weak."))
	} else if before > 0 && g.Player.Hunger <= 0 {
		g.msg(g.tr("目の前が暗い…餓死寸前だ！", "You are fainting from hunger!"))
	}

	if g.Player.Hunger <= -150 {
		g.setDeath(g.tr("餓死した", "died of starvation"))
		return
	}
	if g.Player.Hunger <= 0 && g.percent(12) {
		g.Player.Held += g.d(1, 3)
		g.msg(g.tr("空腹で気を失った…", "You faint from lack of food."))
	}
}

func (g *Game) regen() {
	g.Player.RegenCnt++
	period := 20 - g.Player.Level
	if period < 3 {
		period = 3
	}
	if g.Player.RegenCnt >= period {
		g.Player.RegenCnt = 0
		heal := 1 + g.ringBonus("regeneration")
		if g.Player.HP < g.Player.MaxHP {
			g.Player.HP += heal
			if g.Player.HP > g.Player.MaxHP {
				g.Player.HP = g.Player.MaxHP
			}
		}
	}
}
