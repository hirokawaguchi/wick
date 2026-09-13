package sshd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/hirokawaguchi/wick/internal/acl"
	"github.com/hirokawaguchi/wick/internal/assets"
	"github.com/hirokawaguchi/wick/internal/command"
	"github.com/hirokawaguchi/wick/internal/host"
	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/reason"
	"github.com/hirokawaguchi/wick/internal/session"
	"github.com/hirokawaguchi/wick/internal/shell"
	"github.com/hirokawaguchi/wick/internal/store"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	Listen    string
	HostKey   string
	MaxAuth   int
	IdleAfter time.Duration
	Lang      string // 局の既定表示言語（認証前バナーに使う）
}

type Server struct {
	Store  store.Store
	Host   *host.Host
	ACL    *acl.Table
	Assets assets.Dir
	Agents command.AgentControl // エージェント常駐の制御（Env へ注入）
	Cfg    Config
	ssh    *ssh.Server
	wg     sync.WaitGroup // 在室ハンドラの完了待ち（graceful drain 用）
}

func (srv *Server) ListenAndServe() error {
	if srv.Cfg.Listen == "" {
		srv.Cfg.Listen = ":2222"
	}
	if srv.Cfg.MaxAuth <= 0 {
		srv.Cfg.MaxAuth = 4
	}
	signer, err := loadOrCreateHostKey(srv.Cfg.HostKey)
	if err != nil {
		return err
	}

	srv.ssh = &ssh.Server{
		Addr:            srv.Cfg.Listen,
		Handler:         srv.handle,
		HostSigners:     []ssh.Signer{signer},
		PasswordHandler: srv.password,
		BannerHandler:   srv.banner,
		Version:         "Wick",
		// ファイル転送はこの局では行わない。SFTP サブシステムは提供しない。
		// 外部にアップしたファイルは file attach <URL> でリンクを貼る。
	}
	log.Printf("wick ssh listening on %s", srv.Cfg.Listen)
	return srv.ssh.ListenAndServe()
}

func (srv *Server) Shutdown(ctx context.Context) error {
	if srv.ssh == nil {
		return nil
	}
	return srv.ssh.Shutdown(ctx)
}

// GracefulStop は停止時に (1) 新規接続を止め (2) 在室の人間セッションへ告知して閉じ
// (3) 各ハンドラが切断ログ(FinishLog)を書き終えるまで待つ。ctx 期限で待ちは打ち切る。
// これで再起動・停止でも「接続時ログ＋切断時更新」が取りこぼされない。
func (srv *Server) GracefulStop(ctx context.Context) error {
	if srv.ssh != nil {
		_ = srv.ssh.Shutdown(ctx) // 新規受付を止める
	}
	if srv.Host != nil {
		srv.Host.CloseHumans() // 在室へ言語別告知してハンドラを返させる
	}
	done := make(chan struct{})
	go func() { srv.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

// banner は認証前に全接続へ出す案内。ゲスト口からの登録手順を知らせる。
func (srv *Server) banner(ctx ssh.Context) string {
	lang := i18n.Normalize(srv.Cfg.Lang)
	text := i18n.T(lang, "sshd.banner")
	// 認証前なので個人設定は不明。局の既定言語（Cfg.Lang）で出す。
	if b, err := srv.Assets.ReadLang(srv.Cfg.Lang, "msg", "banner.msg"); err == nil && strings.TrimSpace(b) != "" {
		text = b
	}
	// SSH バナーは CRLF 区切りが無難。
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\r\n")
	if !strings.HasSuffix(text, "\r\n") {
		text += "\r\n"
	}
	return text
}

func (srv *Server) password(ctx ssh.Context, password string) bool {
	u, err := srv.Store.Authenticate(ctx, ctx.User(), password)
	if err != nil {
		if err == store.ErrBadPassword {
			_ = srv.Store.IncPWErr(ctx, ctx.User())
		} else {
			log.Printf("auth %s: %v", ctx.User(), err)
		}
		return false
	}
	ctx.SetValue("user", u)
	return true
}

func (srv *Server) handle(sess ssh.Session) {
	srv.wg.Add(1)
	defer srv.wg.Done()
	ctx := sess.Context()
	u, _ := ctx.Value("user").(store.User)
	if u.ID == "" {
		got, err := srv.Store.GetUser(ctx, sess.User())
		if err != nil {
			fmt.Fprint(sess, "login failed\r\n")
			_ = sess.Exit(1)
			return
		}
		u = got
	}

	ch := "ssh"
	if addr, ok := sess.RemoteAddr().(*net.TCPAddr); ok {
		ch = fmt.Sprintf("ssh:%s", addr.IP)
	}

	s := session.New(ch, sess, sess)
	s.User = u
	s.Lang = i18n.Normalize(u.Lang) // 表示言語（未設定は既定 ja）
	s.ID = sess.Context().SessionID()
	s.SetConn(sess) // kill 用に下位接続を登録
	// pty の有無でサーバエコーを切り替える。pty 無し（cooked な行モード）の
	// クライアントは自分でローカルエコーするので、サーバはエコーしない（二重化防止）。
	if _, _, isPty := sess.Pty(); !isPty {
		s.SetPTY(false)
	}

	if !srv.Host.TryEnter(s) {
		s.Print("\n" + s.T("sshd.dup_id") + "\n")
		disc := time.Now()
		_ = srv.Store.InsertLog(ctx, store.AccessLog{
			UserID: u.ID, Handle: u.Handle,
			ConnectTime: s.Connected, DisconnectTime: &disc,
			Channel: ch, Reason: reason.Duplicate,
		})
		_ = sess.Exit(1)
		return
	}
	defer srv.Host.Leave(u.ID)

	_ = srv.Store.ClearPWErr(ctx, u.ID)
	_ = srv.Store.IncAccess(ctx, u.ID)

	// 接続時に先行してログ行を記録する（切断時刻なし＝online）。
	// これで再起動・回線断で異常終了しても「ログインした事実」が残る。
	// ctx はセッション文脈なので、切断時の更新には局全体の背景 ctx を使う。
	logID, _ := srv.Store.StartLog(context.Background(), store.AccessLog{
		UserID: u.ID, Handle: u.Handle,
		ConnectTime: s.Connected, Channel: ch, Reason: reason.Online,
	})

	run := &shell.Runner{
		Host:   srv.Host,
		Store:  srv.Store,
		ACL:    srv.ACL,
		Assets: srv.Assets,
		Agents: srv.Agents,
	}
	sctx, cancel := s.Context(ctx)
	defer cancel()

	code := run.Run(sctx, s)
	now := time.Now()
	// 切断時の書き込みはセッション ctx が切れていても行いたいので背景 ctx を使う。
	bg := context.Background()
	_ = srv.Store.UpdateSequencer(bg, u.ID, s.Sequencer)
	_ = srv.Store.SetLoginTimes(bg, u.ID, s.Login, now)
	if logID > 0 {
		_ = srv.Store.FinishLog(bg, logID, now, code)
	} else {
		// 先行記録に失敗していたら従来どおり 1 行で残す。
		_ = srv.Store.InsertLog(bg, store.AccessLog{
			UserID: u.ID, Handle: u.Handle,
			ConnectTime: s.Connected, DisconnectTime: &now,
			Channel: ch, Reason: code,
		})
	}
	s.Close()
	_ = sess.Exit(0)
}

func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if path == "" {
		path = "data/ssh_host_ed25519"
	}
	if b, err := os.ReadFile(path); err == nil {
		return gossh.ParsePrivateKey(b)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	b, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(b)
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(pemBytes)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
