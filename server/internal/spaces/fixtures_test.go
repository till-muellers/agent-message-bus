package spaces

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

const fixtureTS = "2026-01-01T00:00:00Z"

type env struct {
	db    *sql.DB
	auth  *auth.Service
	inbox *inbox.Service
	svc   *Service
}

func newEnv(t *testing.T, hooks ...DepartureHook) env {
	t.Helper()
	db := dbtest.Open(t)
	a := auth.NewService(db, "admin-secret", time.Now)
	ib := inbox.NewService(db, broker.NewMemory(64), time.Now)
	return env{db: db, auth: a, inbox: ib, svc: NewService(db, NewResolver(db), a, ib, time.Now, hooks...)}
}

// seedLeader inserts a leader agent and returns its principal.
func seedLeader(t *testing.T, db *sql.DB, name string) auth.Principal {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO agents (name, kind, created_at) VALUES (?, 'leader', ?)`, name, fixtureTS)
	if err != nil {
		t.Fatalf("seeding leader %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return auth.Principal{Kind: auth.Leader, AgentID: id, AgentName: name}
}

// inviteMember invites name into space and authenticates its token.
func inviteMember(t *testing.T, e env, lead auth.Principal, space, name string) (auth.Principal, string) {
	t.Helper()
	tok, err := e.svc.Invite(context.Background(), lead, space, name)
	if err != nil {
		t.Fatalf("inviting %s: %v", name, err)
	}
	p, err := e.auth.Authenticate(context.Background(), tok)
	if err != nil {
		t.Fatalf("authenticating %s: %v", name, err)
	}
	return p, tok
}

func mustCreate(t *testing.T, e env, p auth.Principal, space string) {
	t.Helper()
	if err := e.svc.Create(context.Background(), p, space); err != nil {
		t.Fatalf("creating %s: %v", space, err)
	}
}

func assertErr(t *testing.T, err error, kind apperr.Kind, msg string) {
	t.Helper()
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Kind != kind || (msg != "" && ae.Msg != msg) {
		t.Fatalf("err = %v, want %s %q", err, kind, msg)
	}
}

func countRows(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}
