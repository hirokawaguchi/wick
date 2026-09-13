package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/agent"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/config"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/mcp"
	"github.com/hirokawaguchi/wick/internal/sshd"
	"github.com/hirokawaguchi/wick/internal/store"
)

func main() {
	cfg := config.Load()
	if loc, err := cfg.ApplyTimeZone(); err != nil {
		log.Printf("timezone %s: using %s", cfg.TimeZone, loc)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	var st store.Store
	var err error
	if cfg.Driver == "postgres" {
		st, err = store.OpenPostgres(cfg.PGDSN)
	} else {
		st, err = store.OpenSQLite(cfg.DBPath)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	if err := st.SeedIfEmpty(ctx, cfg.SeedPassword); err != nil {
		log.Fatal(err)
	}
	if err := st.SeedGuest(ctx, cfg.GuestPassword); err != nil {
		log.Fatal(err)
	}
	if err := st.SeedAgents(ctx, cfg.SeedPassword); err != nil {
		log.Fatal(err)
	}
	lang := i18n.Normalize(cfg.Lang)
	if err := st.SeedBoards(ctx, lang); err != nil {
		log.Fatal(err)
	}
	if err := st.SeedTalkRooms(ctx); err != nil {
		log.Fatal(err)
	}
	if err := st.SeedNewsGroups(ctx); err != nil {
		log.Fatal(err)
	}
	// 保守用: WICK_RESET_BOARD（カンマ区切り）で指定した scratch 板のノートを一掃する。
	// 一度きりの掃除に使い、通常運用では空にしておく。
	if rb := os.Getenv("WICK_RESET_BOARD"); rb != "" {
		for _, name := range strings.Split(rb, ",") {
			if name = strings.TrimSpace(name); name != "" {
				if n, err := st.DeleteBoardNotes(ctx, name); err != nil {
					log.Printf("board %s: delete failed: %v", name, err)
				} else {
					log.Printf("board %s: deleted %d notes", name, n)
				}
			}
		}
	}
	// HyperNotes の慣習: junk.test に話題ベースノート（README 相当）を用意する。
	// 通常の書き込みはこの話題へのレスとして積む（ベースノートは増やさない）。
	if err := st.SeedTopics(ctx, "junk.test", "sysop", "Sysop", []store.NoteSeed{
		{Title: i18n.T(lang, "seed.topic.chat.t"), Body: i18n.T(lang, "seed.topic.chat.b")},
		{Title: i18n.T(lang, "seed.topic.book.t"), Body: i18n.T(lang, "seed.topic.book.b")},
		{Title: i18n.T(lang, "seed.topic.find.t"), Body: i18n.T(lang, "seed.topic.find.b")},
		{Title: i18n.T(lang, "seed.topic.poem.t"), Body: i18n.T(lang, "seed.topic.poem.b")},
	}); err != nil {
		log.Fatal(err)
	}

	tbl, err := acl.Load(cfg.AssetDir + "/etc")
	if err != nil {
		log.Fatal(err)
	}

	as := assets.Dir{Root: cfg.AssetDir}

	h := host.New(cfg.MaxSessions)

	// エージェント常駐（AgentIO）。AGENTS.txt を読み、auto 指定を起動する。
	mgr := agent.NewManager(h, st, tbl, as)
	mgr.SetLang(i18n.Normalize(cfg.Lang))
	mgr.SetModel(agent.ModelConfig{
		Endpoint:    cfg.AgentModelEndpoint,
		APIKey:      cfg.AgentModelKey,
		Model:       cfg.AgentModelName,
		TokenBudget: cfg.AgentTokenBudget,
	})
	// web 検索の MCP サーバが設定されていれば接続し、Provider を差し替える。
	// 失敗してもスタブのまま運用を続ける（起動は止めない）。
	if cfg.AgentWebMCPURL != "" {
		// 別サービス（websearch）が起動途中のことがあるので数回リトライしてから諦める。
		var cli *mcp.Client
		var err error
		for attempt := 1; attempt <= 5; attempt++ {
			if cli, err = mcp.DialTimeout(cfg.AgentWebMCPURL, cfg.AgentWebMCPToken, 10*time.Second); err == nil {
				break
			}
			log.Printf("web MCP waiting (%d/5): %v", attempt, err)
			time.Sleep(3 * time.Second)
		}
		if err != nil {
			log.Printf("web MCP failed (keeping stub): %v", err)
		} else {
			mgr.SetWebProvider(agent.NewMCPWebProvider(cli, cfg.AgentWebMCPTool, cfg.AgentWebMCPArg))
			auth := "no auth"
			if cfg.AgentWebMCPToken != "" {
				auth = "bearer"
			}
			log.Printf("web MCP connected: %s (%s, proto=%s)", cfg.AgentWebMCPURL, auth, cli.Protocol())
		}
	}
	if specs, err := agent.LoadSpecs(cfg.AssetDir + "/etc/AGENTS.txt"); err != nil {
		log.Printf("AGENTS.txt: %v", err)
	} else {
		for _, sp := range specs {
			mgr.Register(sp)
		}
		mgr.StartConfigured()
		mgr.StartJanitor() // sys.jobs の決着ジョブを定期的に自動クローズ
		if cfg.AgentModelEndpoint != "" {
			log.Printf("agents: %d registered, model=%s @ %s", len(specs), cfg.AgentModelName, cfg.AgentModelEndpoint)
		} else {
			log.Printf("agents: %d registered (no model = scripted brains)", len(specs))
		}
	}

	srv := &sshd.Server{
		Store:  st,
		Host:   h,
		ACL:    tbl,
		Assets: as,
		Agents: mgr,
		Cfg: sshd.Config{
			Listen:  cfg.Listen,
			HostKey: cfg.HostKey,
			MaxAuth: cfg.MaxAuth,
			Lang:    cfg.Lang,
		},
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			log.Print("shutting down (signal)")
		case <-h.ShutdownC():
			log.Print("shutting down (shutdown command)")
		}
		// 在室セッションを閉じ、切断ログを書き終えるまで最大 8 秒待つ（graceful drain）。
		sctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = srv.GracefulStop(sctx)
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Print(err)
	}
}
