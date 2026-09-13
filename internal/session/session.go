package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hirokawaguchi/wick/internal/i18n"
	"github.com/hirokawaguchi/wick/internal/store"
)

// ErrInterrupt は入力中に Ctrl-C が押されたことを表す。
// 「今の処理を中止」する合図。メニュー階層を上がるのは "." で別扱い。
var ErrInterrupt = errors.New("interrupt")

// セッション種別。人間は SSH コンソール、agent はホスト内エージェント（AgentIO）。
const (
	KindHuman = "human"
	KindAgent = "agent"
)

type Session struct {
	ID      string
	Channel string
	Chan    int       // who と ! で使う回線番号。1 から
	Kind    string    // KindHuman / KindAgent。既定は human
	Lang    i18n.Lang // 表示言語。空は既定(ja)。ログイン時に User.Lang から設定
	User    store.User
	In      io.Reader
	Out     io.Writer
	br      *bufio.Reader

	doing     string // 現在の場所（who/ps 表示）。SetDoing/GetDoing で同期
	ChatRoom  int
	TalkRoom  int
	NoteBoard string
	NoteNum   int
	NoteID    int64
	Connected time.Time
	Login     time.Time
	Sequencer time.Time

	notices chan Notice

	// pty はクライアントに擬似端末（pty）が割り当てられているか。pty があるとき、
	// クライアント端末は raw モード（ローカルエコー無し）なので、サーバが打鍵を
	// エコーし、ANSI で入力行を再描画する。pty が無い（cooked な行モードの）
	// クライアントはローカルエコーするので、サーバはエコーしない（＝二重化を防ぐ）。
	// 既定は true。sshd が pty 無し接続を検出したら SetPTY(false) にする。
	pty bool

	// 入力中の再描画用（チャットのラインモード）。プロンプト付き読取の間だけ有効。
	// コマンドループと同じゴルーチンからのみ触るので追加ロックは不要。
	inputActive bool
	inputPrompt string
	inputBuf    *[]rune

	// 全画面（rogue など）表示中は通知を即描画せず保留する。画面が壊れないよう、
	// ゲーム側が TakeHeldNotices で取り出してメッセージ行に自前で出す。
	holdNotices bool
	heldNotices []Notice

	mu     sync.Mutex
	closed bool
	cancel context.CancelFunc
	conn   io.Closer
}

// SetDoing は現在の場所（who/ps 表示）を安全に設定する。
// コマンドループとは別ゴルーチン（who / agent list）から読まれるため同期する。
func (s *Session) SetDoing(d string) {
	s.mu.Lock()
	s.doing = d
	s.mu.Unlock()
}

// GetDoing は現在の場所を安全に取得する。
func (s *Session) GetDoing() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doing
}

// SetConn は強制切断（kill）用に、下位の接続クローザを登録する。
func (s *Session) SetConn(c io.Closer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = c
}

func New(channel string, in io.Reader, out io.Writer) *Session {
	return &Session{
		Channel:   channel,
		Kind:      KindHuman,
		In:        in,
		Out:       out,
		Connected: time.Now(),
		notices:   make(chan Notice, 16),
		pty:       true, // 既定は pty あり（サーバエコー）。pty 無し接続は sshd が下げる
	}
}

// SetPTY はクライアント pty の有無を設定する。false のとき、サーバは打鍵を
// エコーせず（クライアントがローカルエコーする前提）、入力行の ANSI 再描画も
// 行わない（cooked な行モード端末での二重表示・画面崩れを避ける）。
func (s *Session) SetPTY(v bool) { s.pty = v }

// HasPTY は pty ありかどうか。
func (s *Session) HasPTY() bool { return s.pty }

// IsAgent はエージェント（AgentIO）セッションかどうか。
func (s *Session) IsAgent() bool { return s.Kind == KindAgent }

// NewAgent は在室表（who）に出すためのエージェントセッションを作る。
// 出力は捨て、入力は即 EOF。掲示板への書き込みは監督が Store 経由で行う（第一弾）。
func NewAgent(id, handle string) *Session {
	s := New("agent", strings.NewReader(""), io.Discard)
	s.Kind = KindAgent
	s.User.ID = id
	s.User.Handle = handle
	return s
}

// NewPipe はエージェント（AgentIO）用のセッションと、その入出力口を返す。
// 入力はパイプ（Feed でキー列を流す）、出力は非ブロッキングな観測シンク。
// 人間のコマンドループ（menu.Enter）をそのまま回すための土台。
//
// 出力を io.Pipe（同期）にすると、Print が読み手待ちで固まりループが止まる。
// そこで Out は上限付きバッファにして、Print が決してブロックしないようにする。
func NewPipe(channel string, id, handle string) (*Session, *AgentIO) {
	inR, inW := io.Pipe()
	sink := &outSink{}
	s := New(channel, inR, sink)
	s.Kind = KindAgent
	s.User.ID = id
	s.User.Handle = handle
	io := &AgentIO{in: inW, sink: sink}
	s.conn = io // Session.Close -> AgentIO.Close で入力を閉じ EOF 終了
	return s, io
}

// AgentIO はエージェントセッションの入出力口。
type AgentIO struct {
	in   *io.PipeWriter
	sink *outSink
}

// Feed はキー列（コマンド文字列）を入力へ流し込む。
func (a *AgentIO) Feed(s string) error {
	_, err := io.WriteString(a.in, s)
	return err
}

// Snapshot は現在の画面出力を消さずに返す（観測用）。
func (a *AgentIO) Snapshot() string { return a.sink.snapshot() }

// Drain は現在の画面出力を返して空にする。
func (a *AgentIO) Drain() string { return a.sink.drain() }

// Close は入力パイプを閉じ、コマンドループを EOF 終了させる。
func (a *AgentIO) Close() error { return a.in.Close() }

const agentOutCap = 64 * 1024

// outSink は上限付きの非ブロッキング出力バッファ。
type outSink struct {
	mu  sync.Mutex
	buf []byte
}

func (o *outSink) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.buf = append(o.buf, p...)
	if len(o.buf) > agentOutCap {
		o.buf = o.buf[len(o.buf)-agentOutCap:]
	}
	return len(p), nil
}

func (o *outSink) snapshot() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.buf)
}

func (o *outSink) drain() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	s := string(o.buf)
	o.buf = o.buf[:0]
	return s
}

func (s *Session) Context(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	return ctx, cancel
}

func (s *Session) Print(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	_, _ = io.WriteString(s.Out, strings.ReplaceAll(text, "\n", "\r\n"))
}

func (s *Session) Printf(format string, args ...any) {
	s.Print(fmt.Sprintf(format, args...))
}

// T はこのセッションの表示言語でメッセージを引く（i18n カタログ）。
// s.Lang が空なら既定(ja)にフォールバックする。
func (s *Session) T(key string, args ...any) string {
	return i18n.T(s.Lang, key, args...)
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *Session) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Seen は未読境界を t まで進める。
func (s *Session) Seen(t time.Time) {
	if t.After(s.Sequencer) {
		s.Sequencer = t
	}
}

func (s *Session) reader() *bufio.Reader {
	if s.br == nil {
		s.br = bufio.NewReader(s.In)
	}
	return s.br
}

// readRune は 1 ルーン待つ。待ち中に電報・チャットが来たら割り込んで表示する。
func (s *Session) readRune() (rune, error) {
	type rec struct {
		r rune
		e error
	}
	ch := make(chan rec, 1)
	go func() {
		r, _, err := s.reader().ReadRune()
		ch <- rec{r, err}
	}()
	for {
		s.DrainNotices()
		select {
		case n := <-s.notices:
			s.emitNotice(n)
		case x := <-ch:
			return x.r, x.e
		}
	}
}

// ReadKey はコマンド 1 キーを読む。本文入力には使わない。
// CR は '\n'。エコーしない。全角スペース・全角英数は半角コマンドに寄せる。
func (s *Session) ReadKey() (byte, error) {
	r, err := s.readRune()
	if err != nil {
		return 0, err
	}
	if r == '\r' {
		if s.reader().Buffered() > 0 {
			if peek, err := s.reader().Peek(1); err == nil && peek[0] == '\n' {
				_, _ = s.reader().ReadByte()
			}
		}
		return '\n', nil
	}
	if r == 0x1b && s.reader().Buffered() > 0 {
		if peek, err := s.reader().Peek(1); err == nil && peek[0] == '[' {
			_, _ = s.reader().ReadByte()
			for {
				x, err := s.reader().ReadByte()
				if err != nil || (x >= 0x40 && x <= 0x7e) {
					break
				}
			}
		}
		return 0x1b, nil
	}
	return mapCommandKey(r), nil
}

func foldCommandRune(r rune) rune {
	switch r {
	case '\u00a0', '\u2002', '\u2003', '\u2007', '\u2008', '\u2009', '\u200a', '\u202f', '\u205f', '\u3000':
		return ' '
	}
	if r >= 0xff01 && r <= 0xff5e {
		return r - 0xff01 + '!'
	}
	return r
}

func mapCommandKey(r rune) byte {
	r = foldCommandRune(r)
	if r <= 0xff {
		return byte(r)
	}
	return 0
}

// FoldCommand はメニューや番号入力向け。本文・タイトルには使わない。
func FoldCommand(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(foldCommandRune(r))
	}
	return b.String()
}

// KeyWaiting はバッファにキーがあるか。ノンストップ解除用。
func (s *Session) KeyWaiting() bool {
	if s.br == nil {
		return false
	}
	return s.br.Buffered() > 0
}

// ReadLine は本文・タイトル用。UTF-8 をそのまま返す。全角は変換しない。
func (s *Session) ReadLine(max int) (string, error) {
	return s.readLine(max, true, "")
}

// ReadLinePrompt は自前でプロンプトを出す行入力。着信で割り込まれても、
// 通知のあとにプロンプトと打ちかけを描き直す（チャットのラインモード用）。
func (s *Session) ReadLinePrompt(prompt string, max int) (string, error) {
	return s.readLine(max, true, prompt)
}

// ReadCommand はメニュー・番号・確認用。エコーは入力どおり、返す値だけ半角に寄せる。
func (s *Session) ReadCommand(max int) (string, error) {
	line, err := s.readLine(max, true, "")
	if err != nil {
		return "", err
	}
	return FoldCommand(line), nil
}

// ReadSecret はパスワード用。エコーしない。
func (s *Session) ReadSecret(max int) (string, error) {
	return s.readLine(max, false, "")
}

func (s *Session) readLine(max int, echo bool, prompt string) (string, error) {
	if max <= 0 {
		max = 256
	}
	// pty が無いクライアント（cooked な行モード）は自分でローカルエコーするため、
	// サーバはエコーしない（打鍵が二重に見えるのを防ぐ）。
	if !s.pty {
		echo = false
	}
	var runes []rune
	if prompt != "" {
		s.Print(prompt)
		s.inputActive = true
		s.inputPrompt = prompt
		s.inputBuf = &runes
		defer func() {
			s.inputActive = false
			s.inputBuf = nil
		}()
	}
	for {
		r, err := s.readRune()
		if err != nil {
			if len(runes) == 0 {
				return "", err
			}
			return string(runes), err
		}
		switch r {
		case 0x03: // Ctrl-C 中止
			if echo {
				s.Print("^C\n")
			}
			return "", ErrInterrupt
		case '\n':
			if echo {
				s.Print("\n")
			}
			return string(runes), nil
		case '\r':
			if s.reader().Buffered() > 0 {
				if peek, err := s.reader().Peek(1); err == nil && peek[0] == '\n' {
					_, _ = s.reader().ReadByte()
				}
			}
			if echo {
				s.Print("\n")
			}
			return string(runes), nil
		case 0x7f, '\b':
			if n := len(runes); n > 0 {
				last := runes[n-1]
				runes = runes[:n-1]
				if echo {
					s.Print("\b \b")
					if wideRune(last) {
						s.Print("\b \b")
					}
				}
			}
		case 0, utf8.RuneError:
			continue
		default:
			if r < 32 {
				continue
			}
			if len(runes) < max {
				runes = append(runes, r)
				if echo {
					s.Print(string(r))
				}
			}
		}
	}
}

func wideRune(r rune) bool {
	return r >= 0x1100 && (r < 0x2000 || r >= 0x2E80)
}
