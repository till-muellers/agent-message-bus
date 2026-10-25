package messaging

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
	db := dbtest.Open(t)
	ib := inbox.NewService(db, broker.NewMemory(64), time.Now)
	w := &world{svc: NewService(db, spaces.NewResolver(db), ib, time.Now), ib: ib, db: db, p: map[string]auth.Principal{}}
	lead := seed(t, db, `INSERT INTO agents (name, kind, created_at) VALUES ('lead', 'leader', '')`)
	sp := seed(t, db, `INSERT INTO spaces (name, created_by, created_at) VALUES (?, ?, '')`, space, lead)
	seed(t, db, `INSERT INTO topics (space_id, name, created_by, created_at) VALUES (?, 'all', ?, '')`, sp, lead)
	w.addParticipant(t, sp, lead, "lead", spaces.RoleLeader)
	for _, n := range []string{"ann", "bob"} {
		id := seed(t, db, `INSERT INTO agents (name, kind, created_at) VALUES (?, 'member', '')`, n)
		w.addParticipant(t, sp, id, n, spaces.RoleMember)
	}
	return w
}

func (w *world) addParticipant(t *testing.T, sp, id int64, name string, role spaces.Role) {
	t.Helper()
	seed(t, w.db, `INSERT INTO participants (space_id, agent_id, role, joined_at) VALUES (?, ?, ?, '')`, sp, id, role)
	kind := auth.Leader
	if role == spaces.RoleMember {
		kind = auth.Member
	}
	w.p[name] = auth.Principal{Kind: kind, AgentID: id, AgentName: name, SpaceID: sp}
	if kind == auth.Leader {
		w.p[name] = auth.Principal{Kind: kind, AgentID: id, AgentName: name}
	}
}

func seed(t *testing.T, db *sql.DB, q string, args ...any) int64 {
	t.Helper()
	res, err := db.Exec(q, args...)
	if err != nil {
		t.Fatalf("seeding %q: %v", q, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seeding id: %v", err)
	}
	return id
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
