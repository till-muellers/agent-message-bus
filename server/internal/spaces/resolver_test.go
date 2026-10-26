package spaces

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func TestResolve(t *testing.T) {
	t.Parallel()
	conn := dbtest.Open(t)
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	dbtest.SeedAgent(t, conn, 2, "work", "member")
	dbtest.SeedAgent(t, conn, 3, "other", "leader")
	dbtest.SeedSpace(t, conn, 10, "alpha", 1)
	dbtest.SeedSpace(t, conn, 20, "beta", 1)
	dbtest.SeedParticipant(t, conn, 10, 1, "leader")
	dbtest.SeedParticipant(t, conn, 10, 2, "member")
	dbtest.SeedParticipant(t, conn, 20, 1, "leader")
	r := NewResolver(conn)
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
		{name: "leader", p: lead, space: "alpha",
			want: Membership{SpaceID: 10, SpaceName: "alpha", AgentID: 1, AgentName: "lead", Role: RoleLeader}},
		{name: "member", p: work, space: "alpha",
			want: Membership{SpaceID: 10, SpaceName: "alpha", AgentID: 2, AgentName: "work", Role: RoleMember}},
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
				apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
