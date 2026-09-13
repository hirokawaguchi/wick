package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hirokawaguchi/wick/internal/i18n"
)

type Config struct {
	Listen        string
	DataDir       string
	AssetDir      string
	DBPath        string
	HostKey       string
	SeedPassword  string
	GuestPassword string
	MaxSessions   int
	MaxAuth       int
	Driver        string
	PGDSN         string
	TimeZone      string
	Lang          string // 局の既定表示言語（ja / en）。新規ユーザ・未設定時の既定

	// エージェントの実 Brain（OpenAI 互換 HTTP）。Endpoint 空なら偽頭脳のまま。
	AgentModelEndpoint string
	AgentModelKey      string
	AgentModelName     string
	AgentTokenBudget   int // 1 体あたりの累計トークン上限（0 で無制限）

	// エージェント専用 web 検索の MCP サーバ（Streamable HTTP）。URL 空ならスタブのまま。
	AgentWebMCPURL   string // 単一エンドポイント URL
	AgentWebMCPToken string // Authorization: Bearer（認証。空なら付けない）
	AgentWebMCPTool  string // 呼ぶツール名（既定 web_search）
	AgentWebMCPArg   string // クエリ引数名（既定 query）
}

// Load はサイト全体の設定を組み立てる。優先順位は
//
//	環境変数 > 設定ファイル（既定 data/etc/wick.conf） > 組み込み既定
//
// なので、Docker などで env を渡す運用はこれまでどおり動き（env が最優先）、
// ソース運用では 1 ファイルにまとめて置ける。設定ファイルのキー名は環境変数と
// 同じ（WICK_* / TZ）。ファイルの場所は WICK_CONFIG で変えられる。
func Load() Config {
	file := loadFile(configPath())
	// env が空なら設定ファイル、それも無ければ def を返す。
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		if v, ok := file[k]; ok && v != "" {
			return v
		}
		return def
	}
	getInt := func(k string, def int) int {
		if s := get(k, ""); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				return n
			}
		}
		return def
	}

	c := Config{
		Listen:        get("WICK_LISTEN", ":2222"),
		DataDir:       get("WICK_DATA", "data"),
		AssetDir:      get("WICK_ASSET", "data"),
		SeedPassword:  get("WICK_SEED_PASSWORD", "wick"),
		GuestPassword: get("WICK_GUEST_PASSWORD", "guest"),
		MaxSessions:   getInt("WICK_MAX_SESSIONS", 500),
		MaxAuth:       getInt("WICK_MAX_AUTH", 4),
		Driver:        get("WICK_DB_DRIVER", "sqlite"),
		PGDSN:         get("WICK_PG_DSN", ""),
		TimeZone:      get("WICK_TZ", get("TZ", "Asia/Tokyo")),
		Lang:          string(i18n.Normalize(get("WICK_LANG", "ja"))),

		AgentModelEndpoint: get("WICK_AGENT_MODEL_ENDPOINT", ""),
		AgentModelKey:      get("WICK_AGENT_MODEL_KEY", ""),
		AgentModelName:     get("WICK_AGENT_MODEL", ""),
		AgentTokenBudget:   getInt("WICK_AGENT_TOKEN_BUDGET", 0),

		AgentWebMCPURL:   get("WICK_AGENT_WEB_MCP_URL", ""),
		AgentWebMCPToken: get("WICK_AGENT_WEB_MCP_TOKEN", ""),
		AgentWebMCPTool:  get("WICK_AGENT_WEB_MCP_TOOL", "web_search"),
		AgentWebMCPArg:   get("WICK_AGENT_WEB_MCP_ARG", "query"),
	}
	c.DBPath = get("WICK_DB", c.DataDir+"/wick.db")
	c.HostKey = get("WICK_HOST_KEY", c.DataDir+"/ssh_host_ed25519")
	return c
}

// configPath は設定ファイルの場所を返す。WICK_CONFIG があればそれを、無ければ
// <WICK_DATA>/etc/wick.conf（既定 data/etc/wick.conf）。
func configPath() string {
	if p := os.Getenv("WICK_CONFIG"); p != "" {
		return p
	}
	data := os.Getenv("WICK_DATA")
	if data == "" {
		data = "data"
	}
	return data + "/etc/wick.conf"
}

// loadFile は KEY=VALUE 形式の設定ファイルを読む（dotenv 風）。
//   - `#` から行末はコメント。空行は無視。
//   - 先頭の `export ` は許容（. で読み込む運用と両立）。
//   - 値の前後の空白と、値全体を囲むダブル/シングルクォートは取り除く。
//
// ファイルが無ければ空マップ（＝すべて env と既定で解決）。
func loadFile(path string) map[string]string {
	m := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		if key != "" {
			m[key] = parseValue(strings.TrimSpace(line[i+1:]))
		}
	}
	return m
}

// parseValue は 1 個の値を解釈する。
//   - 値全体をダブル/シングルクォートで囲めば、中身をそのまま採用（`#` や空白も保持）。
//   - クォート無しのときは「空白の直後の `#`」から行末をコメントとして落とす
//     （`pass#word` のように前に空白の無い `#` は値の一部として残す）。
func parseValue(v string) string {
	if v == "" {
		return v
	}
	if q := v[0]; q == '"' || q == '\'' {
		if j := strings.IndexByte(v[1:], q); j >= 0 {
			return v[1 : 1+j]
		}
		return v[1:] // 閉じクォートが無ければ先頭のクォートだけ落とす
	}
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i])
		}
	}
	return v
}

// ApplyTimeZone は表示・入力のローカル時刻を局のタイムゾーンにする。
// 保存は Unix 時刻のまま。zoneinfo が無い環境では JST 固定にする。
func (c Config) ApplyTimeZone() (*time.Location, error) {
	name := c.TimeZone
	if name == "" {
		name = "Asia/Tokyo"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc = time.FixedZone("JST", 9*60*60)
	}
	time.Local = loc
	return loc, err
}
