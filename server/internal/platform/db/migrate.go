package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var upFile = regexp.MustCompile(`^(\d{4})_.+\.up\.sql$`)

type migration struct {
	version int
	name    string
}

// Migrate applies every pending NNNN_*.up.sql file in fsys in ascending order.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}
	pending, err := listMigrations(fsys)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := applyMigration(ctx, db, fsys, m); err != nil {
			return err
		}
	}
	return nil
}

func listMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migrations: %w", err)
	}
	var out []migration
	for _, e := range entries {
		match := upFile.FindStringSubmatch(e.Name())
		if e.IsDir() || match == nil {
			continue
		}
		v, _ := strconv.Atoi(match[1]) // regex guarantees four digits
		out = append(out, migration{version: v, name: e.Name()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func applyMigration(ctx context.Context, db *sql.DB, fsys fs.FS, m migration) error {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&n)
	if err != nil {
		return fmt.Errorf("checking migration %s: %w", m.name, err)
	}
	if n > 0 {
		return nil
	}
	body, err := fs.ReadFile(fsys, m.name)
	if err != nil {
		return fmt.Errorf("reading migration %s: %w", m.name, err)
	}
	return InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("applying migration %s: %w", m.name, err)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version, Timestamp(time.Now()))
		if err != nil {
			return fmt.Errorf("recording migration %s: %w", m.name, err)
		}
		return nil
	})
}
