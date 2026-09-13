package rogue

// ここは品物・モンスターの定義表。名前は Wick 所有の訳語（意味を原典に合わせる）。

// --- 武器 ---

type weaponDef struct {
	NameJA string
	NameEN string
	MHit   int    // 命中ダイス個数（d? は MDam 側で表現）
	MDam   string // 手持ち攻撃ダメージ（例 "2d4"）
	TDam   string // 投げたときのダメージ
	Group  int    // 発射体グループ（弓と矢を対応づける）。0=なし
}

const (
	grpNone = iota
	grpBow
	grpArrow
	grpDart
)

var weapons = []weaponDef{
	{"ほこ", "mace", 1, "2d4", "1d3", grpNone},
	{"つるぎ", "long sword", 1, "3d4", "1d2", grpNone},
	{"弓", "short bow", 1, "1d1", "1d1", grpBow},
	{"矢", "arrow", 1, "1d1", "2d3", grpArrow},
	{"短剣", "dagger", 1, "1d6", "1d4", grpNone},
	{"だんびら", "two-handed sword", 1, "4d5", "1d2", grpNone},
	{"投げ矢", "dart", 1, "1d1", "1d3", grpDart},
	{"手裏剣", "shuriken", 1, "1d2", "2d4", grpDart},
	{"やり", "spear", 1, "2d3", "1d6", grpNone},
}

// --- よろい ---

type armorDef struct {
	NameJA string
	NameEN string
	Class  int // 基本守備力（大きいほど強い）
}

var armors = []armorDef{
	{"革のよろい", "leather armor", 2},
	{"かたびら", "ring mail", 3},
	{"うろこのよろい", "scale mail", 4},
	{"鎖かたびら", "chain mail", 5},
	{"帯金のよろい", "banded mail", 6},
	{"延金のよろい", "splint mail", 6},
	{"鋼鉄のよろい", "plate mail", 7},
}

// --- 水薬 ---

type potionDef struct {
	NameJA string
	NameEN string
}

var potions = []potionDef{
	{"体力増強", "increase strength"},
	{"回復", "restore strength"},
	{"癒し", "healing"},
	{"特大回復", "extra healing"},
	{"毒", "poison"},
	{"経験増強", "raise level"},
	{"盲目", "blindness"},
	{"幻覚", "hallucination"},
	{"探知（食料）", "detect food"},
	{"探知（魔法）", "detect magic"},
	{"透視", "detect monster"},
	{"浮遊", "levitation"},
	{"俊足", "haste self"},
	{"透明感知", "see invisible"},
}

var potionColors = []string{
	"赤い", "青い", "緑の", "琥珀色の", "紫の", "透明な", "桃色の", "白い",
	"黒い", "黄の", "銀色の", "紅の", "瑠璃色の", "灰色の", "橙の", "水色の",
}

// --- 巻物 ---

type scrollDef struct {
	NameJA string
	NameEN string
}

var scrolls = []scrollDef{
	{"鑑定", "identify"},
	{"テレポート", "teleportation"},
	{"眠り", "sleep"},
	{"よろい強化", "enchant armor"},
	{"武器強化", "enchant weapon"},
	{"呪い除け", "remove curse"},
	{"モンスター創造", "create monster"},
	{"魔法地図", "magic mapping"},
	{"羊皮紙（無地）", "blank paper"},
	{"守り", "protect armor"},
	{"食料探知", "aggravate monster"},
	{"暗黒", "hold monster"},
}

// 巻物のでたらめな題（見知らぬ国の言葉）。
var scrollTitles = []string{
	"ゾルナ ケッシュ", "イグ ナサ トル", "ヴェクリ ムーン", "オルド パス",
	"フェンリ カ", "ブラ シェド", "クゥエル ザ", "ミル ノク ター",
	"ハザ ウィン", "セプ トロ", "ガル ヴィス", "ネブ クラン",
	"ソラ メク", "ティル ダン", "ウォズ ケリ", "ジン バロ",
}

// --- 指輪 ---

type ringDef struct {
	NameJA string
	NameEN string
}

var rings = []ringDef{
	{"体力増強の指輪", "add strength"},
	{"守りの指輪", "protection"},
	{"再生の指輪", "regeneration"},
	{"探知の指輪", "searching"},
	{"透明感知の指輪", "see invisible"},
	{"燃えぬ指輪", "maintain armor"},
	{"満腹の指輪", "slow digestion"},
	{"器用さの指輪", "dexterity"},
	{"かわしの指輪", "stealth"},
	{"呪いの指輪", "teleportation"},
}

var ringStones = []string{
	"ルビーの", "サファイアの", "エメラルドの", "真珠の", "ダイヤの",
	"翡翠の", "琥珀の", "瑪瑙の", "黒曜石の", "月長石の",
	"珊瑚の", "紫水晶の", "水晶の", "オパールの", "石榴石の", "碧玉の",
}

// --- 杖（棒） ---

type wandDef struct {
	NameJA string
	NameEN string
}

var wands = []wandDef{
	{"痛打の杖", "striking"},
	{"微光の杖", "lightning"},
	{"炎の杖", "fire"},
	{"冷気の杖", "cold"},
	{"鈍化の杖", "slow monster"},
	{"睡眠の杖", "sleep"},
	{"消滅の杖", "cancellation"},
	{"魔封じの杖", "polymorph"},
	{"透明化の杖", "make invisible"},
	{"混乱の杖", "confuse monster"},
}

var wandMaterials = []string{
	"樫の", "松の", "黒檀の", "白木の", "鉄の", "銀の", "真鍮の",
	"銅の", "鋼の", "水晶の", "骨の", "竹の", "白金の", "黒鉄の",
	"金の", "錫の",
}

// --- モンスター表（A-Z、26 種） ---

type monDef struct {
	Char   byte
	NameJA string
	NameEN string
	Carry  int // 品物を持つ確率（%）
	Flags  monFlag
	Exp    int
	HD     int    // HP ダイス個数（HP = HD d8）
	AC     int    // 守備の弱さ（大きいほどプレイヤーが当てやすい。原典の m_armor 準拠で低い＝堅い）
	Dmg    string // 攻撃ダメージ
	Depth  int    // 出現し始める階（おおよそ）
}

var monsters = []monDef{
	{'A', "こうもり", "aquator", 0, mfMean | mfRusts, 20, 5, 2, "0d0", 9},
	{'B', "こうもり", "bat", 0, mfFly | mfFlit, 1, 1, 3, "1d2", 1},
	{'C', "けんたうろす", "centaur", 15, 0, 17, 4, 4, "1d2/1d5/1d5", 7},
	{'D', "ドラゴン", "dragon", 100, mfMean | mfFlames, 5000, 10, -1, "1d8/1d8/3d10", 21},
	{'E', "うきめだま", "floating eye", 0, 0, 5, 1, 9, "0d0", 1},
	{'F', "食虫すみれ", "violet fungi", 0, mfMean | mfHolds, 85, 8, 3, "000d0", 8},
	{'G', "ノーム", "gnome", 10, 0, 8, 1, 5, "1d6", 3},
	{'H', "大男", "giant", 25, mfMean, 120, 9, 0, "2d3/2d3/2d5", 12},
	{'I', "こおりの怪", "ice monster", 0, mfFreezes, 15, 1, 9, "0d0", 4},
	{'J', "ジャッカル", "jackal", 0, mfMean, 2, 1, 7, "1d2", 2},
	{'K', "コボルド", "kobold", 0, mfMean, 1, 1, 7, "1d4", 1},
	{'L', "レプラコーン", "leprechaun", 0, mfStealGold, 10, 3, 8, "1d1", 6},
	{'M', "メドゥーサ", "medusa", 25, mfMean | mfConfuses, 200, 8, 2, "3d4/3d4/2d5", 18},
	{'N', "ニンフ", "nymph", 100, mfStealItem, 37, 3, 9, "0d0", 9},
	{'O', "オーク", "orc", 15, mfGreedy, 5, 1, 6, "1d8", 4},
	{'P', "ゆうれい", "phantom", 0, mfInvis | mfFlit, 120, 8, 3, "4d4", 15},
	{'Q', "けだもの", "quagga", 0, mfMean, 15, 3, 3, "1d5/1d5", 8},
	{'R', "がらがらへび", "rattlesnake", 0, mfMean | mfDrainStr, 9, 2, 3, "1d6", 3},
	{'S', "へび", "snake", 0, mfMean, 2, 1, 5, "1d3", 1},
	{'T', "トロル", "troll", 50, mfMean | mfRegen, 120, 6, 4, "1d8/1d8/2d6", 13},
	{'U', "一角の獣", "black unicorn", 33, mfMean, 190, 7, -2, "1d9/1d9/2d9", 17},
	{'V', "きゅうけつき", "vampire", 20, mfMean | mfRegen | mfDrainLife, 350, 8, 1, "1d10", 19},
	{'W', "亡霊", "wraith", 0, mfDrainExp, 55, 5, 4, "1d6", 14},
	{'X', "ザナドゥ", "xeroc", 30, 0, 100, 7, 7, "4d4", 16},
	{'Y', "イエティ", "yeti", 30, 0, 50, 4, 6, "1d6/1d6", 11},
	{'Z', "ゾンビ", "zombie", 0, mfMean, 6, 2, 8, "1d8", 6},
}

// 経験レベルに必要な経験値（1→2 は index 1）。
var expLevels = []int{
	0, 10, 20, 40, 80, 160, 320, 640, 1300, 2600, 5200, 10000,
	20000, 40000, 80000, 160000, 320000, 1000000, 3333333, 6666666, 10000000,
}
