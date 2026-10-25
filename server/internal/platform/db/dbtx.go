package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DBTX is satisfied by both *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// InTx runs fn in one transaction, committing on success and rolling back otherwise.
// fn MUST use only tx: the pool has one connection, so touching conn inside fn deadlocks.
func InTx(ctx context.Context, conn *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// Timestamp formats t as the RFC3339Nano UTC text stored in every *_at column.
func Timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
