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
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	other := leader(t, e, 3, "other")
	dbtest.SeedSpace(t, e.db, 10, "alpha", 1)
	dbtest.SeedSpace(t, e.db, 20, "beta", 1)
	work := authenticated(t, e, 2, "work", "member", 10)
	dbtest.SeedParticipant(t, e.db, 10, 1, "leader")
	dbtest.SeedParticipant(t, e.db, 10, 2, "member")
	dbtest.SeedParticipant(t, e.db, 20, 1, "leader")
	revoked := authenticated(t, e, 4, "gone", "leader", 0)
	dbtest.SeedParticipant(t, e.db, 10, 4, "leader")
	dbtest.Exec(t, e.db, `DELETE FROM tokens WHERE agent_id = 4`)
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
		{name: "unknown space", p: lead, space: "nope", wantKind: apperr.NotFound, wantMsg: "space nope not found"},
		{name: "member other space", p: work, space: "beta", wantKind: apperr.Forbidden,
			wantMsg: "token is not valid for space beta"},
		{name: "not participant", p: other,
			space: "alpha", wantKind: apperr.Forbidden, wantMsg: "not a participant of space alpha"},
		{name: "revoked token", p: revoked, space: "alpha", wantKind: apperr.Unauthenticated, wantMsg: "invalid token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.svc.r.Resolve(context.Background(), e.db, tc.p, tc.space)
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
