package blackboards

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// A re-invited member keeps its boards but loses its thread subscriptions in the space.
func TestReadmissionDropsSubscriptionsAndKeepsBoards(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	id := boardWithThread(t, f, f.m1)
	m := spaces.Membership{SpaceID: 10, SpaceName: "alpha", AgentID: f.m1.AgentID, AgentName: "m1", Role: spaces.RoleMember}
	must(t, f.svc.OnReadmission(ctx, f.db, m))
	boards, err := f.svc.List(ctx, f.m2, "alpha")
	must(t, err)
	if len(boards) != 1 {
		t.Fatalf("boards = %+v, want the member's board kept", boards)
	}
	if subscribed(t, f, f.m1, id) {
		t.Fatal("re-admitted member is still subscribed to its thread")
	}
}
