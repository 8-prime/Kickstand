// Package store is the SQLite persistence layer: trips, the shared roadbook
// log, checklist state, and the cached route geometry.
package store

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"

	_ "modernc.org/sqlite" // pure Go: no cgo, so this cross-compiles anywhere
)

//go:embed schema.sql
var schemaSQL string

// Store owns the database handle.
type Store struct {
	db *sql.DB
}

// Open connects to the SQLite file at path and applies the schema.
//
// WAL keeps a reader from blocking the writer, which matters as soon as more
// than one person has the share link open. busy_timeout covers the brief
// contention that remains.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	// SQLite takes one writer at a time; more connections just means more
	// contention to time out on.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// newToken returns an unguessable identifier for a share link. 16 bytes is
// well past what anyone will brute-force over the public internet, and short
// enough to paste into a chat.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
