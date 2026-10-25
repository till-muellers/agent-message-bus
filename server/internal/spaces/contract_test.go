package spaces

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func TestMembershipNotify(t *testing.T) {
	t.Parallel()
	m := Membership{SpaceID: 10, SpaceName: "alpha", AgentID: 1, AgentName: "lead"}
	proto := inbox.Entry{Type: inbox.TopicMessage, Topic: "dev", Body: "hi", Critical: true}
	got := m.Notify([]int64{2, 3}, proto)
	want := []inbox.Entry{
		{AgentID: 2, SpaceID: 10, Space: "alpha", From: "lead", Type: inbox.TopicMessage, Topic: "dev", Body: "hi", Critical: true},
		{AgentID: 3, SpaceID: 10, Space: "alpha", From: "lead", Type: inbox.TopicMessage, Topic: "dev", Body: "hi", Critical: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Notify = %+v, want %+v", got, want)
	}
	if got := m.Notify(nil, proto); len(got) != 0 {
		t.Fatalf("Notify(nil) = %v, want empty", got)
	}
}

func TestResolverParticipants(t *testing.T) {
	t.Parallel()
	conn := dbtest.Open(t)
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	dbtest.SeedAgent(t, conn, 2, "work", "member")
	dbtest.SeedAgent(t, conn, 3, "out", "member")
	dbtest.SeedSpace(t, conn, 10, "alpha", 1)
	dbtest.SeedParticipant(t, conn, 10, 2, "member")
	dbtest.SeedParticipant(t, conn, 10, 1, "leader")
	r, ctx := NewResolver(conn), context.Background()

	ids, err := r.ParticipantIDs(ctx, conn, 10)
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatalf("ParticipantIDs = %v, %v; want [1 2]", ids, err)
	}
	tests := []struct {
		name   string
		wantID int64
		found  bool
	}{{"work", 2, true}, {"out", 0, false}, {"nobody", 0, false}}
	for _, tc := range tests {
		id, found, err := r.ParticipantID(ctx, conn, 10, tc.name)
		if err != nil || id != tc.wantID || found != tc.found {
			t.Fatalf("ParticipantID(%q) = %d, %v, %v", tc.name, id, found, err)
		}
	}
}
