package invites

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

var start = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type env struct {
	db     *sql.DB
	auth   *auth.Service
	spaces *spaces.Service
	ib     *inbox.Service
	pub    *broker.Memory
	svc    *Service
	clock  *time.Time
	lead   auth.Principal
}

// newEnv returns invites over real auth, spaces and inbox services, with leader lead of
// space demo and Revocation wired as the spaces revocation hook.
func newEnv(t *testing.T) env {
	t.Helper()
	conn := dbtest.Open(t)
	clock := start
	now := func() time.Time { return clock }
	a := auth.NewService(conn, "admin-secret", now)
	pub := broker.NewMemory(64)
	ib := inbox.NewService(conn, pub, now)
	hooks := spaces.Hooks{Revocation: []spaces.RevocationHook{Revocation{}}}
	sp := spaces.NewService(conn, spaces.NewResolver(conn), a, ib, now, hooks)
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	lead := auth.Principal{Kind: auth.Leader, AgentID: 1, AgentName: "lead"}
	if err := sp.Create(context.Background(), lead, "demo"); err != nil {
		t.Fatalf("creating demo: %v", err)
	}
	return env{db: conn, auth: a, spaces: sp, ib: ib, pub: pub, svc: NewService(conn, sp, ib, now), clock: &clock, lead: lead}
}

// mustCreate creates an invite code for name in demo with the optional first prompt task.
func mustCreate(t *testing.T, e env, name string, task *string) Invite {
	t.Helper()
	inv, err := e.svc.Create(context.Background(), e.lead, "demo", name, task)
	if err != nil {
		t.Fatalf("creating invite for %s: %v", name, err)
	}
	return inv
}

func countInvites(t *testing.T, e env) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM invites`).Scan(&n); err != nil {
		t.Fatalf("counting invites: %v", err)
	}
	return n
}
