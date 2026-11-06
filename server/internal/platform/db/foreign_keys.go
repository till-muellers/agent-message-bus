package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

// withoutForeignKeys runs fn on one connection of db with foreign key enforcement off and
// turns it back on before the connection returns to the pool. SQLite's table rebuild
// (create the new table, copy, drop the old one, rename) needs this: inside a transaction
// the pragma is a no-op, and with enforcement on DROP TABLE cascades into the children of
// the dropped table. Callers check the keys themselves (checkForeignKeys) before committing.
func withoutForeignKeys(ctx context.Context, db *sql.DB, fn func(conn *sql.Conn) error) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserving a connection: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disabling foreign keys: %w", err)
	}
	defer func() { err = errors.Join(err, enableForeignKeys(ctx, conn)) }()
	return fn(conn)
}

// enableForeignKeys turns enforcement back on, even after ctx ended. When that fails it
// discards conn, so no connection without enforcement returns to the pool.
func enableForeignKeys(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn }) // ErrBadConn makes database/sql drop conn
		return fmt.Errorf("re-enabling foreign keys: %w", err)
	}
	return nil
}

// checkForeignKeys fails when q holds a row whose foreign key matches no parent row.
func checkForeignKeys(ctx context.Context, q DBTX) error {
	rows, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("checking foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return rows.Err()
	}
	var table, parent string
	var rowid sql.NullInt64
	var fk int
	if err := rows.Scan(&table, &rowid, &parent, &fk); err != nil {
		return fmt.Errorf("checking foreign keys: %w", err)
	}
	return fmt.Errorf("foreign key violation: %s row %d references no %s row", table, rowid.Int64, parent)
}
