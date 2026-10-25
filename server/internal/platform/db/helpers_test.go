package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func countAgents(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInTx(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name      string
		fnErr     error
		wantCount int
	}{
		{name: "commits on success", wantCount: 1},
		{name: "rolls back on error", fnErr: boom, wantCount: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conn := dbtest.Open(t)
			err := db.InTx(context.Background(), conn, func(tx *sql.Tx) error {
				dbtest.SeedAgent(t, tx, 1, "lead", "leader")
				return tc.fnErr
			})
			if !errors.Is(err, tc.fnErr) || (tc.fnErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tc.fnErr)
			}
			if got := countAgents(t, conn); got != tc.wantCount {
				t.Fatalf("agents = %d, want %d", got, tc.wantCount)
			}
		})
	}
}

func TestInsertIDAndQueryIDs(t *testing.T) {
	t.Parallel()
	conn := dbtest.Open(t)
	ctx := context.Background()
	var ids []int64
	for _, name := range []string{"b", "a"} {
		id, err := db.InsertID(ctx, conn, `INSERT INTO agents (name, kind, created_at) VALUES (?, 'leader', 'x')`, name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	got, err := db.QueryIDs(ctx, conn, `SELECT id FROM agents ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != ids[1] || got[1] != ids[0] {
		t.Fatalf("QueryIDs = %v, inserted %v", got, ids)
	}
	if _, err := db.InsertID(ctx, conn, `INSERT INTO nope VALUES (1)`); err == nil {
		t.Fatal("want error for bad insert")
	}
}

func TestTimestamp(t *testing.T) {
	t.Parallel()
	in := time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("x", 3600))
	if got, want := db.Timestamp(in), "2026-01-02T02:04:05.000000006Z"; got != want {
		t.Fatalf("Timestamp = %q, want %q", got, want)
	}
}
