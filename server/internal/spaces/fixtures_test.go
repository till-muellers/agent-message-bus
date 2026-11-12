package spaces

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

type env struct {
	db    *sql.DB
	auth  *auth.Service
	inbox *inbox.Service
	svc   *Service
	adm   *Admitter
}

func newEnv(t *testing.T, departure []DepartureHook, creation []CreationHook) env {
	t.Helper()
	return newEnvHooks(t, Hooks{Departure: departure, Creation: creation})
}

// adminSecret is the instance admin token of every env.
const adminSecret = "admin-secret"

func newEnvHooks(t *testing.T, hooks Hooks) env {
	t.Helper()
	conn := dbtest.Open(t)
	a := auth.NewService(conn, adminSecret, time.Now)
	ib := inbox.NewService(t.Context(), conn, broker.NewMemory(64), a, time.Now)
	r := NewResolver(a)
	return env{db: conn, auth: a, inbox: ib, svc: NewService(conn, r, a, ib, time.Now, hooks),
		adm: NewAdmitter(r, a, ib, time.Now, nil)}
}

// leader seeds leader agent id with a token and returns its principal.
func leader(t *testing.T, e env, id int64, name string) auth.Principal {
	t.Helper()
	return authenticated(t, e, id, name, "leader", 0)
}

// authenticated seeds agent id with name and kind plus a token bound to spaceID (0: none)
// and returns the principal that token authenticates.
func authenticated(t *testing.T, e env, id int64, name, kind string, spaceID int64) auth.Principal {
	t.Helper()
	dbtest.SeedAgent(t, e.db, id, name, kind)
	tok := "tok-" + name
	dbtest.Exec(t, e.db, `INSERT INTO tokens (hash, agent_id, space_id, created_at) VALUES (?, ?, ?, ?)`,
		auth.Hash(tok), id, sql.NullInt64{Int64: spaceID, Valid: spaceID != 0}, dbtest.SeedTime)
	p, err := e.auth.Authenticate(context.Background(), tok)
	if err != nil {
		t.Fatalf("authenticating %s: %v", name, err)
	}
	return p
}

// instanceAdmin returns the principal of the instance admin token.
func instanceAdmin(t *testing.T, e env) auth.Principal {
	t.Helper()
	p, err := e.auth.Authenticate(context.Background(), adminSecret)
	if err != nil {
		t.Fatalf("authenticating the instance admin token: %v", err)
	}
	return p
}

// inviteMember authorizes and admits name into space and authenticates its token.
func inviteMember(t *testing.T, e env, lead auth.Principal, space, name string) (auth.Principal, string) {
	t.Helper()
	tok, err := invite(context.Background(), e, lead, space, name)
	if err != nil {
		t.Fatalf("inviting %s: %v", name, err)
	}
	p, err := e.auth.Authenticate(context.Background(), tok)
	if err != nil {
		t.Fatalf("authenticating %s: %v", name, err)
	}
	return p, tok
}

// invite runs AuthorizeInvite and AdmitMember in one transaction and returns the token.
func invite(ctx context.Context, e env, lead auth.Principal, space, name string) (string, error) {
	var tok string
	err := db.InTx(ctx, e.db, func(tx *sql.Tx) error {
		id, err := e.adm.AuthorizeInvite(ctx, tx, lead, space, name)
		if err != nil {
			return err
		}
		_, _, tok, err = e.adm.AdmitMember(ctx, tx, id, name)
		return err
	})
	return tok, err
}

func mustCreate(t *testing.T, e env, p auth.Principal, space string) {
	t.Helper()
	if err := e.svc.Create(context.Background(), p, space); err != nil {
		t.Fatalf("creating %s: %v", space, err)
	}
}

// wantSpace asserts through the service whether space exists.
func wantSpace(t *testing.T, e env, space string, exists bool) {
	t.Helper()
	_, err := e.svc.Participants(context.Background(), instanceAdmin(t, e), space)
	if !exists {
		apperrtest.WantKind(t, err, apperr.NotFound, "space "+space+" not found")
		return
	}
	if err != nil {
		t.Fatalf("space %s: %v", space, err)
	}
}
