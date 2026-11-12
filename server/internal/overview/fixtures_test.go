package overview_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/blackboards"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/messaging"
	"github.com/till-muellers/agent-message-bus/server/internal/overview"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
)

// world mirrors the Background of server/spec/overview.feature: leader "lead" created
// spaces "demo" and "beta"; members "worker" and "peer" are in "demo"; leader "lead2" and
// admin "ops" exist but joined nothing. Every service shares one database and a clock that
// advances one second per call, so lastPostAt orders strictly.
type world struct {
	conn     *sql.DB
	reg      *tools.Registry
	sp       *spaces.Service
	msg      *messaging.Service
	bb       *blackboards.Service
	ib       *inbox.Service
	inv      *invites.Service
	ov       *overview.Service
	resolves *atomic.Int64 // Resolve calls of messaging, blackboards and overview
	lead     auth.Principal
	lead2    auth.Principal
	ops      auth.Principal
	wkr      auth.Principal
	peer     auth.Principal
}

// anyCredential accepts every principal: these tests do not exercise credential
// re-validation, which the spaces tests cover.
type anyCredential struct{}

func (anyCredential) StillValid(context.Context, db.DBTX, auth.Principal) error { return nil }

// countingResolver counts the Resolve calls it forwards to the real resolver.
type countingResolver struct {
	*spaces.Resolver
	n *atomic.Int64
}

func (r countingResolver) Resolve(ctx context.Context, q db.DBTX, p auth.Principal, space string) (spaces.Membership, error) {
	r.n.Add(1)
	return r.Resolver.Resolve(ctx, q, p, space)
}

func newWorld(t *testing.T) *world {
	t.Helper()
	conn := dbtest.Open(t)
	now := stepClock()
	ib := inbox.NewService(t.Context(), conn, broker.NewMemory(64), anyCredential{}, now)
	r := countingResolver{Resolver: spaces.NewResolver(anyCredential{}), n: &atomic.Int64{}}
	w := &world{conn: conn, ib: ib, resolves: r.n,
		msg: messaging.NewService(conn, r, ib, now), bb: blackboards.NewService(conn, r, ib, now)}
	authSvc := auth.NewService(conn, "overview-test-admin-token-0123456789", now)
	w.sp = spaces.NewService(conn, r.Resolver, authSvc, ib, now, spaces.Hooks{
		Departure: []spaces.DepartureHook{w.msg, w.bb}, Creation: []spaces.CreationHook{w.msg}})
	w.inv = invites.NewService(conn, spaces.NewAdmitter(r.Resolver, authSvc, ib, now, nil), ib, now)
	w.ov = overview.NewService(overview.Deps{DB: conn, Resolver: r, Spaces: w.sp, Topics: w.msg,
		Boards: w.bb, Invites: w.inv, Inbox: ib})
	w.reg = tools.New(tools.Deps{Auth: authSvc, Spaces: w.sp, Messaging: w.msg, Blackboards: w.bb, Inbox: ib,
		Overview: w.ov})
	w.seed(t)
	return w
}

// seed creates the agents, the spaces demo and beta, and the demo members.
func (w *world) seed(t *testing.T) {
	t.Helper()
	for _, a := range []struct {
		id         int64
		name, kind string
	}{{1, "lead", "leader"}, {2, "worker", "member"}, {3, "peer", "member"}, {4, "lead2", "leader"}, {9, "ops", "admin"}} {
		dbtest.SeedAgent(t, w.conn, a.id, a.name, a.kind)
	}
	w.lead = auth.Principal{Kind: auth.Leader, AgentID: 1, AgentName: "lead"}
	w.lead2 = auth.Principal{Kind: auth.Leader, AgentID: 4, AgentName: "lead2"}
	w.ops = auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
	w.must(t, w.sp.Create(t.Context(), w.lead, "demo"))
	w.must(t, w.sp.Create(t.Context(), w.lead, "beta"))
	demo := w.spaceID(t, "demo")
	dbtest.SeedParticipant(t, w.conn, demo, 2, "member")
	dbtest.SeedParticipant(t, w.conn, demo, 3, "member")
	w.wkr = auth.Principal{Kind: auth.Member, AgentID: 2, AgentName: "worker", SpaceID: demo}
	w.peer = auth.Principal{Kind: auth.Member, AgentID: 3, AgentName: "peer", SpaceID: demo}
}

func (w *world) spaceID(t *testing.T, name string) int64 {
	t.Helper()
	m, err := spaces.NewResolver(anyCredential{}).Resolve(t.Context(), w.conn, w.lead, name)
	w.must(t, err)
	return m.SpaceID
}

func (w *world) must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// thread creates a thread with title on board in demo as p and returns its id.
func (w *world) thread(t *testing.T, p auth.Principal, board, title string) int64 {
	t.Helper()
	id, err := w.bb.CreateThread(t.Context(), p, "demo", board, title, "first")
	w.must(t, err)
	return id
}

// overview calls bus_space_overview as p with raw JSON args.
func (w *world) overview(ctx context.Context, t *testing.T, p auth.Principal, args string) (any, error) {
	t.Helper()
	return w.reg.Call(ctx, p, tools.BusSpaceOverview, json.RawMessage(args))
}

// stepClock returns a clock that starts at 2026-01-01 and advances one second per call.
func stepClock() func() time.Time {
	var n atomic.Int64
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return base.Add(time.Duration(n.Add(1)) * time.Second) }
}
