package session

import (
	"fmt"
	"strings"
	"time"
)

const (
	NoticeTelegram  = 1
	NoticeChat      = 2
	NoticeChatJoin  = 3
	NoticeChatLeave = 4
	NoticeTalk      = 5
	NoticeTalkJoin  = 6
	NoticeTalkLeave = 7
	NoticeTalkKnock = 8
	NoticeTalkAdmit = 9
	NoticeSystem    = 10
)

const TelegramMax = 78

// ChatMax はチャット発言 1 行の最大文字数（runes）。電報より長めに許して、
// 出典 URL＋コメントが 1 発言に収まるようにする（表示は FoldLine で桁折り）。
const ChatMax = 240

type Notice struct {
	Kind   int
	FromID string
	Handle string
	Time   time.Time
	Body   string
	Room   int
	Title  string
	Line   int
}

func ClipRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}

func (s *Session) Notify(n Notice) {
	if s == nil || s.notices == nil {
		return
	}
	select {
	case s.notices <- n:
	default:
	}
}

func (s *Session) DrainNotices() {
	if s == nil || s.notices == nil {
		return
	}
	for {
		select {
		case n := <-s.notices:
			s.emitNotice(n)
		default:
			return
		}
	}
}

// PrintNotice は 1 件の通知をそのまま（前後に改行を付けて）表示する。
func (s *Session) PrintNotice(n Notice) {
	s.Print(s.noticeText(n))
}

// cols は表示に使う桁数（term_width、未設定は 80）。
func (s *Session) cols() int {
	if s != nil && s.User.TermWidth > 0 {
		return s.User.TermWidth
	}
	return 80
}

// FoldLine は「prefix + 本文」を端末幅で折り返した表示文字列を返す（末尾改行つき）。
// 2 行目以降は prefix と同じ幅だけ字下げして、発言者名の下に本文を揃える。
// 会話（チャット/talk）の発言を切らずに全文表示するのに使う。
func (s *Session) FoldLine(prefix, body string) string {
	cols := s.cols()
	avail := cols - DisplayWidth(prefix)
	if avail < 8 { // prefix が広すぎる場合は字下げせず全幅で折る
		avail = cols
	}
	indent := strings.Repeat(" ", cols-avail)
	var b strings.Builder
	for i, ln := range WrapWidth(body, avail) {
		if i == 0 {
			b.WriteString(prefix)
		} else {
			b.WriteString(indent)
		}
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}

// emitNotice は通知を表示する。人間がチャットで入力中（プロンプト付き読取中）は、
// 入力行を一度消してから通知を出し、プロンプトと打ちかけの入力を描き直す
// （ラインモードでも入出力が読めるようにする＝A案）。エージェントや通常時は素通し。
func (s *Session) emitNotice(n Notice) {
	if s.inputActive && !s.IsAgent() {
		body := strings.TrimPrefix(s.noticeText(n), "\n")
		buf := ""
		if s.inputBuf != nil {
			buf = string(*s.inputBuf)
		}
		// \r + 行消去で入力行を消す → 通知 → プロンプト＋入力中を再描画。
		s.Print("\r\x1b[K" + body + s.inputPrompt + buf)
		return
	}
	s.PrintNotice(n)
}

// noticeText は通知の表示文字列（先頭に改行、末尾に改行）を返す。
// 会話系（電報・チャット・talk）の本文は端末幅で折り返し、切らずに全文を出す。
func (s *Session) noticeText(n Notice) string {
	switch n.Kind {
	case NoticeTelegram:
		when := n.Time.Format("15:04:05")
		head := "\n" + s.T("notice.telegram", n.FromID, n.Handle, when) + "\n"
		return head + s.FoldLine("", n.Body)
	case NoticeChat:
		return "\n" + s.FoldLine(fmt.Sprintf("%s> ", n.FromID), n.Body)
	case NoticeChatJoin:
		return "\n" + s.T("notice.join", n.FromID) + "\n"
	case NoticeChatLeave:
		return "\n" + s.T("notice.leave", n.FromID) + "\n"
	case NoticeTalk:
		return "\n" + s.FoldLine(fmt.Sprintf("%4d %s> ", n.Line, n.FromID), n.Body)
	case NoticeTalkJoin:
		return "\n" + s.T("notice.join", n.FromID) + "\n"
	case NoticeTalkLeave:
		return "\n" + s.T("notice.leave", n.FromID) + "\n"
	case NoticeTalkKnock:
		return "\n" + s.T("notice.knock", n.FromID) + "\n"
	case NoticeTalkAdmit:
		return "\n" + s.T("notice.admit", n.FromID) + "\n"
	case NoticeSystem:
		return "\n" + s.T("notice.system", n.Body) + "\n"
	}
	return ""
}
