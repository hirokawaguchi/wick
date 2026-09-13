package session

import (
	"fmt"
	"strings"
)

// 複数行スクリーンエディタ。docs/screen-editor.md 参照。
// Enter/Shift+Enter は常に改行。送信は単独行の "." のみ。中止は Ctrl-C。

const (
	KeyNone = iota
	KeyRune
	KeyEnter
	KeyBackspace
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyCancel
)

// EditKey は編集用に解釈済みのキー。
type EditKey struct {
	Kind int
	R    rune
}

// ReadEditKey は 1 打鍵を編集用のキーコードに解釈して返す。
// 矢印などの ESC シーケンスを分解する。メニュー用の ReadKey とは別。
func (s *Session) ReadEditKey() (EditKey, error) {
	r, err := s.readRune()
	if err != nil {
		return EditKey{}, err
	}
	switch r {
	case '\r', '\n':
		if r == '\r' && s.reader().Buffered() > 0 {
			if p, e := s.reader().Peek(1); e == nil && p[0] == '\n' {
				_, _ = s.reader().ReadByte()
			}
		}
		return EditKey{Kind: KeyEnter}, nil
	case 0x7f, '\b':
		return EditKey{Kind: KeyBackspace}, nil
	case 0x03: // Ctrl-C
		return EditKey{Kind: KeyCancel}, nil
	case 0x01: // Ctrl-A
		return EditKey{Kind: KeyHome}, nil
	case 0x05: // Ctrl-E
		return EditKey{Kind: KeyEnd}, nil
	case 0x1b:
		return s.readEscSeq()
	}
	if r < 32 {
		return EditKey{Kind: KeyNone}, nil
	}
	return EditKey{Kind: KeyRune, R: r}, nil
}

func (s *Session) readEscSeq() (EditKey, error) {
	br := s.reader()
	if br.Buffered() == 0 {
		return EditKey{Kind: KeyNone}, nil // 単独 ESC は無視
	}
	b, err := br.ReadByte()
	if err != nil {
		return EditKey{}, err
	}
	if b != '[' && b != 'O' {
		return EditKey{Kind: KeyNone}, nil
	}
	c, err := br.ReadByte()
	if err != nil {
		return EditKey{}, err
	}
	switch c {
	case 'A':
		return EditKey{Kind: KeyUp}, nil
	case 'B':
		return EditKey{Kind: KeyDown}, nil
	case 'C':
		return EditKey{Kind: KeyRight}, nil
	case 'D':
		return EditKey{Kind: KeyLeft}, nil
	case 'H':
		return EditKey{Kind: KeyHome}, nil
	case 'F':
		return EditKey{Kind: KeyEnd}, nil
	}
	if c >= '0' && c <= '9' {
		num := []byte{c}
		for {
			d, err := br.ReadByte()
			if err != nil {
				return EditKey{}, err
			}
			if d >= '0' && d <= '9' {
				num = append(num, d)
				continue
			}
			switch string(num) {
			case "1", "7":
				return EditKey{Kind: KeyHome}, nil
			case "4", "8":
				return EditKey{Kind: KeyEnd}, nil
			}
			return EditKey{Kind: KeyNone}, nil
		}
	}
	return EditKey{Kind: KeyNone}, nil
}

// editorBuf は行の可変バッファ。row/col はカーソル位置（col はルーン単位）。
type editorBuf struct {
	lines [][]rune
	row   int
	col   int
}

func newEditorBuf(init string) *editorBuf {
	b := &editorBuf{}
	if init == "" {
		b.lines = [][]rune{{}}
	} else {
		for _, ln := range strings.Split(init, "\n") {
			b.lines = append(b.lines, []rune(ln))
		}
	}
	b.row = len(b.lines) - 1
	b.col = len(b.lines[b.row])
	return b
}

func (b *editorBuf) insertRune(r rune) {
	line := b.lines[b.row]
	out := make([]rune, 0, len(line)+1)
	out = append(out, line[:b.col]...)
	out = append(out, r)
	out = append(out, line[b.col:]...)
	b.lines[b.row] = out
	b.col++
}

func (b *editorBuf) newline() {
	line := b.lines[b.row]
	left := append([]rune{}, line[:b.col]...)
	right := append([]rune{}, line[b.col:]...)
	b.lines[b.row] = left
	rest := append([][]rune{right}, b.lines[b.row+1:]...)
	b.lines = append(b.lines[:b.row+1], rest...)
	b.row++
	b.col = 0
}

func (b *editorBuf) backspace() {
	if b.col > 0 {
		line := b.lines[b.row]
		out := append([]rune{}, line[:b.col-1]...)
		out = append(out, line[b.col:]...)
		b.lines[b.row] = out
		b.col--
		return
	}
	if b.row > 0 {
		prev := b.lines[b.row-1]
		cur := b.lines[b.row]
		b.col = len(prev)
		b.lines[b.row-1] = append(append([]rune{}, prev...), cur...)
		b.lines = append(b.lines[:b.row], b.lines[b.row+1:]...)
		b.row--
	}
}

func (b *editorBuf) left() {
	if b.col > 0 {
		b.col--
	} else if b.row > 0 {
		b.row--
		b.col = len(b.lines[b.row])
	}
}

func (b *editorBuf) right() {
	if b.col < len(b.lines[b.row]) {
		b.col++
	} else if b.row < len(b.lines)-1 {
		b.row++
		b.col = 0
	}
}

func (b *editorBuf) up() {
	if b.row > 0 {
		b.row--
		if b.col > len(b.lines[b.row]) {
			b.col = len(b.lines[b.row])
		}
	}
}

func (b *editorBuf) down() {
	if b.row < len(b.lines)-1 {
		b.row++
		if b.col > len(b.lines[b.row]) {
			b.col = len(b.lines[b.row])
		}
	}
}

func (b *editorBuf) text() string {
	parts := make([]string, len(b.lines))
	for i, ln := range b.lines {
		parts[i] = string(ln)
	}
	return strings.Join(parts, "\n")
}

// isDotLine はカーソル行が最終行かつ内容がちょうど "." か。送信判定用。
func (b *editorBuf) isDotLine() bool {
	return b.row == len(b.lines)-1 && string(b.lines[b.row]) == "."
}

// EditText は複数行本文をスクリーン編集で読む。init は既存本文（新規は空）。
// 戻り値 submitted=false は Ctrl-C 破棄。送信時は "." 行を除いた本文を返す（末尾改行は付けない）。
func (s *Session) EditText(init string, maxLineLen int) (string, bool, error) {
	if maxLineLen <= 0 {
		maxLineLen = 500
	}
	b := newEditorBuf(init)
	prevRows, prevCurRow := s.renderEditor(b, 0, 0, true)
	for {
		k, err := s.ReadEditKey()
		if err != nil {
			return "", false, err
		}
		switch k.Kind {
		case KeyRune:
			if k.R >= 32 && len(b.lines[b.row]) < maxLineLen {
				b.insertRune(k.R)
			}
		case KeyEnter:
			if b.isDotLine() {
				b.lines = b.lines[:b.row]
				if len(b.lines) == 0 {
					b.lines = [][]rune{{}}
				}
				b.row = len(b.lines) - 1
				b.col = len(b.lines[b.row])
				s.finishEditor(b, prevCurRow)
				return b.text(), true, nil
			}
			b.newline()
		case KeyBackspace:
			b.backspace()
		case KeyLeft:
			b.left()
		case KeyRight:
			b.right()
		case KeyUp:
			b.up()
		case KeyDown:
			b.down()
		case KeyHome:
			b.col = 0
		case KeyEnd:
			b.col = len(b.lines[b.row])
		case KeyCancel:
			s.finishEditor(b, prevCurRow)
			return "", false, nil
		}
		prevRows, prevCurRow = s.renderEditor(b, prevRows, prevCurRow, false)
	}
}

func escSeq(n int, c byte) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("\x1b[%d%c", n, c)
}

func runesWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		if wideRune(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// renderEditor はバッファを描画し、(行数, カーソルの画面行) を返す。
// initial=false のときは前回描画分を消してから描き直す。
func (s *Session) renderEditor(b *editorBuf, prevRows, prevCurRow int, initial bool) (int, int) {
	var sb strings.Builder
	if !initial {
		sb.WriteString(escSeq(prevCurRow, 'A')) // 領域先頭へ
		sb.WriteString("\r")
		sb.WriteString("\x1b[J") // カーソルから下を消去
	}
	for i := range b.lines {
		sb.WriteString(string(b.lines[i]))
		if i < len(b.lines)-1 {
			sb.WriteString("\n") // Print が \r\n に変換
		}
	}
	if up := (len(b.lines) - 1) - b.row; up > 0 {
		sb.WriteString(escSeq(up, 'A'))
	}
	sb.WriteString("\r")
	if col := runesWidth(b.lines[b.row][:b.col]); col > 0 {
		sb.WriteString(escSeq(col, 'C'))
	}
	s.Print(sb.String())
	return len(b.lines), b.row
}

// finishEditor は編集終了時に本文を確定表示し、カーソルを本文の下の新しい行へ送る。
func (s *Session) finishEditor(b *editorBuf, prevCurRow int) {
	var sb strings.Builder
	sb.WriteString(escSeq(prevCurRow, 'A'))
	sb.WriteString("\r\x1b[J")
	for i := range b.lines {
		sb.WriteString(string(b.lines[i]))
		sb.WriteString("\n")
	}
	s.Print(sb.String())
}
