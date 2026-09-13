package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func ParseMailMembers(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(s) {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) >= MaxGroupMembers {
			break
		}
	}
	return out
}

func JoinMailMembers(ids []string) string {
	return strings.Join(ParseMailMembers(strings.Join(ids, " ")), " ")
}

func (s *SQLite) SendMail(ctx context.Context, m Mail) (Mail, error) {
	m.FromID = strings.ToLower(m.FromID)
	m.ToID = strings.ToLower(m.ToID)
	if m.SentAt.IsZero() {
		m.SentAt = time.Now()
	}
	var n int
	if err := s.queryRow(ctx, `
SELECT COUNT(*) FROM mails WHERE to_id = ? AND inbox_del = 0 AND killed = 0`, m.ToID).Scan(&n); err != nil {
		return Mail{}, err
	}
	if n >= MaxInbox {
		return Mail{}, ErrTooManyMails
	}
	saved := 0
	if m.Saved {
		saved = 1
	}
	id, err := s.insertID(ctx, `
INSERT INTO mails (from_id, from_handle, to_id, subject, body, sent_at, read_at, inbox_del, killed, saved)
VALUES (?, ?, ?, ?, ?, ?, NULL, 0, 0, ?)`,
		m.FromID, m.FromHandle, m.ToID, m.Subject, m.Body, m.SentAt.Unix(), saved)
	if err != nil {
		return Mail{}, err
	}
	m.ID = id
	m.InboxDel = false
	m.Killed = false
	return m, nil
}

func (s *SQLite) ListInbox(ctx context.Context, userID string) ([]Mail, error) {
	return s.listMail(ctx, `
SELECT id, from_id, from_handle, to_id, subject, body, sent_at, read_at, inbox_del, killed, saved
FROM mails WHERE to_id = ? AND inbox_del = 0 AND killed = 0
ORDER BY sent_at, id`, strings.ToLower(userID))
}

func (s *SQLite) ListSent(ctx context.Context, userID string) ([]Mail, error) {
	return s.listMail(ctx, `
SELECT id, from_id, from_handle, to_id, subject, body, sent_at, read_at, inbox_del, killed, saved
FROM mails WHERE from_id = ? AND saved = 1 AND killed = 0
ORDER BY sent_at, id`, strings.ToLower(userID))
}

func (s *SQLite) ListWithdrawable(ctx context.Context, userID string) ([]Mail, error) {
	return s.listMail(ctx, `
SELECT id, from_id, from_handle, to_id, subject, body, sent_at, read_at, inbox_del, killed, saved
FROM mails WHERE from_id = ? AND killed = 0 AND read_at IS NULL
ORDER BY sent_at, id`, strings.ToLower(userID))
}

func (s *SQLite) GetMail(ctx context.Context, id int64) (Mail, error) {
	m, err := scanMail(s.queryRow(ctx, `
SELECT id, from_id, from_handle, to_id, subject, body, sent_at, read_at, inbox_del, killed, saved
FROM mails WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Mail{}, ErrNotFound
	}
	return m, err
}

func (s *SQLite) MarkMailRead(ctx context.Context, id int64) error {
	now := time.Now().Unix()
	_, err := s.exec(ctx, `UPDATE mails SET read_at = ? WHERE id = ? AND read_at IS NULL`, now, id)
	return err
}

func (s *SQLite) DeleteInboxMail(ctx context.Context, id int64, userID string) error {
	res, err := s.exec(ctx, `
UPDATE mails SET inbox_del = 1 WHERE id = ? AND to_id = ? AND inbox_del = 0 AND killed = 0`,
		id, strings.ToLower(userID))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) KillMail(ctx context.Context, id int64, fromID string) error {
	res, err := s.exec(ctx, `
UPDATE mails SET killed = 1 WHERE id = ? AND from_id = ? AND killed = 0 AND read_at IS NULL`,
		id, strings.ToLower(fromID))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) CountUnreadMail(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.queryRow(ctx, `
SELECT COUNT(*) FROM mails WHERE to_id = ? AND inbox_del = 0 AND killed = 0 AND read_at IS NULL`,
		strings.ToLower(userID)).Scan(&n)
	return n, err
}

func (s *SQLite) ListMailGroups(ctx context.Context, owner string) ([]MailGroup, error) {
	rows, err := s.query(ctx, `
SELECT owner, name, members FROM mail_groups WHERE owner = ? ORDER BY name`, strings.ToLower(owner))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MailGroup
	for rows.Next() {
		var g MailGroup
		if err := rows.Scan(&g.Owner, &g.Name, &g.Members); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLite) GetMailGroup(ctx context.Context, owner, name string) (MailGroup, error) {
	var g MailGroup
	err := s.queryRow(ctx, `
SELECT owner, name, members FROM mail_groups WHERE owner = ? AND name = ?`,
		strings.ToLower(owner), strings.ToLower(name)).Scan(&g.Owner, &g.Name, &g.Members)
	if err == sql.ErrNoRows {
		return MailGroup{}, ErrNotFound
	}
	return g, err
}

func (s *SQLite) SaveMailGroup(ctx context.Context, g MailGroup) error {
	g.Owner = strings.ToLower(g.Owner)
	g.Name = strings.ToLower(strings.TrimSpace(g.Name))
	g.Members = JoinMailMembers(ParseMailMembers(g.Members))
	if g.Name == "" {
		return ErrNotFound
	}
	_, err := s.GetMailGroup(ctx, g.Owner, g.Name)
	if err == ErrNotFound {
		var n int
		if err := s.queryRow(ctx, `SELECT COUNT(*) FROM mail_groups WHERE owner = ?`, g.Owner).Scan(&n); err != nil {
			return err
		}
		if n >= MaxMailGroups {
			return ErrTooManyGroups
		}
		_, err = s.exec(ctx, `
INSERT INTO mail_groups (owner, name, members) VALUES (?, ?, ?)`, g.Owner, g.Name, g.Members)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.exec(ctx, `
UPDATE mail_groups SET members = ? WHERE owner = ? AND name = ?`, g.Members, g.Owner, g.Name)
	return err
}

func (s *SQLite) DeleteMailGroup(ctx context.Context, owner, name string) error {
	res, err := s.exec(ctx, `DELETE FROM mail_groups WHERE owner = ? AND name = ?`,
		strings.ToLower(owner), strings.ToLower(name))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) listMail(ctx context.Context, query string, args ...any) ([]Mail, error) {
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mail
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanMail(row rowScanner) (Mail, error) {
	var m Mail
	var sent int64
	var read sql.NullInt64
	var inboxDel, killed, saved int
	err := row.Scan(&m.ID, &m.FromID, &m.FromHandle, &m.ToID, &m.Subject, &m.Body, &sent, &read, &inboxDel, &killed, &saved)
	if err != nil {
		return Mail{}, err
	}
	m.SentAt = time.Unix(sent, 0)
	if read.Valid {
		t := time.Unix(read.Int64, 0)
		m.ReadAt = &t
	}
	m.InboxDel = inboxDel != 0
	m.Killed = killed != 0
	m.Saved = saved != 0
	return m, nil
}
