package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

func TestWhoami(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	lead := leader(t, e, 1, "lead")
	mustCreate(t, e, lead, "beta")
	mustCreate(t, e, lead, "alpha")
	member, _ := inviteMember(t, e, lead, "alpha", "work")
	tests := []struct {
		name string
		p    auth.Principal
		want WhoamiResult
	}{
		{name: "admin", p: admin, want: WhoamiResult{Kind: "admin", Spaces: []SpaceRole{}}},
		{name: "leader", p: lead, want: WhoamiResult{Kind: "leader", Name: "lead",
			Spaces: []SpaceRole{{"alpha", RoleLeader}, {"beta", RoleLeader}}}},
		{name: "member", p: member, want: WhoamiResult{Kind: "member", Name: "work", Space: "alpha",
			Spaces: []SpaceRole{{"alpha", RoleMember}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.svc.Whoami(context.Background(), tc.p)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestWhoamiAdmin(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, nil)
	p := auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: auth.BootstrapName, Bootstrap: true}
	got, err := e.svc.Whoami(context.Background(), p)
	want := WhoamiResult{Kind: "admin", Name: "admin", Spaces: []SpaceRole{}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
}
