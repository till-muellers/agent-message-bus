package spaces

import (
	"context"
	"database/sql"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	db := dbtest.Open(t)
	const ts = "2026-01-01T00:00:00Z"
	mustExec(t, db, `INSERT INTO agents (id, name, kind, created_at) VALUES
		(1,'lead','leader',?),(2,'work','member',?),(3,'other','leader',?)`, ts, ts, ts)
	mustExec(t, db, `INSERT INTO spaces (id, name, created_by, created_at) VALUES (10,'alpha',1,?),(20,'beta',1,?)`, ts, ts)
	mustExec(t, db, `INSERT INTO participants (space_id, agent_id, role, joined_at) VALUES
		(10,1,'leader',?),(10,2,'member',?),(20,1,'leader',?)`, ts, ts, ts)
	r := NewResolver(db)
	lead := auth.Principal{Kind: auth.Leader, AgentID: 1, AgentName: "lead"}
	work := auth.Principal{Kind: auth.Member, AgentID: 2, AgentName: "work", SpaceID: 10}
	tests := []struct {
		name     string
		p        auth.Principal
		space    string
		want     Membership
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "leader", p: lead, space: "alpha", want: Membership{10, "alpha", 1, "lead", RoleLeader}},
		{name: "member", p: work, space: "alpha", want: Membership{10, "alpha", 2, "work", RoleMember}},
		{name: "admin", p: auth.Principal{Kind: auth.Admin}, space: "alpha", wantKind: apperr.Forbidden,
			wantMsg: "admin token cannot act in spaces; mint a leader with bus_mint_leader"},
		{name: "unknown space", p: lead, space: "nope", wantKind: apperr.NotFound, wantMsg: "space nope not found"},
		{name: "member other space", p: work, space: "beta", wantKind: apperr.Forbidden,
			wantMsg: "token is not valid for space beta"},
		{name: "not participant", p: auth.Principal{Kind: auth.Leader, AgentID: 3, AgentName: "other"},
			space: "alpha", wantKind: apperr.Forbidden, wantMsg: "not a participant of space alpha"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Resolve(context.Background(), tc.p, tc.space)
			if tc.wantKind != "" {
				kind, _ := apperr.KindOf(err)
				if kind != tc.wantKind || err.Error() != tc.wantMsg {
					t.Fatalf("err = %v (%s), want %s %q", err, kind, tc.wantKind, tc.wantMsg)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
