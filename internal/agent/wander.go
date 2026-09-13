package agent

// wanderBrain は本来の頭脳(inner)を包み、inner が黙っている（idle）あいだ、
// MAIN にいるときに気晴らしとしてノートを読みに行く（open <board> で INDEX に入り、
// 数心拍のあいだ滞在してから q で戻る）。これにより who での居場所が
// 「MAIN だけ」ではなく「ノート閲覧中」を含めて人間のように動いて見える。
//
// 読取のみ（open で開いて q で戻るだけ）。書き込みや破壊的操作はしない。
// inner が実際の手（発話・投稿など）を返すときはそのまま通し、idle のときだけ
// 出歩く。model（チャット常駐）や talker（自分で TALK を巡回）のように
// 既に MAIN 以外にいる頭脳では発動しない。
type wanderBrain struct {
	inner  Brain
	boards func() []string // 読めるボード名（巡回先）
	selfID string

	phase     int // 0=通常, 1=どこかを閲覧中
	leaveTick int // この心拍に達したら退出する
	nextTick  int // この心拍まで出歩かない
	idx       int
	stay      int // 1 回の閲覧で滞在する心拍数
	gap       int // 次に出歩くまでの最短心拍数
}

func newWanderBrain(inner Brain, boards func() []string, selfID string) *wanderBrain {
	return &wanderBrain{inner: inner, boards: boards, selfID: selfID, stay: 5, gap: 20}
}

func (w *wanderBrain) Next(obs Observation) (string, bool) {
	// 閲覧中: 一定心拍で退出して MAIN へ戻る。
	if w.phase == 1 {
		if obs.Tick >= w.leaveTick {
			w.phase = 0
			w.nextTick = obs.Tick + w.gap
			return "q\n", false
		}
		return "", false // ノートを開いたまま滞在（who に居場所が出る）
	}
	// 通常: まず本来の頭脳に手を渡す。実際の手があればそれを優先。
	s, done := w.inner.Next(obs)
	if done {
		return s, true
	}
	if s != "" {
		return s, false
	}
	// inner が黙ったとき、MAIN にいるなら気晴らしにノートを読みに行く。
	if obs.Doing != "" && obs.Doing != "MAIN" {
		return "", false // 既にどこか（本来の場）にいる
	}
	if obs.Tick < w.nextTick {
		return "", false
	}
	bs := w.boards()
	if len(bs) == 0 {
		return "", false
	}
	b := bs[w.idx%len(bs)]
	w.idx++
	w.phase = 1
	w.leaveTick = obs.Tick + w.stay
	return "open " + b + "\n", false
}
