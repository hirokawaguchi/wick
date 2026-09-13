package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Spec は 1 体分の設定（AGENTS.txt の 1 行）。
type Spec struct {
	ID        string
	Behavior  string        // greeter / chatter / ...
	Heartbeat time.Duration // 心拍間隔
	Budget    int           // 最大操作回数（-1 で無制限）
	AutoStart bool          // 起動時に自動で動かすか
	Board     string        // notes 対象ボード
	Room      int           // chat 部屋
	Target    string        // 電報/mail 宛先
	Interval  time.Duration // poster の投稿間隔 / worker の再割り当て待ち（interval=秒）
	Interests []string      // poster の興味キーワード（interest=詩,月,夜）

	// ActiveSet が真なら、自発的な発話は毎日 [ActiveStart, ActiveEnd) の時間帯だけ
	// （分単位・ローカルTZ）。窓の外は受け身（話しかけられたら返す）にする。
	// active=21:00-21:30 のように指定する。
	ActiveSet   bool
	ActiveStart int // 0:00 からの分
	ActiveEnd   int // 0:00 からの分

	// Rooms は talker が巡回する talk 部屋番号（rooms=1,2,3 または rooms=1-10）。
	Rooms []int

	// Web はエージェント専用の web 検索ツールを許すか（web=on）。sysop 設定のみ。
	// WebGet は URL 取得（web-get）を許すか（webget=on。より重く危険なので別能力）。
	// WebBudget は 1 体あたりの検索＋取得の合計回数上限（webbudget=、0 なら既定 20）。
	Web       bool
	WebGet    bool
	WebBudget int
}

// LoadSpecs は AGENTS.txt を読む。1 行 1 体。書式:
//
//	# id  behavior  heartbeat_sec  budget  [auto] [key=val ...]
//	scout   greeter  3  3  auto target=sysop board=junk.test
//	poet    chatter  4  4       room=1
//
// 追加キー: room= / rooms=1-10 / board= / target= / interval= / interest=a,b /
// active=21:00-21:30 / web=on / webget=on / webbudget=20（web/webget はエージェント専用の
// 検索・取得の許可。いずれも既定 off。webget は MCP サーバ側で web_get 有効時のみ働く）。
//
// ファイルが無ければ空スライス（エージェント無し）。
func LoadSpecs(path string) ([]Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Spec
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sp := Spec{ID: strings.ToLower(fields[0]), Behavior: strings.ToLower(fields[1]), Budget: -1}
		if len(fields) >= 3 {
			if n, err := strconv.Atoi(fields[2]); err == nil && n > 0 {
				sp.Heartbeat = time.Duration(n) * time.Second
			}
		}
		if len(fields) >= 4 {
			if n, err := strconv.Atoi(fields[3]); err == nil {
				sp.Budget = n
			}
		}
		kv := splitKV(fields[4:])
		for _, f := range fields[4:] {
			if strings.EqualFold(f, "auto") {
				sp.AutoStart = true
			}
		}
		sp.Board = kv["board"]
		sp.Target = kv["target"]
		if r, err := strconv.Atoi(kv["room"]); err == nil {
			sp.Room = r
		}
		if n, err := strconv.Atoi(kv["interval"]); err == nil && n > 0 {
			sp.Interval = time.Duration(n) * time.Second
		}
		if v := kv["interest"]; v != "" {
			for _, kw := range strings.Split(v, ",") {
				if kw = strings.TrimSpace(kw); kw != "" {
					sp.Interests = append(sp.Interests, kw)
				}
			}
		}
		if v := kv["active"]; v != "" {
			if s, e, ok := parseWindow(v); ok {
				sp.ActiveSet, sp.ActiveStart, sp.ActiveEnd = true, s, e
			}
		}
		if v := kv["rooms"]; v != "" {
			sp.Rooms = parseRooms(v)
		}
		if v := kv["web"]; v != "" {
			sp.Web = strings.EqualFold(v, "on") || v == "1" || strings.EqualFold(v, "true")
		}
		if v := kv["webget"]; v != "" {
			sp.WebGet = strings.EqualFold(v, "on") || v == "1" || strings.EqualFold(v, "true")
		}
		if n, err := strconv.Atoi(kv["webbudget"]); err == nil && n >= 0 {
			sp.WebBudget = n
		}
		out = append(out, sp)
	}
	return out, sc.Err()
}

// parseWindow は "21:00-21:30" を開始・終了の分（0:00 からの分）に変換する。
func parseWindow(s string) (start, end int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(s), "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := parseHM(parts[0])
	end, ok2 := parseHM(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return start, end, true
}

func parseHM(s string) (int, bool) {
	hm := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(hm) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(hm[0])
	m, err2 := strconv.Atoi(hm[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// parseRooms は "1,2,3" または "1-10" を部屋番号スライスに展開する。
func parseRooms(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexByte(part, '-'); i > 0 {
			lo, err1 := strconv.Atoi(strings.TrimSpace(part[:i]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(part[i+1:]))
			if err1 == nil && err2 == nil && lo >= 1 && hi >= lo {
				for n := lo; n <= hi; n++ {
					out = append(out, n)
				}
			}
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 1 {
			out = append(out, n)
		}
	}
	return out
}
