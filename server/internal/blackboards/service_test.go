package blackboards

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

func TestBoardsLifecycle(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	boardWithThread(t, f, f.m1)
	apperrtest.WantKind(t, f.svc.Create(ctx, f.m2, "alpha", "b"), apperr.Conflict, "blackboard b already exists")
	must(t, f.svc.Create(ctx, f.b1, "beta", "b"))
	got, err := f.svc.List(ctx, f.lead, "alpha")
	must(t, err)
	if len(got) != 1 || got[0] != (BoardInfo{Name: "b", Owner: "m1", Threads: 1}) {
		t.Fatalf("List = %+v", got)
	}
	_, err = f.svc.ListThreads(ctx, f.lead, "alpha", "nope")
	apperrtest.WantKind(t, err, apperr.NotFound, "blackboard nope not found")
}

func TestCreateBadName(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	err := f.svc.Create(context.Background(), f.m1, "alpha", "Bad Name")
	if k, _ := apperr.KindOf(err); k != apperr.Invalid {
		t.Fatalf("err = %v, want invalid", err)
	}
}

func TestThreadValidation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	id := boardWithThread(t, f, f.m1)
	tests := []struct {
		name, title, body, msg string
	}{
		{"empty title", "", "x", "title must be 1..200 characters"},
		{"long title", strings.Repeat("é", 201), "x", "title must be 1..200 characters"},
		{"empty body", "t", "", "body must be 1..65536 bytes"},
		{"long body", "t", strings.Repeat("x", 65537), "body must be 1..65536 bytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.CreateThread(ctx, f.m1, "alpha", "b", tc.title, tc.body)
			apperrtest.WantKind(t, err, apperr.Invalid, tc.msg)
		})
	}
	_, err := f.svc.AddPost(ctx, f.m1, "alpha", id, "")
	apperrtest.WantKind(t, err, apperr.Invalid, "body must be 1..65536 bytes")
	_, err = f.svc.CreateThread(ctx, f.m1, "alpha", "b", strings.Repeat("é", 200), "x")
	must(t, err)
}

func TestReadAndListThreads(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	first := boardWithThread(t, f, f.m1)
	second, err := f.svc.CreateThread(ctx, f.m2, "alpha", "b", "other", "hello")
	must(t, err)
	_, err = f.svc.AddPost(ctx, f.lead, "alpha", first, "reply")
	must(t, err)
	list, err := f.svc.ListThreads(ctx, f.m1, "alpha", "b")
	must(t, err)
	if len(list) != 2 || list[0].ID != first || list[0].Posts != 2 || list[1].ID != second ||
		list[0].CreatedBy != "m1" || list[0].LastPostAt == "" {
		t.Fatalf("ListThreads = %+v", list)
	}
	th, err := f.svc.ReadThread(ctx, f.m1, "alpha", first)
	must(t, err)
	if th.Blackboard != "b" || th.Title != "plan" || th.CreatedBy != "m1" || !th.Subscribed ||
		len(th.Posts) != 2 || th.Posts[0].Body != "first" || th.Posts[1].Author != "lead" {
		t.Fatalf("ReadThread = %+v", th)
	}
	th, err = f.svc.ReadThread(ctx, f.lead, "alpha", first)
	must(t, err)
	if th.Subscribed {
		t.Fatalf("lead should not be subscribed")
	}
}

func TestPostNotifications(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	id := boardWithThread(t, f, f.m1)
	must(t, f.svc.Subscribe(ctx, f.m2, "alpha", id))
	must(t, f.svc.Subscribe(ctx, f.m2, "alpha", id)) // idempotent
	_, err := f.svc.AddPost(ctx, f.m2, "alpha", id, "hi")
	must(t, err)
	es := readInbox(t, f.ib, f.m1)
	want := inbox.Entry{Space: "alpha", Type: inbox.ThreadPost, From: "m2", Blackboard: "b",
		ThreadID: id, ThreadTitle: "plan", Body: "hi"}
	if len(es) != 1 || es[0].Space != want.Space || es[0].Type != want.Type || es[0].From != want.From ||
		es[0].Blackboard != want.Blackboard || es[0].ThreadID != id || es[0].ThreadTitle != "plan" || es[0].Body != "hi" {
		t.Fatalf("m1 inbox = %+v", es)
	}
	if es := readInbox(t, f.ib, f.m2); len(es) != 0 {
		t.Fatalf("author got %+v", es)
	}
	must(t, f.svc.Unsubscribe(ctx, f.m1, "alpha", id))
	must(t, f.svc.Unsubscribe(ctx, f.m1, "alpha", id))
	_, err = f.svc.AddPost(ctx, f.m2, "alpha", id, "again")
	must(t, err)
	if es := readInbox(t, f.ib, f.m1); len(es) != 1 {
		t.Fatalf("unsubscribed m1 got %+v", es)
	}
}

func TestRemoveThreadPermissions(t *testing.T) {
	t.Parallel()
	for _, who := range []string{"creator", "leader", "other"} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()
			id := boardWithThread(t, f, f.m1)
			must(t, f.svc.Subscribe(ctx, f.m2, "alpha", id))
			p := map[string]auth.Principal{"creator": f.m1, "leader": f.lead, "other": f.m2}[who]
			err := f.svc.RemoveThread(ctx, p, "alpha", id)
			if who == "other" {
				apperrtest.WantKind(t, err, apperr.Forbidden, "only the creator or a leader may remove thread "+strconv.FormatInt(id, 10))
				return
			}
			must(t, err)
			_, err = f.svc.ReadThread(ctx, f.m1, "alpha", id)
			apperrtest.WantKind(t, err, apperr.NotFound, "thread "+strconv.FormatInt(id, 10)+" not found")
			es := readInbox(t, f.ib, f.m2)
			if len(es) != 1 || es[0].Type != inbox.ThreadRemoved || es[0].ThreadID != id || es[0].ThreadTitle != "plan" {
				t.Fatalf("m2 inbox = %+v", es)
			}
			if es := readInbox(t, f.ib, p); len(es) != 0 {
				t.Fatalf("caller got %+v", es)
			}
		})
	}
}

func TestDeleteBoardPermissions(t *testing.T) {
	t.Parallel()
	for _, who := range []string{"owner", "leader", "other"} {
		t.Run(who, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()
			id := boardWithThread(t, f, f.m1)
			must(t, f.svc.Subscribe(ctx, f.m2, "alpha", id))
			must(t, f.svc.Subscribe(ctx, f.lead, "alpha", id))
			principal := map[string]auth.Principal{"owner": f.m1, "leader": f.lead, "other": f.m2}[who]
			err := f.svc.Delete(ctx, principal, "alpha", "b")
			if who == "other" {
				apperrtest.WantKind(t, err, apperr.Forbidden, "only the owner or a leader may delete blackboard b")
				return
			}
			must(t, err)
			boards, err := f.svc.List(ctx, f.lead, "alpha")
			must(t, err)
			if len(boards) != 0 {
				t.Fatalf("boards remain: %+v", boards)
			}
			if es := readInbox(t, f.ib, principal); len(es) != 0 {
				t.Fatalf("caller got %+v", es)
			}
			es := readInbox(t, f.ib, f.m2)
			if len(es) != 1 || es[0].Type != inbox.BlackboardRemoved || es[0].Blackboard != "b" {
				t.Fatalf("m2 inbox = %+v", es)
			}
			apperrtest.WantKind(t, f.svc.Delete(ctx, f.lead, "alpha", "b"), apperr.NotFound, "blackboard b not found")
		})
	}
}

func TestThreadFromOtherSpace(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	id := boardWithThread(t, f, f.m1)
	msg := "thread " + strconv.FormatInt(id, 10) + " not found"
	_, err := f.svc.ReadThread(ctx, f.lead, "beta", id)
	apperrtest.WantKind(t, err, apperr.NotFound, msg)
	_, err = f.svc.AddPost(ctx, f.b1, "beta", id, "x")
	apperrtest.WantKind(t, err, apperr.NotFound, msg)
	apperrtest.WantKind(t, f.svc.Subscribe(ctx, f.b1, "beta", id), apperr.NotFound, msg)
	apperrtest.WantKind(t, f.svc.Unsubscribe(ctx, f.b1, "beta", id), apperr.NotFound, msg)
	apperrtest.WantKind(t, f.svc.RemoveThread(ctx, f.lead, "beta", id), apperr.NotFound, msg)
}

func TestDeparture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		role       spaces.Role
		boardsLeft int
		entries    int
	}{
		{"member leaves: boards gone", spaces.RoleMember, 0, 1},
		{"leader leaves: boards remain", spaces.RoleLeader, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ctx := context.Background()
			owner := f.m1
			if tc.role == spaces.RoleLeader {
				owner = f.lead
			}
			id := boardWithThread(t, f, owner)
			must(t, f.svc.Subscribe(ctx, f.m2, "alpha", id))
			m := spaces.Membership{SpaceID: 10, SpaceName: "alpha", AgentID: owner.AgentID, AgentName: owner.AgentName, Role: tc.role}
			tx, err := f.db.BeginTx(ctx, nil)
			must(t, err)
			es, err := f.svc.OnDeparture(ctx, tx, m)
			must(t, err)
			must(t, tx.Commit())
			if len(es) != tc.entries {
				t.Fatalf("entries = %+v", es)
			}
			if tc.entries == 1 && (es[0].AgentID != 3 || es[0].Type != inbox.BlackboardRemoved ||
				es[0].Blackboard != "b" || es[0].Space != "alpha" || es[0].SpaceID != 10 || es[0].From != owner.AgentName) {
				t.Fatalf("entry = %+v", es[0])
			}
			boards, err := f.svc.List(ctx, f.m2, "alpha")
			must(t, err)
			if len(boards) != tc.boardsLeft {
				t.Fatalf("boards = %+v", boards)
			}
			if tc.boardsLeft == 1 {
				var n int
				must(t, f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM thread_subscriptions WHERE agent_id = ?`, owner.AgentID).Scan(&n))
				if n != 0 {
					t.Fatalf("departing subscriptions remain: %d", n)
				}
			}
		})
	}
}
