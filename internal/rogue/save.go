package rogue

import (
	"bytes"
	"encoding/gob"
	"math/rand"
	"time"
)

// Persist はセーブ・スコアの保管先。command 層が store をつないで実装する。
type Persist interface {
	Load() ([]byte, bool, error) // セーブ本体と有無
	Save(blob []byte) error
	Delete() error
	AddScore(sc Score) error
	Top(limit int) ([]Score, error)
}

// Score は 1 局の成績（command 層で store.RogueScore へ写す）。
type Score struct {
	Gold     int
	Depth    int
	MaxDepth int
	Cause    string
	Won      bool
	Time     time.Time
}

// encode はゲーム状態を gob で固める。
func (g *Game) encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeGame はセーブから Game を復元し、一時状態（io/rng）を付け直す。
func decodeGame(blob []byte, io IO) (*Game, error) {
	g := &Game{}
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(g); err != nil {
		return nil, err
	}
	g.io = io
	g.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	g.markVisible()
	return g, nil
}
