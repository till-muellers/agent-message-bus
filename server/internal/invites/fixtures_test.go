package invites

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

var start = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// anyCredential accepts every principal: these tests do not exercise credential
// re-validation, which the spaces tests cover.
type anyCredential struct{}

func (anyCredential) StillValid(context.Context, db.DBTX, auth.Principal) error { return nil }

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
// space demo and the invites Service wired as the spaces revocation hook.
func newEnv(t *testing.T) env {
	t.Helper()
	conn := dbtest.Open(t)
	clock := start
	now := func() time.Time { return clock }
	a := auth.NewService(conn, "admin-secret", now)
	pub := broker.NewMemory(64)
	ib := inbox.NewService(t.Context(), conn, pub, anyCredential{}, now)
	r := spaces.NewResolver(anyCredential{})
	svc := NewService(conn, spaces.NewAdmitter(r, a, now), ib, now)
	sp := spaces.NewService(conn, r, a, ib, now, spaces.Hooks{Revocation: []spaces.RevocationHook{svc}})
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	lead := auth.Principal{Kind: auth.Leader, AgentID: 1, AgentName: "lead"}
	if err := sp.Create(context.Background(), lead, "demo"); err != nil {
		t.Fatalf("creating demo: %v", err)
	}
	return env{db: conn, auth: a, spaces: sp, ib: ib, pub: pub, svc: svc, clock: &clock, lead: lead}
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
