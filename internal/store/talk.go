package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLite) SeedTalkRooms(ctx context.Context) error {
	var n int
	if err := s.queryRow(ctx, `SELECT COUNT(*) FROM talk_rooms`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for i := 1; i <= MaxTalkRooms; i++ {
		if _, err := s.exec(ctx, `
INSERT INTO talk_rooms (num, title, status, leader, line_count, last_update)
VALUES (?, ?, ?, ?, ?, ?)`, i, fmt.Sprintf("Room %d", i), TalkOpen, "", 0, 0); err != nil {
			return fmt.Errorf("seed talk %d: %w", i, err)
		}
	}
	return nil
}

func (s *SQLite) ListTalkRooms(ctx context.Context) ([]TalkRoom, error) {
	rows, err := s.query(ctx, `
SELECT num, title, status, leader, line_count, last_update
FROM talk_rooms ORDER BY num`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TalkRoom
	for rows.Next() {
		r, err := scanTalkRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) GetTalkRoom(ctx context.Context, num int) (TalkRoom, error) {
	r, err := scanTalkRoom(s.queryRow(ctx, `
SELECT num, title, status, leader, line_count, last_update
FROM talk_rooms WHERE num = ?`, num))
	if err == sql.ErrNoRows {
		return TalkRoom{}, ErrNotFound
	}
	return r, err
}

func (s *SQLite) UpdateTalkRoom(ctx context.Context, r TalkRoom) error {
	res, err := s.exec(ctx, `
UPDATE talk_rooms SET title = ?, status = ?, leader = ? WHERE num = ?`,
		r.Title, r.Status, r.Leader, r.Num)
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

func (s *SQLite) ListTalkLines(ctx context.Context, room, fromNum int) ([]TalkLine, error) {
	if fromNum < 1 {
		fromNum = 1
	}
	rows, err := s.query(ctx, `
SELECT id, room, num, author, handle, post_time, body
FROM talk_lines WHERE room = ? AND num >= ? ORDER BY num`, room, fromNum)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTalkLines(rows)
}

func (s *SQLite) ListTalkLinesSince(ctx context.Context, room int, t time.Time) ([]TalkLine, error) {
	rows, err := s.query(ctx, `
SELECT id, room, num, author, handle, post_time, body
FROM talk_lines WHERE room = ? AND post_time > ? ORDER BY num`, room, t.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTalkLines(rows)
}

func (s *SQLite) CreateTalkLine(ctx context.Context, ln TalkLine) (TalkLine, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TalkLine{}, err
	}
	defer tx.Rollback()
	var max int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(num),0) FROM talk_lines WHERE room = ?`), ln.Room).Scan(&max); err != nil {
		return TalkLine{}, err
	}
	if max >= MaxTalkLines {
		return TalkLine{}, ErrTooManyTalkLines
	}
	ln.Num = max + 1
	if ln.PostTime.IsZero() {
		ln.PostTime = time.Now()
	}
	id, err := insertIDTx(tx, s.dollar, s.q, ctx, `
INSERT INTO talk_lines (room, num, author, handle, post_time, body)
VALUES (?, ?, ?, ?, ?, ?)`,
		ln.Room, ln.Num, ln.Author, ln.Handle, ln.PostTime.Unix(), ln.Body)
	if err != nil {
		return TalkLine{}, err
	}
	ln.ID = id
	if _, err := tx.ExecContext(ctx, s.q(`
UPDATE talk_rooms SET line_count = ?, last_update = ? WHERE num = ?`),
		ln.Num, ln.PostTime.Unix(), ln.Room); err != nil {
		return TalkLine{}, err
	}
	if err := tx.Commit(); err != nil {
		return TalkLine{}, err
	}
	return ln, nil
}

func scanTalkLines(rows *sql.Rows) ([]TalkLine, error) {
	var out []TalkLine
	for rows.Next() {
		ln, err := scanTalkLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ln)
	}
	return out, rows.Err()
}

func scanTalkRoom(row rowScanner) (TalkRoom, error) {
	var r TalkRoom
	var last int64
	err := row.Scan(&r.Num, &r.Title, &r.Status, &r.Leader, &r.LineCount, &last)
	if err != nil {
		return TalkRoom{}, err
	}
	if last > 0 {
		r.LastUpdate = time.Unix(last, 0)
	}
	return r, nil
}

func scanTalkLine(row rowScanner) (TalkLine, error) {
	var ln TalkLine
	var post int64
	err := row.Scan(&ln.ID, &ln.Room, &ln.Num, &ln.Author, &ln.Handle, &post, &ln.Body)
	if err != nil {
		return TalkLine{}, err
	}
	ln.PostTime = time.Unix(post, 0)
	return ln, nil
}
