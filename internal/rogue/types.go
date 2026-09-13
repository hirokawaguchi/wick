package rogue

// terrain はマップの地形。
type terrain uint8

const (
	tNothing terrain = iota
	tFloor
	tHWall // 横壁 '-'
	tVWall // 縦壁 '|'
	tDoor  // ドア '+'
	tPassage
	tStairs
)

// cellFlag はセルの状態。
type cellFlag uint8

const (
	cfDiscovered cellFlag = 1 << iota // 一度でも見た（地形を描く）
	cfLit                             // 明るい部屋の一部
	cfTrapSeen                        // わなが露見した（^ を描く）
)

// Cell は 1 マス。Obj は床に落ちている品物（無ければ nil）。
type Cell struct {
	Terr  terrain
	Flags cellFlag
	Trap  trapType // tNoTrap = 無し
	Obj   *Object
}

func (c Cell) has(f cellFlag) bool { return c.Flags&f != 0 }

// Room は部屋。座標は壁を含む矩形（inclusive）。
type Room struct {
	Exists bool
	Top    int
	Left   int
	Bottom int
	Right  int
	Lit    bool
	Maze   bool // 迷路部屋（未使用の目印）
}

func (r Room) inside(x, y int) bool {
	return r.Exists && x > r.Left && x < r.Right && y > r.Top && y < r.Bottom
}

func (r Room) contains(x, y int) bool {
	return r.Exists && x >= r.Left && x <= r.Right && y >= r.Top && y <= r.Bottom
}

// objKind は品物の大分類。
type objKind uint8

const (
	oGold objKind = iota
	oWeapon
	oArmor
	oPotion
	oScroll
	oRing
	oWand
	oFood
	oAmulet
)

// objFlag は品物の状態。
type objFlag uint16

const (
	ofIdentified objFlag = 1 << iota
	ofCursed
	ofProtected  // 保護（さび防止）
	ofKnownCurse // 呪いが判明
)

// Object は品物。Which は分類ごとのサブ種別表への添字。
type Object struct {
	Kind     objKind
	Which    int
	Count    int // 数量（金塊は金額、矢・食料など束ねる物）
	Enchant1 int // 命中補正 / よろい強化 / 指輪の効き / 杖の残り回数
	Enchant2 int // 武器の damage 補正
	Flags    objFlag
	CallName string // c コマンドの呼び名
	Pos      Coord  // 床にあるときの位置
}

func (o *Object) has(f objFlag) bool { return o.Flags&f != 0 }
func (o *Object) set(f objFlag)      { o.Flags |= f }
func (o *Object) clear(f objFlag)    { o.Flags &^= f }
func (o *Object) identified() bool   { return o.has(ofIdentified) }

// Coord は座標（マップ内、x=0..MapW-1, y=0..MapH-1）。
type Coord struct {
	X int
	Y int
}

// monFlag はモンスターの性質。
type monFlag uint32

const (
	mfMean        monFlag = 1 << iota // 見つけると向かってくる
	mfFly                             // 飛ぶ
	mfRegen                           // 再生する
	mfInvis                           // 見えない
	mfGreedy                          // 金塊に群がる
	mfFlit                            // ふらつく（不規則移動）
	mfConfuses                        // 命中で混乱させる
	mfRusts                           // よろいをさびさせる
	mfHolds                           // 押さえ込む
	mfFreezes                         // 凍らせる（行動不能）
	mfStealGold                       // 金塊を盗んで消える
	mfStealItem                       // 品物を盗んで消える
	mfDrainStr                        // 強さを奪う
	mfDrainLife                       // 最大 HP を奪う
	mfDrainExp                        // 経験値を奪う
	mfFlames                          // 炎（ドラゴン）
	mfSplits                          // 分裂
	mfAsleepStart                     // 最初は眠っている確率が高い
)

// monState は個体の一時状態。
type monState uint16

const (
	msAsleep monState = 1 << iota
	msWake            // 起きて追跡中
	msHeld            // 押さえ込まれ（プレイヤーに）
	msConfused
	msImitates // 化けている（未使用の目印）
)

// Monster は洞窟の住人 1 体。
type Monster struct {
	Which int // モンスター表への添字
	Char  byte
	Pos   Coord
	HP    int
	MaxHP int
	State monState
	// 盗む・化ける等で運んでいる品物（倒すと落とす）
	Carry *Object
}

func (m *Monster) has(s monState) bool { return m.State&s != 0 }
func (m *Monster) add(s monState)      { m.State |= s }
func (m *Monster) rm(s monState)       { m.State &^= s }

// trapType はわなの種類。
type trapType uint8

const (
	tNoTrap trapType = iota
	trapDoor
	trapBear
	trapTele
	trapDart
	trapSleep
	trapRust
	trapKindCount
)

// Player は主人公。装備は Pack の添字で持つ（gob でポインタ共有を避ける）。
type Player struct {
	Pos      Coord
	HP       int
	MaxHP    int
	Str      int
	MaxStr   int
	Exp      int
	Level    int
	Gold     int
	Hunger   int // 残り食料（0 で餓死）
	Pack     []*Object
	Weapon   int // Pack 添字。-1 = 素手
	Armor    int // Pack 添字。-1 = 裸
	LeftRing int // Pack 添字。-1
	RightRng int // Pack 添字。-1
	Confused int // 残りターン
	Blind    int
	Haste    int
	Levit    int // 浮遊
	Held     int // 押さえ込み・凍結で動けない残りターン
	Sees     bool
	RegenCnt int
}
