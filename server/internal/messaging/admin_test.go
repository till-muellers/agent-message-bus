package messaging

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

const joinFirst = "join space alpha first; only participants receive messages"

// withAdmin adds the admin agent "ops", which has not joined alpha, and the leader
// "lead2", which is no participant of alpha.
func withAdmin(t *testing.T, w *world) {
	t.Helper()
	dbtest.SeedAgent(t, w.db, 9, "ops", "admin")
	dbtest.SeedAgent(t, w.db, 10, "lead2", "leader")
	w.p["ops"] = auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
	w.p["lead2"] = auth.Principal{Kind: auth.Leader, AgentID: 10, AgentName: "lead2"}
}

// adminEntries counts the inbox rows of the admin "ops".
func adminEntries(t *testing.T, w *world) int {
	t.Helper()
	var n int
	if err := w.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM inbox WHERE agent_id = 9`).Scan(&n); err != nil {
		t.Fatalf("counting admin entries: %v", err)
	}
	return n
}

type adminCase struct {
	name    string
	setup   []step
	op      step
	kind    apperr.Kind
	msg     string
	inboxes map[string][]string
}

func runAdminCases(t *testing.T, tests []adminCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			w := newWorld(t)
			withAdmin(t, w)
			for i, s := range tc.setup {
				if err := s(ctx, w); err != nil {
					t.Fatalf("setup %d: %v", i, err)
				}
			}
			err := tc.op(ctx, w)
			if tc.kind == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.kind != "" {
				apperrtest.WantKind(t, err, tc.kind, tc.msg)
			}
			for name, want := range tc.inboxes {
				if got := w.inbox(t, name); !reflect.DeepEqual(got, want) {
					t.Errorf("inbox %s = %q, want %q", name, got, want)
				}
			}
			if n := adminEntries(t, w); n != 0 {
				t.Errorf("unjoined admin has %d inbox entries", n)
			}
		})
	}
}

func TestAdminActsMessaging(t *testing.T) {
	t.Parallel()
	runAdminCases(t, []adminCase{
		{name: "direct message", op: direct("ops", "bob", "hi", false),
			inboxes: map[string][]string{"bob": {"direct_message:ops::hi"}}},
		{name: "critical direct message", op: direct("ops", "bob", "stop", true),
			inboxes: map[string][]string{"bob": {"direct_message:ops::stop:critical"}}},
		{name: "send to all", op: sendTopic("ops", "all", "hi", false),
			inboxes: map[string][]string{"lead": {"topic_message:ops:all:hi"}, "ann": {"topic_message:ops:all:hi"}}},
		{name: "send to a topic", setup: []step{create("ann", "dev"), sub("bob", "dev")}, op: sendTopic("ops", "dev", "m", false),
			inboxes: map[string][]string{"bob": {"topic_message:ops:dev:m"}, "ann": {"topic_message:ops:dev:m"}}},
		{name: "create a topic", op: create("ops", "ops-notes")},
		{name: "close a topic it did not create", setup: []step{create("ann", "dev"), sub("bob", "dev")}, op: closeT("ops", "dev"),
			inboxes: map[string][]string{"bob": {"topic_closed:ops:dev:"}}},
	})
}

func TestUnjoinedAdminUnreachable(t *testing.T) {
	t.Parallel()
	runAdminCases(t, []adminCase{
		{name: "direct message to the admin", op: direct("ann", "ops", "hello", false),
			kind: apperr.NotFound, msg: "participant ops not in space alpha"},
		{name: "all skips the admin", op: sendTopic("lead", "all", "hi", false)},
		{name: "topic subscribe", setup: []step{create("ann", "dev")}, op: sub("ops", "dev"), kind: apperr.Forbidden, msg: joinFirst},
		{name: "topic unsubscribe", setup: []step{create("ann", "dev")}, op: unsub("ops", "dev"), kind: apperr.Forbidden, msg: joinFirst},
		{name: "own topic does not subscribe", setup: []step{create("ops", "ops-notes")}, op: sendTopic("ann", "ops-notes", "x", false)},
	})
}

func TestAuthorityDoesNotLeak(t *testing.T) {
	t.Parallel()
	runAdminCases(t, []adminCase{
		{name: "leader not participant", op: direct("lead2", "bob", "x", false),
			kind: apperr.Forbidden, msg: "not a participant of space alpha"},
		{name: "member critical", op: direct("ann", "lead", "x", true),
			kind: apperr.Forbidden, msg: "only leaders may send critical messages"},
		{name: "member to all", op: sendTopic("ann", "all", "x", false),
			kind: apperr.Forbidden, msg: "only leaders may send to all"},
	})
}

func TestAdminListsTopicsUnsubscribed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	withAdmin(t, w)
	if err := create("ops", "ops-notes")(ctx, w); err != nil {
		t.Fatal(err)
	}
	got, err := w.svc.ListTopics(ctx, w.p["ops"], space)
	want := []TopicInfo{{Name: "all", CreatedBy: "lead"}, {Name: "ops-notes", CreatedBy: "ops"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ListTopics = %+v, %v; want %+v", got, err, want)
	}
}
