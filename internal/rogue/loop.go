package rogue

import (
	"fmt"
	"time"
)

// Options はゲーム開始時の設定。
type Options struct {
	Lang string // "ja" / "en"
	Name string // 墓標・スコアに出す名前（ハンドル）
}

// Play は 1 局を回す。セーブがあれば再開を尋ね、再開したらセーブを消す（原典どおり）。
// 戻り値 Outcome.Ended=false は「S でセーブして中断」または切断（セーブ保持）。
func Play(io IO, pr Persist, opt Options) (Outcome, error) {
	var g *Game
	if pr != nil {
		if blob, ok, err := pr.Load(); err == nil && ok && len(blob) > 0 {
			if askYesNo(io, opt.Lang, trStr(opt.Lang,
				"セーブがあります。続きから始めますか? (y/n) ",
				"A saved game exists. Restore it? (y/n) ")) {
				if gg, e := decodeGame(blob, io); e == nil {
					g = gg
					_ = pr.Delete()
				}
			}
			if g == nil {
				_ = pr.Delete()
			}
		}
	}
	if g == nil {
		g = newGame(io)
		g.Lang = opt.Lang
		g.name = opt.Name
		g.startNew()
	} else {
		g.Lang = opt.Lang
		g.name = opt.Name
	}
	return g.run(pr)
}

func (g *Game) startNew() {
	g.Depth = 1
	g.MaxDepth = 1
	g.initPlayer()
	g.genLevel(1)
	g.msg(g.tr("やあ、戦士。運命の洞窟へようこそ。", "Hello, adventurer. Welcome to the Dungeons of Doom."))
}

func (g *Game) run(pr Persist) (Outcome, error) {
	g.io.Print(ansiClear)
	g.markVisible()
	g.render()
	g.flushMsg()

	for !g.quit && !g.dead && !g.won && !g.saveReq && !g.quitGame {
		c, err := g.io.ReadKey()
		if err != nil {
			// 切断。自動セーブして中断（セーブは残す）。
			if pr != nil {
				if blob, e := g.encode(); e == nil {
					_ = pr.Save(blob)
				}
			}
			return Outcome{Ended: false}, err
		}
		g.msgs = g.msgs[:0]
		g.handleKey(c)
		g.surfacePending()
		if g.dead || g.won || g.saveReq || g.quitGame || g.quit {
			break
		}
		g.render()
		g.flushMsg()
	}

	switch {
	case g.saveReq:
		var err error
		if pr != nil {
			if blob, e := g.encode(); e == nil {
				err = pr.Save(blob)
			} else {
				err = e
			}
		}
		g.io.Print(ansiClear + ansiHome + ansiShowCur)
		g.putMsg(g.tr("ゲームをセーブしました。", "Game saved."))
		g.io.Print("\n")
		return Outcome{Ended: false}, err
	case g.dead:
		g.render()
		g.flushMsg()
		g.tombstone()
		if pr != nil {
			_ = pr.AddScore(g.score(false))
			_ = pr.Delete()
		}
		return Outcome{Ended: true, Won: false, Gold: g.Player.Gold,
			Depth: g.Depth, MaxDepth: g.MaxDepth, Cause: g.cause}, nil
	case g.won:
		g.winScreen()
		if pr != nil {
			_ = pr.AddScore(g.score(true))
			_ = pr.Delete()
		}
		return Outcome{Ended: true, Won: true, Gold: g.Player.Gold,
			Depth: g.Depth, MaxDepth: g.MaxDepth}, nil
	case g.quitGame:
		g.cause = g.tr("投了した", "quit the game")
		if pr != nil {
			_ = pr.AddScore(g.score(false))
			_ = pr.Delete()
		}
		g.io.Print(ansiClear + ansiHome + ansiShowCur)
		g.putMsg(g.tr("また会おう、戦士よ。", "Goodbye, adventurer."))
		g.io.Print("\n")
		return Outcome{Ended: true, Won: false, Gold: g.Player.Gold,
			Depth: g.Depth, MaxDepth: g.MaxDepth, Cause: g.cause}, nil
	}
	return Outcome{Ended: false}, nil
}

func (g *Game) score(won bool) Score {
	return Score{
		Gold:     g.Player.Gold,
		Depth:    g.Depth,
		MaxDepth: g.MaxDepth,
		Cause:    g.cause,
		Won:      won,
		Time:     time.Now(),
	}
}

func (g *Game) surfacePending() {
	for _, s := range g.io.Pending() {
		g.msg(s)
	}
}

// handleKey は 1 キーのコマンドを処理する（原典のゲーム内キー）。
func (g *Game) handleKey(c byte) {
	// 移動（小文字＝1 歩、大文字＝走る）。
	if dx, dy, ok := dirOf(c); ok {
		if c >= 'A' && c <= 'Z' {
			if g.blockedHeld() {
				return
			}
			g.runPlayer(dx, dy)
		} else {
			if g.blockedHeld() {
				return
			}
			g.movePlayer(dx, dy, true)
		}
		return
	}

	switch c {
	case '.':
		g.rest()
	case 's':
		g.search()
	case '>':
		g.goDown()
		g.endTurnLite()
	case '<':
		g.goUp()
		g.endTurnLite()
	case ',':
		g.pickUp()
		g.endTurn()
	case 'i':
		g.doInventory()
	case 'I':
		g.selectiveInv()
	case 'q':
		g.doQuaff()
	case 'r':
		g.doRead()
	case 'e':
		g.doEat()
	case 'w':
		g.doWield()
	case 'W':
		g.doWear()
	case 'T':
		g.doTakeOff()
	case 'P':
		g.doPutRing()
	case 'R':
		g.doRemoveRing()
	case 'd':
		g.doDrop()
	case 'c':
		g.doCall()
	case 'z':
		g.doZap()
	case 't':
		g.doThrow()
	case 'm':
		g.moveNoPickup()
	case '^':
		g.identifyTrap()
	case '/':
		g.whatIs()
	case 'D':
		g.discoveries()
	case ')':
		g.showWeapon()
	case ']':
		g.showArmor()
	case '=':
		g.showRings()
	case '@':
		// ステータス行再表示（render が毎回出すので何もしない）。
	case '?':
		g.helpScreen()
	case 'o':
		g.putMsg(g.tr("この局ではオプション設定はありません。", "No options to set here."))
	case 'Q':
		if askYesNo(g.io, g.Lang, g.tr("本当に終了しますか? (y/n) ", "Really quit? (y/n) ")) {
			g.quitGame = true
		}
	case 'S':
		if askYesNo(g.io, g.Lang, g.tr("セーブして中断しますか? (y/n) ", "Save and suspend? (y/n) ")) {
			g.saveReq = true
		}
	case ' ', '\n':
		// 何もしない。
	default:
		g.putMsg(g.tr("そのキーは使えません（? でヘルプ）。", "Unknown command (? for help)."))
	}
}

// endTurnLite は階段移動など、モンスター行動を伴わない軽い更新。
func (g *Game) endTurnLite() {
	g.markVisible()
}

func (g *Game) blockedHeld() bool {
	if g.Player.Held > 0 {
		g.msg(g.tr("押さえ込まれて動けない！", "You are being held!"))
		g.endTurn()
		return true
	}
	return false
}

func (g *Game) moveNoPickup() {
	dx, dy, ok := g.getDirection(g.tr("どの方向へ（拾わずに）? ", "move onto (no pickup) which way? "))
	if !ok {
		return
	}
	if g.blockedHeld() {
		return
	}
	g.movePlayer(dx, dy, false)
}

func (g *Game) identifyTrap() {
	dx, dy, ok := g.getDirection(g.tr("どの方向のわな? ", "which direction is the trap? "))
	if !ok {
		return
	}
	x, y := g.Player.Pos.X+dx, g.Player.Pos.Y+dy
	if inMap(x, y) && g.at(x, y).Trap != tNoTrap {
		g.at(x, y).Flags |= cfTrapSeen
		g.putMsg(g.trapName(g.at(x, y).Trap))
	} else {
		g.putMsg(g.tr("そこにわなはない。", "No trap there."))
	}
}

func (g *Game) trapName(t trapType) string {
	switch t {
	case trapDoor:
		return g.tr("落とし穴", "a trap door")
	case trapBear:
		return g.tr("熊のわな", "a bear trap")
	case trapTele:
		return g.tr("テレポートのわな", "a teleport trap")
	case trapDart:
		return g.tr("矢のわな", "a dart trap")
	case trapSleep:
		return g.tr("眠りガスのわな", "a sleeping gas trap")
	case trapRust:
		return g.tr("水のわな", "a rust trap")
	}
	return g.tr("わな", "a trap")
}

func (g *Game) whatIs() {
	g.putMsg(g.tr("どの記号? ", "what is which symbol? "))
	c, err := g.io.ReadKey()
	if err != nil {
		g.quit = true
		return
	}
	g.putMsg(fmt.Sprintf("%c - %s", c, symbolMeaning(g.Lang, c)))
}

func symbolMeaning(lang string, c byte) string {
	m := map[byte][2]string{
		'@': {"あなた（冒険者）", "you, the adventurer"},
		'.': {"部屋の床", "floor of a room"},
		'#': {"通路", "passage"},
		'+': {"ドア", "a door"},
		'-': {"壁", "a wall"},
		'|': {"壁", "a wall"},
		'*': {"金塊", "gold"},
		')': {"武器", "a weapon"},
		']': {"よろい", "armor"},
		'!': {"水薬", "a potion"},
		'?': {"巻物", "a scroll"},
		'=': {"指輪", "a ring"},
		'/': {"杖", "a wand or staff"},
		'^': {"わな", "a trap"},
		'%': {"階段", "a staircase"},
		':': {"食料", "food"},
		',': {"イェンダーの魔除け", "the Amulet of Yendor"},
	}
	if v, ok := m[c]; ok {
		if lang == "en" {
			return v[1]
		}
		return v[0]
	}
	if c >= 'A' && c <= 'Z' {
		if lang == "en" {
			return "a monster"
		}
		return "モンスター"
	}
	if lang == "en" {
		return "unknown"
	}
	return "不明"
}

func (g *Game) selectiveInv() {
	idx, ok := g.pickItem(g.tr("どれを調べる? ", "inventory which item? "), nil)
	if ok {
		g.putMsg(string(packLetter(idx)) + ") " + g.objName(g.Player.Pack[idx]))
	}
}

func (g *Game) showWeapon() {
	if w := g.equippedWeapon(); w != nil {
		g.putMsg(g.tr("手に持っている武器: ", "wielding: ") + g.objName(w))
	} else {
		g.putMsg(g.tr("素手だ。", "You are empty-handed."))
	}
}

func (g *Game) showArmor() {
	if a := g.equippedArmor(); a != nil {
		g.putMsg(g.tr("着ているよろい: ", "wearing: ") + g.objName(a))
	} else {
		g.putMsg(g.tr("よろいを着ていない。", "You are not wearing armor."))
	}
}

func (g *Game) showRings() {
	rs := g.ringObjs()
	if len(rs) == 0 {
		g.putMsg(g.tr("指輪をしていない。", "You are not wearing any rings."))
		return
	}
	var lines []string
	for _, r := range rs {
		lines = append(lines, g.objName(r))
	}
	g.showList(lines)
	g.render()
}

func (g *Game) discoveries() {
	var lines []string
	add := func(kind objKind, n int, name func(int) string, known []bool) {
		for i := 0; i < n; i++ {
			if known[i] {
				lines = append(lines, name(i))
			}
		}
	}
	add(oPotion, len(potions), func(i int) string { return g.tr("水薬（"+g.potionName(i)+"）", "potion of "+g.potionName(i)) }, g.PotionKnown)
	add(oScroll, len(scrolls), func(i int) string { return g.tr("巻物（"+g.scrollName(i)+"）", "scroll of "+g.scrollName(i)) }, g.ScrollKnown)
	add(oRing, len(rings), func(i int) string { return g.ringName(i) }, g.RingKnown)
	add(oWand, len(wands), func(i int) string { return g.wandName(i) }, g.WandKnown)
	if len(lines) == 0 {
		lines = []string{g.tr("まだ何も判明していない。", "Nothing has been discovered yet.")}
	}
	g.showList(lines)
	g.render()
}

func (g *Game) helpScreen() {
	lines := []string{
		g.tr("=== コマンド一覧 ===", "=== Commands ==="),
		g.tr("h j k l  上下左右へ移動   y u b n  斜め移動   大文字=その方向へ走る", "h j k l  move    y u b n  diagonals   CAPS = run"),
		g.tr(".  休む     s  さがす     ,  拾う     >  降りる     <  上る", ".  rest   s  search   ,  pick up   >  down   <  up"),
		g.tr("i  持ち物   I  1つ調べる  d  落とす   c  呼び名", "i  inventory   I  single   d  drop   c  call"),
		g.tr("w  武器を持つ  W  よろいを着る  T  よろいを脱ぐ", "w  wield   W  wear   T  take off"),
		g.tr("q  水薬を飲む  r  巻物を読む  e  食べる  z  杖を振る  t  投げる", "q  quaff   r  read   e  eat   z  zap   t  throw"),
		g.tr("P  指輪をはめる  R  指輪を外す  ^  わな確認  /  記号の意味", "P  put on ring   R  remove ring   ^  id trap   /  what is"),
		g.tr("m  拾わず移動  D  判明した物  )  武器  ]  よろい  =  指輪", "m  move-no-pickup   D  discoveries   )  weapon   ]  armor   =  rings"),
		g.tr("S  セーブして中断   Q  終了", "S  save and suspend   Q  quit"),
		g.tr("目的: 地下26階のイェンダーの魔除けを持ち帰れ。", "Goal: fetch the Amulet of Yendor from level 26 and return."),
	}
	g.showList(lines)
	g.render()
}

// tombstone は死亡時の墓標。
func (g *Game) tombstone() {
	g.io.Print(ansiClear + ansiShowCur)
	name := g.name
	if name == "" {
		name = g.tr("名もなき戦士", "a nameless hero")
	}
	rip := []string{
		"",
		"                    ----------",
		"                   /          \\",
		"                  /    REST    \\",
		"                 /      IN       \\",
		"                /     PEACE       \\",
		"                |                 |",
		"                | " + centerPad(name, 15) + " |",
		"                |                 |",
		"                | " + centerPad(g.tr("金塊 "+itoa(g.Player.Gold), "gold "+itoa(g.Player.Gold)), 15) + " |",
		"                |                 |",
		"                | " + centerPad(g.tr("地下"+itoa(g.Depth)+"階", "level "+itoa(g.Depth)), 15) + " |",
		"               *|     *  *  *     | *",
		"      _________)/\\\\_//(\\/(/\\)/\\//\\/|_)_______",
		"",
		g.tr("君は"+g.cause+"。", "You "+g.cause+"."),
		g.tr("[スペースで戻る]", "[press space to return]"),
	}
	for i, ln := range rip {
		g.io.Print(gotoRC(i, 0) + ansiClearEOL + ln)
	}
	g.waitMore()
}

func (g *Game) winScreen() {
	g.io.Print(ansiClear + ansiShowCur)
	lines := []string{
		"",
		g.tr("  おめでとう、戦士よ！", "  Congratulations, adventurer!"),
		"",
		g.tr("  君はイェンダーの魔除けを手に、生きて地上へ帰り着いた。",
			"  You have escaped the Dungeons of Doom with the Amulet of Yendor!"),
		g.tr("  地元の戦士協会は君を正会員として迎えるだろう。",
			"  The local guild welcomes you as a full member."),
		"",
		g.tr("  金塊 "+itoa(g.Player.Gold)+" 枚を持ち帰った。", "  You brought back "+itoa(g.Player.Gold)+" gold."),
		"",
		g.tr("[スペースで戻る]", "[press space to return]"),
	}
	for i, ln := range lines {
		g.io.Print(gotoRC(i, 0) + ansiClearEOL + ln)
	}
	g.waitMore()
}

func centerPad(s string, w int) string {
	// 表示セル幅で中央寄せ（全角対応）。
	cw := 0
	for _, r := range s {
		if r >= 0x1100 && (r < 0x2000 || r >= 0x2E80) {
			cw += 2
		} else {
			cw++
		}
	}
	if cw >= w {
		return clip(s, w)
	}
	left := (w - cw) / 2
	right := w - cw - left
	return spaces(left) + s + spaces(right)
}

func spaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func askYesNo(io IO, lang, prompt string) bool {
	io.Print(gotoRC(MsgRow, 0) + ansiClearEOL + prompt)
	for {
		c, err := io.ReadKey()
		if err != nil {
			return false
		}
		switch c {
		case 'y', 'Y':
			return true
		case 'n', 'N', 0x1b, '\n':
			return false
		}
	}
}

func trStr(lang, ja, en string) string {
	if lang == "en" {
		return en
	}
	return ja
}
