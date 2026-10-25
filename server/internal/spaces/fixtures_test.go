package spaces

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

type env struct {
	db    *sql.DB
	auth  *auth.Service
	inbox *inbox.Service
	svc   *Service
}

func newEnv(t *testing.T, departure []DepartureHook, creation []CreationHook) env {
	t.Helper()
	conn := dbtest.Open(t)
	a := auth.NewService(conn, "admin-secret", time.Now)
	ib := inbox.NewService(conn, broker.NewMemory(64), time.Now)
	return env{db: conn, auth: a, inbox: ib, svc: NewService(conn, NewResolver(conn), a, ib, time.Now, departure, creation)}
}

// leader seeds leader agent id and returns its principal.
func leader(t *testing.T, e env, id int64, name string) auth.Principal {
	t.Helper()
	dbtest.SeedAgent(t, e.db, id, name, "leader")
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

func countRows(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}
