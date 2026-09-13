package config

import (
	"os"
	"strconv"
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

func Load() Config {
	c := Config{
		Listen:        env("WICK_LISTEN", ":2222"),
		DataDir:       env("WICK_DATA", "data"),
		AssetDir:      env("WICK_ASSET", "data"),
		SeedPassword:  env("WICK_SEED_PASSWORD", "wick"),
		GuestPassword: env("WICK_GUEST_PASSWORD", "guest"),
		MaxSessions:   envInt("WICK_MAX_SESSIONS", 500),
		MaxAuth:       envInt("WICK_MAX_AUTH", 4),
		Driver:        env("WICK_DB_DRIVER", "sqlite"),
		PGDSN:         env("WICK_PG_DSN", ""),
		TimeZone:      env("WICK_TZ", env("TZ", "Asia/Tokyo")),
		Lang:          string(i18n.Normalize(env("WICK_LANG", "ja"))),

		AgentModelEndpoint: env("WICK_AGENT_MODEL_ENDPOINT", ""),
		AgentModelKey:      env("WICK_AGENT_MODEL_KEY", ""),
		AgentModelName:     env("WICK_AGENT_MODEL", ""),
		AgentTokenBudget:   envInt("WICK_AGENT_TOKEN_BUDGET", 0),

		AgentWebMCPURL:   env("WICK_AGENT_WEB_MCP_URL", ""),
		AgentWebMCPToken: env("WICK_AGENT_WEB_MCP_TOKEN", ""),
		AgentWebMCPTool:  env("WICK_AGENT_WEB_MCP_TOOL", "web_search"),
		AgentWebMCPArg:   env("WICK_AGENT_WEB_MCP_ARG", "query"),
	}
	c.DBPath = env("WICK_DB", c.DataDir+"/wick.db")
	c.HostKey = env("WICK_HOST_KEY", c.DataDir+"/ssh_host_ed25519")
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
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
