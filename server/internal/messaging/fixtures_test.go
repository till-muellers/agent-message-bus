package messaging

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

const space = "alpha"

// world is space alpha with leader "lead", members "ann" and "bob", and the open all topic.
type world struct {
	svc *Service
	ib  *inbox.Service
	db  *sql.DB
	p   map[string]auth.Principal
}

func newWorld(t *testing.T) *world {
	t.Helper()
	conn := dbtest.Open(t)
	ib := inbox.NewService(t.Context(), conn, broker.NewMemory(64), time.Now)
	w := &world{svc: NewService(conn, spaces.NewResolver(conn), ib, time.Now), ib: ib, db: conn, p: map[string]auth.Principal{}}
	const sp = 1
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	dbtest.SeedSpace(t, conn, sp, space, 1)
	w.addParticipant(t, sp, 1, "lead", spaces.RoleLeader)
	for i, n := range []string{"ann", "bob"} {
		id := int64(i + 2)
		dbtest.SeedAgent(t, conn, id, n, "member")
		w.addParticipant(t, sp, id, n, spaces.RoleMember)
	}
	creator := spaces.Membership{SpaceID: sp, SpaceName: space, AgentID: 1, AgentName: "lead", Role: spaces.RoleLeader}
	err := db.InTx(context.Background(), conn, func(tx *sql.Tx) error {
		return w.svc.OnSpaceCreated(context.Background(), tx, creator)
	})
	if err != nil {
		t.Fatalf("creating all topic: %v", err)
	}
	return w
}

func (w *world) addParticipant(t *testing.T, sp, id int64, name string, role spaces.Role) {
	t.Helper()
	dbtest.SeedParticipant(t, w.db, sp, id, string(role))
	kind := auth.Leader
	if role == spaces.RoleMember {
		kind = auth.Member
	}
	w.p[name] = auth.Principal{Kind: kind, AgentID: id, AgentName: name, SpaceID: sp}
	if kind == auth.Leader {
		w.p[name] = auth.Principal{Kind: kind, AgentID: id, AgentName: name}
	}
}

// inbox returns the entries of agent name as "type:from:topic:body" strings.
func (w *world) inbox(t *testing.T, name string) []string {
	t.Helper()
	es, err := w.ib.Read(context.Background(), w.p[name], 0, 500)
	if err != nil {
		t.Fatalf("reading inbox of %s: %v", name, err)
	}
	out := []string{}
	for _, e := range es {
		s := string(e.Type) + ":" + e.From + ":" + e.Topic + ":" + e.Body
		if e.Critical {
			s += ":critical"
		}
		if e.Space != space {
			s += ":wrongspace"
		}
		out = append(out, s)
	}
	return out
}
