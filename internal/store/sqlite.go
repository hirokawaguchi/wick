package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db     *sql.DB
	dollar bool
}

func OpenSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) q(query string) string {
	if !s.dollar {
		return query
	}
	n := 0
	var b strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

func (s *SQLite) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.q(query), args...)
}

func (s *SQLite) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.q(query), args...)
}

func (s *SQLite) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.q(query), args...)
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) migrate() error {
	logID := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if s.dollar {
		logID = "BIGSERIAL PRIMARY KEY"
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	password_hash TEXT NOT NULL,
	handle TEXT NOT NULL,
	flags INTEGER NOT NULL,
	tlimit INTEGER NOT NULL,
	pwerr INTEGER NOT NULL DEFAULT 0,
	access INTEGER NOT NULL DEFAULT 0,
	last_login INTEGER,
	last_logout INTEGER,
	expert INTEGER NOT NULL DEFAULT 0,
	prompt TEXT NOT NULL DEFAULT '',
	term_width INTEGER NOT NULL DEFAULT 80,
	term_height INTEGER NOT NULL DEFAULT 24,
	esc INTEGER NOT NULL DEFAULT 1,
	last_msgread INTEGER
);
CREATE TABLE IF NOT EXISTS access_logs (
	id ` + logID + `,
	user_id TEXT,
	handle TEXT,
	connect_time INTEGER NOT NULL,
	disconnect_time INTEGER,
	channel TEXT,
	reason INTEGER NOT NULL
);
`)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE users ADD COLUMN expert INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN prompt TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN term_width INTEGER NOT NULL DEFAULT 80`,
		`ALTER TABLE users ADD COLUMN term_height INTEGER NOT NULL DEFAULT 24`,
		`ALTER TABLE users ADD COLUMN esc INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE users ADD COLUMN last_msgread INTEGER`,
		`ALTER TABLE users ADD COLUMN real_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN birthday TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN address TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN phone TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN comment TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN scan_list TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN autosign TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN profile TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN mail_save INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE boards ADD COLUMN sign TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE news_cursors ADD COLUMN unsub INTEGER NOT NULL DEFAULT 0`,
		// ファイル添付は廃止。旧テーブルは掃除する。
		`DROP TABLE IF EXISTS attachments`,
	} {
		_, _ = s.db.Exec(stmt)
	}
	if _, err := s.db.Exec(`SELECT last_msgread FROM users LIMIT 1`); err != nil {
		if _, err2 := s.db.Exec(`ALTER TABLE users ADD COLUMN last_msgread INTEGER`); err2 != nil {
			return fmt.Errorf("last_msgread: %w", err2)
		}
	}
	idType := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if s.dollar {
		idType = "BIGSERIAL PRIMARY KEY"
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS boards (
	id ` + idType + `,
	name TEXT NOT NULL UNIQUE,
	desc_text TEXT NOT NULL DEFAULT '',
	slug TEXT NOT NULL DEFAULT '',
	last_update INTEGER NOT NULL DEFAULT 0,
	msgcount INTEGER NOT NULL DEFAULT 0,
	read_mask BIGINT NOT NULL,
	write_mask BIGINT NOT NULL,
	basenote_mask BIGINT NOT NULL,
	sign TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS notes (
	id ` + idType + `,
	board_id INTEGER NOT NULL,
	num INTEGER NOT NULL,
	title TEXT NOT NULL,
	author TEXT NOT NULL,
	handle TEXT NOT NULL,
	post_time INTEGER NOT NULL,
	last_update INTEGER NOT NULL,
	flags INTEGER NOT NULL DEFAULT 0,
	response INTEGER NOT NULL DEFAULT 0,
	body TEXT NOT NULL DEFAULT '',
	UNIQUE(board_id, num)
);
CREATE TABLE IF NOT EXISTS responses (
	id ` + idType + `,
	note_id INTEGER NOT NULL,
	num INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	author TEXT NOT NULL,
	handle TEXT NOT NULL,
	post_time INTEGER NOT NULL,
	flags INTEGER NOT NULL DEFAULT 0,
	body TEXT NOT NULL DEFAULT '',
	UNIQUE(note_id, num)
);
`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS talk_rooms (
	num INTEGER PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	status INTEGER NOT NULL DEFAULT 0,
	leader TEXT NOT NULL DEFAULT '',
	line_count INTEGER NOT NULL DEFAULT 0,
	last_update INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS talk_lines (
	id ` + idType + `,
	room INTEGER NOT NULL,
	num INTEGER NOT NULL,
	author TEXT NOT NULL,
	handle TEXT NOT NULL,
	post_time INTEGER NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	UNIQUE(room, num)
);
CREATE TABLE IF NOT EXISTS mails (
	id ` + idType + `,
	from_id TEXT NOT NULL,
	from_handle TEXT NOT NULL,
	to_id TEXT NOT NULL,
	subject TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL DEFAULT '',
	sent_at INTEGER NOT NULL,
	read_at INTEGER,
	inbox_del INTEGER NOT NULL DEFAULT 0,
	killed INTEGER NOT NULL DEFAULT 0,
	saved INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS mail_groups (
	owner TEXT NOT NULL,
	name TEXT NOT NULL,
	members TEXT NOT NULL DEFAULT '',
	UNIQUE(owner, name)
);
CREATE TABLE IF NOT EXISTS news_groups (
	name TEXT PRIMARY KEY,
	last_num INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS news_articles (
	id ` + idType + `,
	grp TEXT NOT NULL,
	num INTEGER NOT NULL,
	from_id TEXT NOT NULL,
	from_handle TEXT NOT NULL,
	subject TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL DEFAULT '',
	posted INTEGER NOT NULL,
	ref_num INTEGER NOT NULL DEFAULT 0,
	UNIQUE(grp, num)
);
CREATE TABLE IF NOT EXISTS news_cursors (
	user_id TEXT NOT NULL,
	grp TEXT NOT NULL,
	last_num INTEGER NOT NULL DEFAULT 0,
	unsub INTEGER NOT NULL DEFAULT 0,
	UNIQUE(user_id, grp)
);
`)
	return err
}

func (s *SQLite) SeedIfEmpty(ctx context.Context, password string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	seeds := []User{
		{ID: "sysop", Handle: "Sysop", Flags: 1 << 15, TLimit: 65535},
		{ID: "alice", Handle: "Alice", Flags: 1 << 31, TLimit: 30},
		{ID: "bob", Handle: "Bob", Flags: 1 << 31, TLimit: 30},
	}
	for _, u := range seeds {
		_, err := s.exec(ctx, `
INSERT INTO users (id, password_hash, handle, flags, tlimit)
VALUES (?, ?, ?, ?, ?)`, strings.ToLower(u.ID), string(hash), u.Handle, u.Flags, u.TLimit)
		if err != nil {
			return fmt.Errorf("seed %s: %w", u.ID, err)
		}
	}
	return nil
}

func (s *SQLite) GetUser(ctx context.Context, id string) (User, error) {
	return s.scanUser(s.queryRow(ctx, `
SELECT id, password_hash, handle, flags, tlimit, pwerr, access, last_login, last_logout,
       expert, prompt, term_width, term_height, esc, last_msgread,
       real_name, birthday, address, phone, comment, scan_list, autosign, profile, mail_save
FROM users WHERE id = ?`, strings.ToLower(id)))
}

func (s *SQLite) Authenticate(ctx context.Context, id, password string) (User, error) {
	u, err := s.GetUser(ctx, id)
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return User{}, ErrBadPassword
	}
	return u, nil
}

func (s *SQLite) IncPWErr(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `UPDATE users SET pwerr = pwerr + 1 WHERE id = ?`, strings.ToLower(id))
	return err
}

func (s *SQLite) ClearPWErr(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `UPDATE users SET pwerr = 0 WHERE id = ?`, strings.ToLower(id))
	return err
}

func (s *SQLite) IncAccess(ctx context.Context, id string) error {
	_, err := s.exec(ctx, `UPDATE users SET access = access + 1 WHERE id = ?`, strings.ToLower(id))
	return err
}

func (s *SQLite) SetLoginTimes(ctx context.Context, id string, login, logout time.Time) error {
	_, err := s.exec(ctx, `
UPDATE users SET last_login = ?, last_logout = ? WHERE id = ?`,
		login.Unix(), logout.Unix(), strings.ToLower(id))
	return err
}

func (s *SQLite) InsertLog(ctx context.Context, log AccessLog) error {
	var disc any
	if log.DisconnectTime != nil {
		disc = log.DisconnectTime.Unix()
	}
	_, err := s.exec(ctx, `
INSERT INTO access_logs (user_id, handle, connect_time, disconnect_time, channel, reason)
VALUES (?, ?, ?, ?, ?, ?)`,
		log.UserID, log.Handle, log.ConnectTime.Unix(), disc, log.Channel, log.Reason)
	return err
}

// StartLog は接続時に切断時刻 NULL の行を入れ、その id を返す（RETURNING で両ドライバ対応）。
func (s *SQLite) StartLog(ctx context.Context, log AccessLog) (int64, error) {
	var id int64
	err := s.queryRow(ctx, `
INSERT INTO access_logs (user_id, handle, connect_time, disconnect_time, channel, reason)
VALUES (?, ?, ?, NULL, ?, ?) RETURNING id`,
		log.UserID, log.Handle, log.ConnectTime.Unix(), log.Channel, log.Reason).Scan(&id)
	return id, err
}

// FinishLog は StartLog の行に切断時刻・理由を書き込む。
func (s *SQLite) FinishLog(ctx context.Context, id int64, disconnect time.Time, reason int) error {
	_, err := s.exec(ctx, `
UPDATE access_logs SET disconnect_time = ?, reason = ? WHERE id = ?`,
		disconnect.Unix(), reason, id)
	return err
}

func (s *SQLite) ListLogs(ctx context.Context, limit int) ([]AccessLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.query(ctx, `
SELECT id, user_id, handle, connect_time, disconnect_time, channel, reason
FROM access_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessLog
	for rows.Next() {
		var l AccessLog
		var c int64
		var d sql.NullInt64
		if err := rows.Scan(&l.ID, &l.UserID, &l.Handle, &c, &d, &l.Channel, &l.Reason); err != nil {
			return nil, err
		}
		l.ConnectTime = time.Unix(c, 0)
		if d.Valid {
			t := time.Unix(d.Int64, 0)
			l.DisconnectTime = &t
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *SQLite) scanUser(row *sql.Row) (User, error) {
	u, err := scanUserRow(row)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *SQLite) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.query(ctx, `
SELECT id, password_hash, handle, flags, tlimit, pwerr, access, last_login, last_logout,
       expert, prompt, term_width, term_height, esc, last_msgread,
       real_name, birthday, address, phone, comment, scan_list, autosign, profile, mail_save
FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *SQLite) UpdateHandle(ctx context.Context, id, handle string) error {
	_, err := s.exec(ctx, `UPDATE users SET handle = ? WHERE id = ?`, handle, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateExpert(ctx context.Context, id string, expert int) error {
	_, err := s.exec(ctx, `UPDATE users SET expert = ? WHERE id = ?`, expert, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateTerminal(ctx context.Context, id string, width, height, esc int, prompt string) error {
	_, err := s.exec(ctx, `
UPDATE users SET term_width = ?, term_height = ?, esc = ?, prompt = ? WHERE id = ?`,
		width, height, esc, prompt, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdatePassword(ctx context.Context, id, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.exec(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateSequencer(ctx context.Context, id string, t time.Time) error {
	_, err := s.exec(ctx, `UPDATE users SET last_msgread = ? WHERE id = ?`, t.Unix(), strings.ToLower(id))
	return err
}

func (s *SQLite) UpdatePrivate(ctx context.Context, id, realName, birthday, address, phone, comment string) error {
	_, err := s.exec(ctx, `
UPDATE users SET real_name = ?, birthday = ?, address = ?, phone = ?, comment = ? WHERE id = ?`,
		realName, birthday, address, phone, comment, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateScanList(ctx context.Context, id, list string) error {
	_, err := s.exec(ctx, `UPDATE users SET scan_list = ? WHERE id = ?`, list, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateAutosign(ctx context.Context, id, text string) error {
	_, err := s.exec(ctx, `UPDATE users SET autosign = ? WHERE id = ?`, text, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateProfile(ctx context.Context, id, text string) error {
	_, err := s.exec(ctx, `UPDATE users SET profile = ? WHERE id = ?`, text, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateMailSave(ctx context.Context, id string, save bool) error {
	v := 0
	if save {
		v = 1
	}
	_, err := s.exec(ctx, `UPDATE users SET mail_save = ? WHERE id = ?`, v, strings.ToLower(id))
	return err
}

func (s *SQLite) CreateUser(ctx context.Context, u User, password string) error {
	id := strings.ToLower(strings.TrimSpace(u.ID))
	if id == "" {
		return ErrNotFound
	}
	if _, err := s.GetUser(ctx, id); err == nil {
		return ErrAlreadyExists
	} else if err != ErrNotFound {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.exec(ctx, `
INSERT INTO users (id, password_hash, handle, flags, tlimit)
VALUES (?, ?, ?, ?, ?)`, id, string(hash), u.Handle, u.Flags, u.TLimit)
	return err
}

// AllocMemberID は prefix＋ゼロ詰め連番の会員IDを 1 つ払い出す。
// 例: prefix="prd", width=5 → "prd00001"。既存の同 prefix の最大番号+1。
// prefix で始まる ID は本メソッドが払い出した数値IDのみである前提（サインアップは自動採番）。
func (s *SQLite) AllocMemberID(ctx context.Context, prefix string, width int) (string, error) {
	prefix = strings.ToLower(prefix)
	row := s.queryRow(ctx,
		`SELECT COALESCE(MAX(CAST(SUBSTR(id, ?) AS INTEGER)), 0) FROM users WHERE id LIKE ?`,
		len(prefix)+1, prefix+"%")
	var max int
	if err := row.Scan(&max); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%0*d", prefix, width, max+1), nil
}

func (s *SQLite) UpdateFlags(ctx context.Context, id string, flags uint32) error {
	_, err := s.exec(ctx, `UPDATE users SET flags = ? WHERE id = ?`, flags, strings.ToLower(id))
	return err
}

func (s *SQLite) UpdateTLimit(ctx context.Context, id string, tlimit int) error {
	_, err := s.exec(ctx, `UPDATE users SET tlimit = ? WHERE id = ?`, tlimit, strings.ToLower(id))
	return err
}

// SeedGuest はオンライン登録用の共有ゲスト口を無ければ作る。パスワードは公開前提。
func (s *SQLite) SeedGuest(ctx context.Context, password string) error {
	if _, err := s.GetUser(ctx, "guest"); err == nil {
		return nil
	} else if err != ErrNotFound {
		return err
	}
	return s.CreateUser(ctx, User{
		ID: "guest", Handle: "Guest", Flags: 1 << 13, TLimit: 10,
	}, password)
}

// SeedAgents はエージェント口座（AgentIO 用の実ユーザー）を無ければ作る。
// 権限は gen|agt（1<<31 | 1<<11）、Expert=2（メニュー出力を抑制）、時間無制限。
// 冪等。AGENTS.txt の id と対応させる。
func (s *SQLite) SeedAgents(ctx context.Context, password string) error {
	const flags uint32 = (1 << 31) | (1 << 11) // gen | agt
	agents := []struct{ ID, Handle string }{
		{"scout", "Scout"},
		{"critic", "Critic"},
		{"poet", "Poet"},
		{"muse", "Muse"},        // poet の会話相手（AI 同士のチャット用）
		{"column", "Columnist"}, // 自発ノートの寄稿者（ID は 8 文字以内）
		{"kai", "Kai"},          // talk 巡回（会議室の書き込み役）
		{"sora", "Sora"},        // talk 巡回（もう 1 体）
	}
	for _, a := range agents {
		if _, err := s.GetUser(ctx, a.ID); err == nil {
			continue
		} else if err != ErrNotFound {
			return err
		}
		if err := s.CreateUser(ctx, User{ID: a.ID, Handle: a.Handle, Flags: flags, TLimit: 65535}, password); err != nil {
			return err
		}
		if err := s.UpdateExpert(ctx, a.ID, 2); err != nil {
			return err
		}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUserRow(row rowScanner) (User, error) {
	var u User
	var login, logout, seq sql.NullInt64
	var save int
	err := row.Scan(&u.ID, &u.PasswordHash, &u.Handle, &u.Flags, &u.TLimit, &u.PWErr, &u.Access, &login, &logout,
		&u.Expert, &u.Prompt, &u.TermWidth, &u.TermHeight, &u.Esc, &seq,
		&u.RealName, &u.Birthday, &u.Address, &u.Phone, &u.Comment, &u.ScanList, &u.Autosign, &u.Profile, &save)
	if err != nil {
		return User{}, err
	}
	u.MailSave = save != 0
	if login.Valid {
		t := time.Unix(login.Int64, 0)
		u.LastLogin = &t
	}
	if logout.Valid {
		t := time.Unix(logout.Int64, 0)
		u.LastLogout = &t
	}
	if seq.Valid {
		t := time.Unix(seq.Int64, 0)
		u.LastMsgRead = &t
	}
	if u.TermWidth <= 0 {
		u.TermWidth = 80
	}
	if u.TermHeight <= 0 {
		u.TermHeight = 24
	}
	return u, nil
}
