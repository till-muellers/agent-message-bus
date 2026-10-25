package blackboards

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

// fixture: space alpha (10) with leader lead(1), members m1(2), m2(3);
// space beta (20) with leader lead and member b1(4).
type fixture struct {
	db   *sql.DB
	svc  *Service
	ib   *inbox.Service
	lead auth.Principal
	m1   auth.Principal
	m2   auth.Principal
	b1   auth.Principal
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := dbtest.Open(t)
	for _, a := range []struct {
		id         int64
		name, kind string
	}{{1, "lead", "leader"}, {2, "m1", "member"}, {3, "m2", "member"}, {4, "b1", "member"}} {
		dbtest.SeedAgent(t, db, a.id, a.name, a.kind)
	}
	dbtest.SeedSpace(t, db, 10, "alpha", 1)
	dbtest.SeedSpace(t, db, 20, "beta", 1)
	for _, p := range []struct {
		space, agent int64
		role         string
	}{{10, 1, "leader"}, {10, 2, "member"}, {10, 3, "member"}, {20, 1, "leader"}, {20, 4, "member"}} {
		dbtest.SeedParticipant(t, db, p.space, p.agent, p.role)
	}
	ib := inbox.NewService(db, broker.NewMemory(64), time.Now)
	return fixture{
		db:   db,
		svc:  NewService(db, spaces.NewResolver(db), ib, time.Now),
		ib:   ib,
		lead: auth.Principal{Kind: auth.Leader, AgentID: 1, AgentName: "lead"},
		m1:   auth.Principal{Kind: auth.Member, AgentID: 2, AgentName: "m1", SpaceID: 10},
		m2:   auth.Principal{Kind: auth.Member, AgentID: 3, AgentName: "m2", SpaceID: 10},
		b1:   auth.Principal{Kind: auth.Member, AgentID: 4, AgentName: "b1", SpaceID: 20},
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func readInbox(t *testing.T, ib *inbox.Service, p auth.Principal) []inbox.Entry {
	t.Helper()
	es, err := ib.Read(context.Background(), p, 0, 500)
	must(t, err)
	return es
}

// boardWithThread creates board "b" owned by owner and a thread by owner; returns thread id.
func boardWithThread(t *testing.T, f fixture, owner auth.Principal) int64 {
	t.Helper()
	ctx := context.Background()
	must(t, f.svc.Create(ctx, owner, "alpha", "b"))
	id, err := f.svc.CreateThread(ctx, owner, "alpha", "b", "plan", "first")
	must(t, err)
	return id
}
