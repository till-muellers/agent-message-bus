package blackboards

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

// unjoinedAdmin seeds the admin agent "ops", which joined no space.
func unjoinedAdmin(t *testing.T, f fixture) auth.Principal {
	t.Helper()
	dbtest.SeedAgent(t, f.db, 9, "ops", "admin")
	return auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
}

func TestAdminActsBlackboards(t *testing.T) {
	t.Parallel()
	const joinFirst = "join space alpha first; only participants receive messages"
	tests := []struct {
		name string
		op   func(ctx context.Context, f fixture, ops auth.Principal, thread int64) error
		kind apperr.Kind
		msg  string
	}{
		{"list boards", func(ctx context.Context, f fixture, ops auth.Principal, _ int64) error {
			_, err := f.svc.List(ctx, ops, "alpha")
			return err
		}, "", ""},
		{"read thread", func(ctx context.Context, f fixture, ops auth.Principal, id int64) error {
			th, err := f.svc.ReadThread(ctx, ops, "alpha", id)
			if err == nil && th.Subscribed {
				t.Error("unjoined admin reads as subscribed")
			}
			return err
		}, "", ""},
		{"create board and thread", func(ctx context.Context, f fixture, ops auth.Principal, _ int64) error {
			if err := f.svc.Create(ctx, ops, "alpha", "audit"); err != nil {
				return err
			}
			id, err := f.svc.CreateThread(ctx, ops, "alpha", "audit", "findings", "first")
			if err == nil && countSubscriptions(t, f, id) != 0 {
				t.Error("unjoined admin subscribed to its own thread")
			}
			return err
		}, "", ""},
		{"post to any thread", func(ctx context.Context, f fixture, ops auth.Principal, id int64) error {
			_, _, err := f.svc.AddPost(ctx, ops, "alpha", id, "reply")
			if err == nil && len(readInbox(t, f.ib, f.m1)) != 1 {
				t.Error("thread subscriber m1 got no post notification")
			}
			return err
		}, "", ""},
		{"remove a thread it did not create", func(ctx context.Context, f fixture, ops auth.Principal, id int64) error {
			return f.svc.RemoveThread(ctx, ops, "alpha", id)
		}, "", ""},
		{"delete a board it does not own", func(ctx context.Context, f fixture, ops auth.Principal, _ int64) error {
			return f.svc.Delete(ctx, ops, "alpha", "b")
		}, "", ""},
		{"subscribe needs joining", func(ctx context.Context, f fixture, ops auth.Principal, id int64) error {
			return f.svc.Subscribe(ctx, ops, "alpha", id)
		}, apperr.Forbidden, joinFirst},
		{"unsubscribe needs joining", func(ctx context.Context, f fixture, ops auth.Principal, id int64) error {
			return f.svc.Unsubscribe(ctx, ops, "alpha", id)
		}, apperr.Forbidden, joinFirst},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			ops := unjoinedAdmin(t, f)
			err := tc.op(context.Background(), f, ops, boardWithThread(t, f, f.m1))
			if tc.kind == "" {
				must(t, err)
				return
			}
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
		})
	}
}

// countSubscriptions counts the thread subscriptions of the admin "ops" on thread id.
func countSubscriptions(t *testing.T, f fixture, id int64) int {
	t.Helper()
	var n int
	err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM thread_subscriptions WHERE thread_id = ? AND agent_id = 9`, id).Scan(&n)
	must(t, err)
	return n
}
