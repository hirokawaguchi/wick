package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *SQLite) insertID(ctx context.Context, query string, args ...any) (int64, error) {
	if s.dollar {
		q := strings.TrimRight(strings.TrimSpace(query), ";") + " RETURNING id"
		var id int64
		if err := s.queryRow(ctx, q, args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := s.exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLite) SeedBoards(ctx context.Context) error {
	test, err := s.ensureBoard(ctx, Board{
		Name: "junk.test", Desc: "scratch board", Slug: "junk",
		Read: MaskAll, Write: MaskMembers, Basenote: MaskMembers,
	})
	if err != nil {
		return err
	}
	if _, err := s.ensureBoard(ctx, Board{
		Name: "junk.sandbox", Desc: "もう一つのジャンク", Slug: "junk",
		Read: MaskAll, Write: MaskMembers, Basenote: MaskMembers,
	}); err != nil {
		return err
	}
	// sys.jobs: エージェント用の仕事ボード（UC11/UC3）。人が依頼を立て、
	// エージェントがレスで引き受けて結論を書き、% でクローズする。
	jobs, err := s.ensureBoard(ctx, Board{
		Name: "sys.jobs", Desc: "エージェントへの依頼", Slug: "jobs",
		Read: MaskMembers, Write: MaskMembers, Basenote: MaskMembers,
	})
	if err != nil {
		return err
	}
	if jn, err := s.ListNotes(ctx, jobs.ID); err == nil && len(jn) == 0 {
		now := time.Now()
		if _, err := s.CreateNote(ctx, Note{
			BoardID: jobs.ID, Title: "調査依頼: HyperNotes の事例",
			Author: "sysop", Handle: "Sysop", PostTime: now, LastUpdate: now,
			Body: "HyperNotes の使い方の良い事例を集めて、要点をまとめてください。\n担当: scout(収集) critic(批評)。結論はこのノートにレスで。\n",
		}); err != nil {
			return err
		}
	}
	notes, err := s.ListNotes(ctx, test.ID)
	if err != nil {
		return err
	}
	if len(notes) > 0 {
		return nil
	}
	now := time.Now()
	_, err = s.CreateNote(ctx, Note{
		BoardID: test.ID, Title: "Welcome to Wick",
		Author: "sysop", Handle: "Sysop",
		PostTime: now, LastUpdate: now,
		Body: "junk.test です。INDEX は直近20件。w がベースノート、OPEN で w がレス、l が未読、new が全ボード未読、q で抜けます。\n",
	})
	return err
}

func (s *SQLite) ensureBoard(ctx context.Context, b Board) (Board, error) {
	got, err := s.GetBoard(ctx, b.Name)
	if err == nil {
		return got, nil
	}
	if err != ErrNotFound {
		return Board{}, err
	}
	return s.CreateBoard(ctx, b)
}

func (s *SQLite) ListBoards(ctx context.Context) ([]Board, error) {
	rows, err := s.query(ctx, `
SELECT id, name, desc_text, slug, last_update, msgcount, read_mask, write_mask, basenote_mask, sign
FROM boards ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		b, err := scanBoard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *SQLite) GetBoard(ctx context.Context, name string) (Board, error) {
	b, err := scanBoard(s.queryRow(ctx, `
SELECT id, name, desc_text, slug, last_update, msgcount, read_mask, write_mask, basenote_mask, sign
FROM boards WHERE lower(name) = ?`, strings.ToLower(name)))
	if err == sql.ErrNoRows {
		return Board{}, ErrNotFound
	}
	return b, err
}

func (s *SQLite) CreateBoard(ctx context.Context, b Board) (Board, error) {
	if b.Read == 0 && b.Write == 0 && b.Basenote == 0 {
		b.Read, b.Write, b.Basenote = MaskAll, MaskMembers, MaskMembers
	}
	if b.Slug == "" {
		b.Slug = b.Name
	}
	id, err := s.insertID(ctx, `
INSERT INTO boards (name, desc_text, slug, last_update, msgcount, read_mask, write_mask, basenote_mask, sign)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Name, b.Desc, b.Slug, b.LastUpdate.Unix(), b.MsgCount, int64(b.Read), int64(b.Write), int64(b.Basenote), b.Sign)
	if err != nil {
		return Board{}, err
	}
	b.ID = id
	return b, nil
}

func (s *SQLite) UpdateBoard(ctx context.Context, b Board) error {
	_, err := s.exec(ctx, `
UPDATE boards SET name = ?, desc_text = ?, slug = ?, last_update = ?, msgcount = ?,
	read_mask = ?, write_mask = ?, basenote_mask = ?, sign = ? WHERE id = ?`,
		b.Name, b.Desc, b.Slug, b.LastUpdate.Unix(), b.MsgCount,
		int64(b.Read), int64(b.Write), int64(b.Basenote), b.Sign, b.ID)
	return err
}

func (s *SQLite) ListNotes(ctx context.Context, boardID int64) ([]Note, error) {
	rows, err := s.query(ctx, `
SELECT id, board_id, num, title, author, handle, post_time, last_update, flags, response, body
FROM notes WHERE board_id = ? ORDER BY num`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *SQLite) GetBoardByID(ctx context.Context, id int64) (Board, error) {
	b, err := scanBoard(s.queryRow(ctx, `
SELECT id, name, desc_text, slug, last_update, msgcount, read_mask, write_mask, basenote_mask, sign
FROM boards WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Board{}, ErrNotFound
	}
	return b, err
}

func (s *SQLite) GetNote(ctx context.Context, boardID int64, num int) (Note, error) {
	n, err := scanNote(s.queryRow(ctx, `
SELECT id, board_id, num, title, author, handle, post_time, last_update, flags, response, body
FROM notes WHERE board_id = ? AND num = ?`, boardID, num))
	if err == sql.ErrNoRows {
		return Note{}, ErrNotFound
	}
	return n, err
}

func (s *SQLite) GetNoteByID(ctx context.Context, id int64) (Note, error) {
	n, err := scanNote(s.queryRow(ctx, `
SELECT id, board_id, num, title, author, handle, post_time, last_update, flags, response, body
FROM notes WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return Note{}, ErrNotFound
	}
	return n, err
}

func (s *SQLite) CreateNote(ctx context.Context, n Note) (Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()
	var max int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(num),0) FROM notes WHERE board_id = ?`), n.BoardID).Scan(&max); err != nil {
		return Note{}, err
	}
	if max >= MaxNotes {
		return Note{}, ErrTooManyNotes
	}
	n.Num = max + 1
	if n.PostTime.IsZero() {
		n.PostTime = time.Now()
	}
	if n.LastUpdate.IsZero() {
		n.LastUpdate = n.PostTime
	}
	id, err := insertIDTx(tx, s.dollar, s.q, ctx, `
INSERT INTO notes (board_id, num, title, author, handle, post_time, last_update, flags, response, body)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		n.BoardID, n.Num, n.Title, n.Author, n.Handle, n.PostTime.Unix(), n.LastUpdate.Unix(), n.Flags, n.Body)
	if err != nil {
		return Note{}, err
	}
	n.ID = id
	if _, err := tx.ExecContext(ctx, s.q(`
UPDATE boards SET last_update = ?, msgcount = msgcount + 1 WHERE id = ?`), n.PostTime.Unix(), n.BoardID); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, err
	}
	return n, nil
}

func (s *SQLite) UpdateNote(ctx context.Context, n Note) error {
	_, err := s.exec(ctx, `
UPDATE notes SET title = ?, flags = ?, last_update = ?, response = ?, body = ? WHERE id = ?`,
		n.Title, n.Flags, n.LastUpdate.Unix(), n.Response, n.Body, n.ID)
	return err
}

// SeedTopics はボードにノートが 1 件も無いときだけ、話題ベースノートを作る（冪等）。
// これらは README 相当の見出し。通常の書き込みはこの話題へのレスとして積む運用。
func (s *SQLite) SeedTopics(ctx context.Context, board, author, handle string, topics []NoteSeed) error {
	b, err := s.GetBoard(ctx, board)
	if err != nil {
		return err
	}
	notes, err := s.ListNotes(ctx, b.ID)
	if err != nil {
		return err
	}
	if len(notes) > 0 {
		return nil // 既に話題（またはノート）がある。触らない
	}
	for _, t := range topics {
		if strings.TrimSpace(t.Title) == "" {
			continue
		}
		if _, err := s.CreateNote(ctx, Note{
			BoardID: b.ID, Title: t.Title, Author: author, Handle: handle,
			Flags: MsgSubject, Body: t.Body,
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteBoardNotes はボードのノートとレスを全消去する（保守用。scratch 板向け）。
// 消したノート数を返す。
func (s *SQLite) DeleteBoardNotes(ctx context.Context, board string) (int, error) {
	b, err := s.GetBoard(ctx, board)
	if err != nil {
		return 0, err
	}
	notes, err := s.ListNotes(ctx, b.ID)
	if err != nil {
		return 0, err
	}
	if _, err := s.exec(ctx, `
DELETE FROM responses WHERE note_id IN (SELECT id FROM notes WHERE board_id = ?)`, b.ID); err != nil {
		return 0, err
	}
	if _, err := s.exec(ctx, `DELETE FROM notes WHERE board_id = ?`, b.ID); err != nil {
		return 0, err
	}
	if _, err := s.exec(ctx, `UPDATE boards SET msgcount = 0 WHERE id = ?`, b.ID); err != nil {
		return 0, err
	}
	return len(notes), nil
}

func (s *SQLite) ListResponses(ctx context.Context, noteID int64) ([]Response, error) {
	rows, err := s.query(ctx, `
SELECT id, note_id, num, title, author, handle, post_time, flags, body
FROM responses WHERE note_id = ? ORDER BY num`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Response
	for rows.Next() {
		r, err := scanResponse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) GetResponse(ctx context.Context, noteID int64, num int) (Response, error) {
	r, err := scanResponse(s.queryRow(ctx, `
SELECT id, note_id, num, title, author, handle, post_time, flags, body
FROM responses WHERE note_id = ? AND num = ?`, noteID, num))
	if err == sql.ErrNoRows {
		return Response{}, ErrNotFound
	}
	return r, err
}

func (s *SQLite) CreateResponse(ctx context.Context, r Response) (Response, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Response{}, err
	}
	defer tx.Rollback()
	var max int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(num),0) FROM responses WHERE note_id = ?`), r.NoteID).Scan(&max); err != nil {
		return Response{}, err
	}
	if max >= MaxResponses {
		return Response{}, ErrTooManyResponses
	}
	r.Num = max + 1
	if r.PostTime.IsZero() {
		r.PostTime = time.Now()
	}
	id, err := insertIDTx(tx, s.dollar, s.q, ctx, `
INSERT INTO responses (note_id, num, title, author, handle, post_time, flags, body)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.NoteID, r.Num, r.Title, r.Author, r.Handle, r.PostTime.Unix(), r.Flags, r.Body)
	if err != nil {
		return Response{}, err
	}
	r.ID = id
	if _, err := tx.ExecContext(ctx, s.q(`
UPDATE notes SET last_update = ?, response = response + 1 WHERE id = ?`), r.PostTime.Unix(), r.NoteID); err != nil {
		return Response{}, err
	}
	if _, err := tx.ExecContext(ctx, s.q(`
UPDATE boards SET last_update = ?, msgcount = msgcount + 1
WHERE id = (SELECT board_id FROM notes WHERE id = ?)`), r.PostTime.Unix(), r.NoteID); err != nil {
		return Response{}, err
	}
	if err := tx.Commit(); err != nil {
		return Response{}, err
	}
	return r, nil
}

func (s *SQLite) UpdateResponse(ctx context.Context, r Response) error {
	_, err := s.exec(ctx, `
UPDATE responses SET title = ?, flags = ?, body = ? WHERE id = ?`,
		r.Title, r.Flags, r.Body, r.ID)
	return err
}

func insertIDTx(tx *sql.Tx, dollar bool, qf func(string) string, ctx context.Context, query string, args ...any) (int64, error) {
	query = qf(query)
	if dollar {
		query = strings.TrimRight(strings.TrimSpace(query), ";") + " RETURNING id"
		var id int64
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanBoard(row rowScanner) (Board, error) {
	var b Board
	var last int64
	var read, write, base int64
	err := row.Scan(&b.ID, &b.Name, &b.Desc, &b.Slug, &last, &b.MsgCount, &read, &write, &base, &b.Sign)
	if err != nil {
		return Board{}, err
	}
	if last > 0 {
		b.LastUpdate = time.Unix(last, 0)
	}
	b.Read, b.Write, b.Basenote = uint32(read), uint32(write), uint32(base)
	return b, nil
}

func scanNote(row rowScanner) (Note, error) {
	var n Note
	var post, last int64
	err := row.Scan(&n.ID, &n.BoardID, &n.Num, &n.Title, &n.Author, &n.Handle, &post, &last, &n.Flags, &n.Response, &n.Body)
	if err != nil {
		return Note{}, err
	}
	n.PostTime = time.Unix(post, 0)
	n.LastUpdate = time.Unix(last, 0)
	return n, nil
}

func scanResponse(row rowScanner) (Response, error) {
	var r Response
	var post int64
	err := row.Scan(&r.ID, &r.NoteID, &r.Num, &r.Title, &r.Author, &r.Handle, &post, &r.Flags, &r.Body)
	if err != nil {
		return Response{}, err
	}
	r.PostTime = time.Unix(post, 0)
	return r, nil
}
