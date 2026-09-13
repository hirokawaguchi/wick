package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrBadPassword      = errors.New("bad password")
	ErrTooManyNotes     = errors.New("too many notes")
	ErrTooManyResponses = errors.New("too many responses")
	ErrTooManyTalkLines = errors.New("too many talk lines")
	ErrTooManyMails     = errors.New("too many mails")
	ErrTooManyGroups    = errors.New("too many mail groups")
	ErrTooManyNews      = errors.New("too many news articles")
)

const (
	MaxNotes     = 9999
	MaxResponses = 9999
	MaxTalkRooms = 16
	MaxTalkLines = 9999
	TalkLineMax  = 80

	// 表示桁（セル）で数える上限。固定桁の列にそのまま収まるよう、
	// 入力・保存もこの桁数で切る（全角=2, 半角=1）。列幅の定義と共有する。
	MaxHandle = 16 // who / agent 一覧などの Handle 列
	MaxTitle  = 40 // ノート/talk/chat のタイトル列

	MaxInbox         = 200
	MaxMailSubject   = 40
	MaxMailGroups    = 8
	MaxGroupMembers  = 16
	MaxProfLines     = 16
	MaxNewsArticles  = 9999
	MaxNewsSubject   = 40
	MaxNewsGroups    = 16
	DefaultNewsGroup = "local"

	TalkOpen   = 0
	TalkClosed = 1
	TalkLocked = 2

	// MaskAll は誰でも（読み向け）。MaskMembers は gen/sys/cos だけ（書き込み向け）。
	// 見習い(pro=1<<12) とゲスト(gst=1<<13) は MaskMembers に入らないので書けない。
	MaskAll     uint32 = 0xffffffff
	MaskMembers uint32 = (1 << 31) | (1 << 15) | (1 << 14)
)

type User struct {
	ID           string
	PasswordHash string
	Handle       string
	Flags        uint32
	TLimit       int // 分。65535 は無制限
	PWErr        int
	Access       int
	Expert       int
	Prompt       string
	TermWidth    int
	TermHeight   int
	Esc          int
	LastLogin    *time.Time
	LastLogout   *time.Time
	LastMsgRead  *time.Time
	RealName     string
	Birthday     string
	Address      string
	Phone        string
	Comment      string
	ScanList     string
	Autosign     string
	Profile      string
	MailSave     bool
	Lang         string // 表示言語（ja / en）。空は既定(ja)扱い
}

func (u User) Unlimited() bool {
	return u.TLimit >= 65535
}

type AccessLog struct {
	ID             int64
	UserID         string
	Handle         string
	ConnectTime    time.Time
	DisconnectTime *time.Time
	Channel        string
	Reason         int
}

type Store interface {
	Close() error
	GetUser(ctx context.Context, id string) (User, error)
	Authenticate(ctx context.Context, id, password string) (User, error)
	IncPWErr(ctx context.Context, id string) error
	ClearPWErr(ctx context.Context, id string) error
	IncAccess(ctx context.Context, id string) error
	SetLoginTimes(ctx context.Context, id string, login, logout time.Time) error
	InsertLog(ctx context.Context, log AccessLog) error
	// StartLog は接続時に「切断時刻なし」の行を先行記録し、その行 ID を返す。
	StartLog(ctx context.Context, log AccessLog) (int64, error)
	// FinishLog は StartLog で得た ID の行に切断時刻と理由を書き込む。
	FinishLog(ctx context.Context, id int64, disconnect time.Time, reason int) error
	ListLogs(ctx context.Context, limit int) ([]AccessLog, error)
	ListUsers(ctx context.Context) ([]User, error)
	UpdateHandle(ctx context.Context, id, handle string) error
	UpdateExpert(ctx context.Context, id string, expert int) error
	UpdateTerminal(ctx context.Context, id string, width, height, esc int, prompt string) error
	UpdatePassword(ctx context.Context, id, password string) error
	UpdateSequencer(ctx context.Context, id string, t time.Time) error
	UpdatePrivate(ctx context.Context, id string, realName, birthday, address, phone, comment string) error
	UpdateScanList(ctx context.Context, id, list string) error
	UpdateAutosign(ctx context.Context, id, text string) error
	UpdateProfile(ctx context.Context, id, text string) error
	UpdateMailSave(ctx context.Context, id string, save bool) error
	UpdateLang(ctx context.Context, id, lang string) error
	CreateUser(ctx context.Context, u User, password string) error
	// AllocMemberID は prefix＋ゼロ詰め連番の会員IDを払い出す（例 prd00001）。
	AllocMemberID(ctx context.Context, prefix string, width int) (string, error)
	UpdateFlags(ctx context.Context, id string, flags uint32) error
	UpdateTLimit(ctx context.Context, id string, tlimit int) error
	SeedIfEmpty(ctx context.Context, password string) error
	SeedGuest(ctx context.Context, password string) error
	SeedAgents(ctx context.Context, password string) error
	SeedBoards(ctx context.Context) error
	// SeedTopics はボードに話題ベースノート（README 相当）を用意する。
	// ボードにノートが 1 件も無いときだけ全件作る（冪等）。
	SeedTopics(ctx context.Context, board, author, handle string, topics []NoteSeed) error
	// DeleteBoardNotes はボードのノートとレスを全消去する（保守用。scratch 板向け）。
	DeleteBoardNotes(ctx context.Context, board string) (int, error)

	ListBoards(ctx context.Context) ([]Board, error)
	GetBoard(ctx context.Context, name string) (Board, error)
	CreateBoard(ctx context.Context, b Board) (Board, error)
	UpdateBoard(ctx context.Context, b Board) error

	ListNotes(ctx context.Context, boardID int64) ([]Note, error)
	GetNote(ctx context.Context, boardID int64, num int) (Note, error)
	GetNoteByID(ctx context.Context, id int64) (Note, error)
	GetBoardByID(ctx context.Context, id int64) (Board, error)
	CreateNote(ctx context.Context, n Note) (Note, error)
	UpdateNote(ctx context.Context, n Note) error

	ListResponses(ctx context.Context, noteID int64) ([]Response, error)
	GetResponse(ctx context.Context, noteID int64, num int) (Response, error)
	CreateResponse(ctx context.Context, r Response) (Response, error)
	UpdateResponse(ctx context.Context, r Response) error

	SeedTalkRooms(ctx context.Context) error
	ListTalkRooms(ctx context.Context) ([]TalkRoom, error)
	GetTalkRoom(ctx context.Context, num int) (TalkRoom, error)
	UpdateTalkRoom(ctx context.Context, r TalkRoom) error
	ListTalkLines(ctx context.Context, room, fromNum int) ([]TalkLine, error)
	ListTalkLinesSince(ctx context.Context, room int, t time.Time) ([]TalkLine, error)
	CreateTalkLine(ctx context.Context, ln TalkLine) (TalkLine, error)

	SendMail(ctx context.Context, m Mail) (Mail, error)
	ListInbox(ctx context.Context, userID string) ([]Mail, error)
	ListSent(ctx context.Context, userID string) ([]Mail, error)
	ListWithdrawable(ctx context.Context, userID string) ([]Mail, error)
	GetMail(ctx context.Context, id int64) (Mail, error)
	MarkMailRead(ctx context.Context, id int64) error
	DeleteInboxMail(ctx context.Context, id int64, userID string) error
	KillMail(ctx context.Context, id int64, fromID string) error
	CountUnreadMail(ctx context.Context, userID string) (int, error)

	ListMailGroups(ctx context.Context, owner string) ([]MailGroup, error)
	GetMailGroup(ctx context.Context, owner, name string) (MailGroup, error)
	SaveMailGroup(ctx context.Context, g MailGroup) error
	DeleteMailGroup(ctx context.Context, owner, name string) error

	SeedNewsGroups(ctx context.Context) error
	ListNewsGroups(ctx context.Context) ([]NewsGroup, error)
	GetNewsGroup(ctx context.Context, name string) (NewsGroup, error)
	EnsureNewsGroup(ctx context.Context, name string) (NewsGroup, error)
	PostNews(ctx context.Context, a NewsArticle) (NewsArticle, error)
	ListNewsAfter(ctx context.Context, group string, after int) ([]NewsArticle, error)
	GetNewsArticle(ctx context.Context, group string, num int) (NewsArticle, error)
	NewsCursor(ctx context.Context, userID, group string) (int, error)
	SetNewsCursor(ctx context.Context, userID, group string, last int) error
	CountUnreadNews(ctx context.Context, userID string) (int, error)
	UnsubscribeNews(ctx context.Context, userID, group string) error
	NewsSubscribed(ctx context.Context, userID, group string) (bool, error)
}

const (
	MsgDeleted   = 0x0001
	MsgAWO       = 0x0002
	MsgImportant = 0x0004
	MsgClosed    = 0x0008
	MsgSubject   = 0x0010
)

type Board struct {
	ID         int64
	Name       string
	Desc       string
	Slug       string
	LastUpdate time.Time
	MsgCount   int
	Read       uint32
	Write      uint32
	Basenote   uint32
	Sign       string
}

func (b Board) CanRead(flags uint32) bool     { return flags&b.Read != 0 }
func (b Board) CanWrite(flags uint32) bool    { return flags&b.Write != 0 }
func (b Board) CanBasenote(flags uint32) bool { return flags&b.Basenote != 0 }

// NoteSeed は SeedTopics で作る話題ベースノート（題と説明本文）。
type NoteSeed struct {
	Title string
	Body  string
}

type Note struct {
	ID         int64
	BoardID    int64
	Num        int
	Title      string
	Author     string
	Handle     string
	PostTime   time.Time
	LastUpdate time.Time
	Flags      int
	Response   int
	Body       string
}

type Response struct {
	ID       int64
	NoteID   int64
	Num      int
	Title    string
	Author   string
	Handle   string
	PostTime time.Time
	Flags    int
	Body     string
}

type TalkRoom struct {
	Num        int
	Title      string
	Status     int
	Leader     string
	LineCount  int
	LastUpdate time.Time
}

func TalkStatusName(status int) string {
	switch status {
	case TalkClosed:
		return "Closed"
	case TalkLocked:
		return "Locked"
	default:
		return "Open"
	}
}

func TalkStatusShort(status int) string {
	switch status {
	case TalkClosed:
		return "Cl"
	case TalkLocked:
		return "Lk"
	default:
		return "Op"
	}
}

type Mail struct {
	ID         int64
	FromID     string
	FromHandle string
	ToID       string
	Subject    string
	Body       string
	SentAt     time.Time
	ReadAt     *time.Time
	InboxDel   bool
	Killed     bool
	Saved      bool
}

type MailGroup struct {
	Owner   string
	Name    string
	Members string
}

type NewsGroup struct {
	Name    string
	LastNum int
}

type NewsArticle struct {
	ID         int64
	Group      string
	Num        int
	FromID     string
	FromHandle string
	Subject    string
	Body       string
	Posted     time.Time
	RefNum     int
}

type TalkLine struct {
	ID       int64
	Room     int
	Num      int
	Author   string
	Handle   string
	PostTime time.Time
	Body     string
}
