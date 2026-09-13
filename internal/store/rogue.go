package store

import (
	"context"
	"database/sql"
	"time"
)

// SaveRogue はユーザのゲームセーブを 1 本だけ保持する（upsert）。
func (s *SQLite) SaveRogue(ctx context.Context, userID string, blob []byte) error {
	now := time.Now().Unix()
	// まず更新、無ければ挿入（sqlite/postgres 共通のため ON CONFLICT を使う）。
	q := `INSERT INTO rogue_saves (user_id, blob, updated_at) VALUES (?, ?, ?)
	      ON CONFLICT (user_id) DO UPDATE SET blob = excluded.blob, updated_at = excluded.updated_at`
	_, err := s.exec(ctx, q, userID, blob, now)
	return err
}

// LoadRogue はセーブを返す。無ければ ErrNotFound。
func (s *SQLite) LoadRogue(ctx context.Context, userID string) ([]byte, error) {
	var blob []byte
	err := s.queryRow(ctx, `SELECT blob FROM rogue_saves WHERE user_id = ?`, userID).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return blob, nil
}

// DeleteRogue はセーブを消す（再開したら消す＝原典どおり）。無くてもエラーにしない。
func (s *SQLite) DeleteRogue(ctx context.Context, userID string) error {
	_, err := s.exec(ctx, `DELETE FROM rogue_saves WHERE user_id = ?`, userID)
	return err
}

// HasRogueSave はセーブがあるか。
func (s *SQLite) HasRogueSave(ctx context.Context, userID string) (bool, error) {
	var n int
	err := s.queryRow(ctx, `SELECT COUNT(*) FROM rogue_saves WHERE user_id = ?`, userID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AddRogueScore は成績を 1 件記録する。
func (s *SQLite) AddRogueScore(ctx context.Context, sc RogueScore) error {
	won := 0
	if sc.Won {
		won = 1
	}
	_, err := s.exec(ctx,
		`INSERT INTO rogue_scores (user_id, handle, gold, depth, max_depth, cause, won, scored_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.UserID, sc.Handle, sc.Gold, sc.Depth, sc.MaxDepth, sc.Cause, won, sc.Time.Unix())
	return err
}

// TopRogueScores は金塊の多い順に上位を返す。
func (s *SQLite) TopRogueScores(ctx context.Context, limit int) ([]RogueScore, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.query(ctx,
		`SELECT id, user_id, handle, gold, depth, max_depth, cause, won, scored_at
		 FROM rogue_scores ORDER BY gold DESC, scored_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RogueScore
	for rows.Next() {
		var sc RogueScore
		var won int
		var at int64
		if err := rows.Scan(&sc.ID, &sc.UserID, &sc.Handle, &sc.Gold, &sc.Depth,
			&sc.MaxDepth, &sc.Cause, &won, &at); err != nil {
			return nil, err
		}
		sc.Won = won != 0
		sc.Time = time.Unix(at, 0)
		out = append(out, sc)
	}
	return out, rows.Err()
}
