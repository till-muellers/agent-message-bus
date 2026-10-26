package invites

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

// TestListPending: redeemed, expired and revoked codes disappear, the code is never
// listed, soonest expiry first; only leaders of the space and admins may list.
func TestListPending(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	expired := mustCreate(t, e, "old", nil)
	*e.clock = start.Add(5 * time.Minute)
	redeemed := mustCreate(t, e, "joined", nil)
	member, err := e.svc.Accept(ctx, redeemed.Code)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	revoked := mustCreate(t, e, "gone", nil)
	if _, err := e.spaces.Revoke(ctx, e.lead, "demo", "gone"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	*e.clock = start.Add(6 * time.Minute)
	second := mustCreate(t, e, "second", nil)
	*e.clock = start.Add(7 * time.Minute)
	first := mustCreate(t, e, "first", nil)
	*e.clock = start.Add(10 * time.Minute)

	admin := auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
	dbtest.SeedAgent(t, e.db, 9, "ops", "admin")
	want := []Pending{
		{Name: "second", InvitedBy: "lead", ExpiresAt: "2026-01-02T03:20:05Z"},
		{Name: "first", InvitedBy: "lead", ExpiresAt: "2026-01-02T03:21:05Z"},
	}
	for _, p := range []auth.Principal{e.lead, admin} {
		got := listPending(t, e, p, "demo")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: ListPending = %+v, want %+v", p.AgentName, got, want)
		}
		raw, _ := json.Marshal(got)
		for _, inv := range []Invite{expired, redeemed, revoked, second, first} {
			if strings.Contains(string(raw), inv.Code) || strings.Contains(string(raw), normalize(inv.Code)) {
				t.Fatalf("listing %s contains a code", raw)
			}
		}
		if n, err := e.svc.CountPending(ctx, p, "demo"); err != nil || n != 2 {
			t.Fatalf("%s: CountPending = %d, %v; want 2", p.AgentName, n, err)
		}
	}
	memberP, err := e.auth.Authenticate(ctx, member.Token)
	if err != nil {
		t.Fatalf("authenticate member: %v", err)
	}
	dbtest.SeedAgent(t, e.db, 7, "stranger", "leader")
	stranger := auth.Principal{Kind: auth.Leader, AgentID: 7, AgentName: "stranger"}
	denied := []struct {
		name, space string
		p           auth.Principal
		kind        apperr.Kind
		msg         string
	}{
		{"member", "demo", memberP, apperr.Forbidden, "only leaders may list or cancel invites"},
		{"leader not in the space", "demo", stranger, apperr.Forbidden, "not a participant of space demo"},
		{"unknown space", "nope", e.lead, apperr.NotFound, "space nope not found"},
	}
	for _, tc := range denied {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.ListPending(ctx, tc.p, tc.space)
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
			_, err = e.svc.CountPending(ctx, tc.p, tc.space)
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
		})
	}
}

func listPending(t *testing.T, e env, p auth.Principal, space string) []Pending {
	t.Helper()
	got, err := e.svc.ListPending(context.Background(), p, space)
	if err != nil {
		t.Fatalf("ListPending as %s: %v", p.AgentName, err)
	}
	return got
}
