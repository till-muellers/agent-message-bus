// Package db opens the SQLite database and applies schema migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open opens the SQLite database at path with foreign keys, WAL and a busy timeout.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening db %s: %w", path, err)
	}
	conn.SetMaxOpenConns(1)
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("opening db %s: %w", path, err)
	}
	return conn, nil
}
