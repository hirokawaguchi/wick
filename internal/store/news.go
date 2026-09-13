package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"
)

func CanonicalNewsGroup(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".")
}

func (s *SQLite) SeedNewsGroups(ctx context.Context) error {
	_, err := s.EnsureNewsGroup(ctx, DefaultNewsGroup)
	return err
}

func (s *SQLite) ListNewsGroups(ctx context.Context) ([]NewsGroup, error) {
	rows, err := s.query(ctx, `SELECT name, last_num FROM news_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NewsGroup
	for rows.Next() {
		var g NewsGroup
		if err := rows.Scan(&g.Name, &g.LastNum); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLite) GetNewsGroup(ctx context.Context, name string) (NewsGroup, error) {
	name = CanonicalNewsGroup(name)
	var g NewsGroup
	err := s.queryRow(ctx, `SELECT name, last_num FROM news_groups WHERE name = ?`, name).Scan(&g.Name, &g.LastNum)
	if err == sql.ErrNoRows {
		return NewsGroup{}, ErrNotFound
	}
	return g, err
}

func (s *SQLite) EnsureNewsGroup(ctx context.Context, name string) (NewsGroup, error) {
	name = CanonicalNewsGroup(name)
	if name == "" {
		return NewsGroup{}, ErrNotFound
	}
	g, err := s.GetNewsGroup(ctx, name)
	if err == nil {
		return g, nil
	}
	if err != ErrNotFound {
		return NewsGroup{}, err
	}
	var n int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM news_groups`).Scan(&n); err != nil {
		return NewsGroup{}, err
	}
	if n >= MaxNewsGroups {
		return NewsGroup{}, ErrTooManyNews
	}
	if _, err := s.exec(ctx, `INSERT INTO news_groups (name, last_num) VALUES (?, 0)`, name); err != nil {
		return NewsGroup{}, err
	}
	return NewsGroup{Name: name}, nil
}

func (s *SQLite) PostNews(ctx context.Context, a NewsArticle) (NewsArticle, error) {
	a.Group = CanonicalNewsGroup(a.Group)
	if a.Group == "" {
		a.Group = DefaultNewsGroup
	}
	if a.Posted.IsZero() {
		a.Posted = time.Now()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewsArticle{}, err
	}
	defer tx.Rollback()
	var last int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT last_num FROM news_groups WHERE name = ?`), a.Group).Scan(&last); err != nil {
		if err == sql.ErrNoRows {
			return NewsArticle{}, ErrNotFound
		}
		return NewsArticle{}, err
	}
	if last >= MaxNewsArticles {
		return NewsArticle{}, ErrTooManyNews
	}
	a.Num = last + 1
	id, err := insertIDTx(tx, s.dollar, s.q, ctx, `
INSERT INTO news_articles (grp, num, from_id, from_handle, subject, body, posted, ref_num)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Group, a.Num, strings.ToLower(a.FromID), a.FromHandle, a.Subject, a.Body, a.Posted.Unix(), a.RefNum)
	if err != nil {
		return NewsArticle{}, err
	}
	a.ID = id
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE news_groups SET last_num = ? WHERE name = ?`), a.Num, a.Group); err != nil {
		return NewsArticle{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewsArticle{}, err
	}
	return a, nil
}

func (s *SQLite) ListNewsAfter(ctx context.Context, group string, after int) ([]NewsArticle, error) {
	rows, err := s.query(ctx, `
SELECT id, grp, num, from_id, from_handle, subject, body, posted, ref_num
FROM news_articles WHERE grp = ? AND num > ? ORDER BY num`, CanonicalNewsGroup(group), after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NewsArticle
	for rows.Next() {
		a, err := scanNewsArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) GetNewsArticle(ctx context.Context, group string, num int) (NewsArticle, error) {
	a, err := scanNewsArticle(s.queryRow(ctx, `
SELECT id, grp, num, from_id, from_handle, subject, body, posted, ref_num
FROM news_articles WHERE grp = ? AND num = ?`, CanonicalNewsGroup(group), num))
	if err == sql.ErrNoRows {
		return NewsArticle{}, ErrNotFound
	}
	return a, err
}

func (s *SQLite) NewsCursor(ctx context.Context, userID, group string) (int, error) {
	var n int
	err := s.queryRow(ctx, `
SELECT last_num FROM news_cursors WHERE user_id = ? AND grp = ?`,
		strings.ToLower(userID), CanonicalNewsGroup(group)).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

func (s *SQLite) SetNewsCursor(ctx context.Context, userID, group string, last int) error {
	userID = strings.ToLower(userID)
	group = CanonicalNewsGroup(group)
	res, err := s.exec(ctx, `
UPDATE news_cursors SET last_num = ? WHERE user_id = ? AND grp = ?`, last, userID, group)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = s.exec(ctx, `
INSERT INTO news_cursors (user_id, grp, last_num) VALUES (?, ?, ?)`, userID, group, last)
	return err
}

func (s *SQLite) CountUnreadNews(ctx context.Context, userID string) (int, error) {
	groups, err := s.ListNewsGroups(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, g := range groups {
		sub, err := s.NewsSubscribed(ctx, userID, g.Name)
		if err != nil {
			return 0, err
		}
		if !sub {
			continue
		}
		cur, err := s.NewsCursor(ctx, userID, g.Name)
		if err != nil {
			return 0, err
		}
		var n int
		if err := s.queryRow(ctx, `
SELECT COUNT(*) FROM news_articles WHERE grp = ? AND num > ?`, g.Name, cur).Scan(&n); err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// UnsubscribeNews は購読解除の印を立てる。既読位置（last_num）は残す。
// 再購読の口は UNIX 同様に無い（手で cursor 行を消すまで解除のまま）。
func (s *SQLite) UnsubscribeNews(ctx context.Context, userID, group string) error {
	userID = strings.ToLower(userID)
	group = CanonicalNewsGroup(group)
	res, err := s.exec(ctx, `
UPDATE news_cursors SET unsub = 1 WHERE user_id = ? AND grp = ?`, userID, group)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = s.exec(ctx, `
INSERT INTO news_cursors (user_id, grp, last_num, unsub) VALUES (?, ?, 0, 1)`, userID, group)
	return err
}

// NewsSubscribed は購読中かどうか。cursor 行が無ければ購読扱い（既定は全購読）。
func (s *SQLite) NewsSubscribed(ctx context.Context, userID, group string) (bool, error) {
	var unsub int
	err := s.queryRow(ctx, `
SELECT unsub FROM news_cursors WHERE user_id = ? AND grp = ?`,
		strings.ToLower(userID), CanonicalNewsGroup(group)).Scan(&unsub)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return unsub == 0, nil
}

func scanNewsArticle(row rowScanner) (NewsArticle, error) {
	var a NewsArticle
	var posted int64
	err := row.Scan(&a.ID, &a.Group, &a.Num, &a.FromID, &a.FromHandle, &a.Subject, &a.Body, &posted, &a.RefNum)
	if err != nil {
		return NewsArticle{}, err
	}
	a.Posted = time.Unix(posted, 0)
	return a, nil
}
