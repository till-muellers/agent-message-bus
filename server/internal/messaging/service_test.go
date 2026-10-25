package messaging

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

type step func(ctx context.Context, w *world) error

func create(by, name string) step {
	return func(ctx context.Context, w *world) error { return w.svc.CreateTopic(ctx, w.p[by], space, name) }
}

func sendTopic(by, topic, body string, critical bool) step {
	return func(ctx context.Context, w *world) error {
		return w.svc.SendTopic(ctx, w.p[by], space, topic, body, critical)
	}
}

func sub(by, topic string) step {
	return func(ctx context.Context, w *world) error { return w.svc.Subscribe(ctx, w.p[by], space, topic) }
}

func unsub(by, topic string) step {
	return func(ctx context.Context, w *world) error { return w.svc.Unsubscribe(ctx, w.p[by], space, topic) }
}

func closeT(by, topic string) step {
	return func(ctx context.Context, w *world) error { return w.svc.CloseTopic(ctx, w.p[by], space, topic) }
}

func direct(by, to, body string, critical bool) step {
	return func(ctx context.Context, w *world) error {
		return w.svc.SendDirect(ctx, w.p[by], space, to, body, critical)
	}
}

func depart(name string) step {
	return func(ctx context.Context, w *world) error {
		p := w.p[name]
		m, err := spaces.NewResolver(w.db).Resolve(ctx, p, space)
		if err != nil {
			return err
		}
		es, err := w.svc.OnDeparture(ctx, w.db, m)
		if len(es) != 0 {
			return errors.New("departure returned entries")
		}
		return err
	}
}

func TestOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   []step
		op      step
		kind    apperr.Kind
		msg     string
		inboxes map[string][]string
	}{
		{name: "direct message", op: direct("ann", "bob", "hi", false),
			inboxes: map[string][]string{"bob": {"direct_message:ann::hi"}, "ann": {}, "lead": {}}},
		{name: "critical direct by leader", op: direct("lead", "bob", "now", true),
			inboxes: map[string][]string{"bob": {"direct_message:lead::now:critical"}}},
		{name: "critical direct by member", op: direct("ann", "bob", "x", true),
			kind: apperr.Forbidden, msg: "only leaders may send critical messages", inboxes: map[string][]string{"bob": {}}},
		{name: "direct to unknown", op: direct("ann", "zed", "x", false),
			kind: apperr.NotFound, msg: "participant zed not in space alpha"},
		{name: "direct to self", op: direct("ann", "ann", "x", false), kind: apperr.Invalid},
		{name: "empty body", op: direct("ann", "bob", "", false), kind: apperr.Invalid, msg: "body must be 1..65536 bytes"},
		{name: "oversize body", op: direct("ann", "bob", strings.Repeat("x", 65537), false),
			kind: apperr.Invalid, msg: "body must be 1..65536 bytes"},
		{name: "max body ok", op: direct("ann", "bob", strings.Repeat("x", 65536), false)},
		{name: "topic fan-out excludes sender", setup: []step{create("ann", "dev"), sub("bob", "dev"), sub("lead", "dev")},
			op:      sendTopic("ann", "dev", "m", false),
			inboxes: map[string][]string{"ann": {}, "bob": {"topic_message:ann:dev:m"}, "lead": {"topic_message:ann:dev:m"}}},
		{name: "all reaches every participant", op: sendTopic("lead", "all", "hey", false),
			inboxes: map[string][]string{"lead": {}, "ann": {"topic_message:lead:all:hey"}, "bob": {"topic_message:lead:all:hey"}}},
		{name: "member to all forbidden", op: sendTopic("ann", "all", "x", false),
			kind: apperr.Forbidden, msg: "only leaders may send to all", inboxes: map[string][]string{"bob": {}}},
		{name: "member critical to topic forbidden", setup: []step{create("ann", "dev")},
			op: sendTopic("ann", "dev", "x", true), kind: apperr.Forbidden, msg: "only leaders may send critical messages"},
		{name: "unknown topic", op: sendTopic("ann", "nope", "x", false), kind: apperr.NotFound},
		{name: "closed topic cannot be sent to", setup: []step{create("ann", "dev"), closeT("ann", "dev")},
			op: sendTopic("ann", "dev", "x", false), kind: apperr.NotFound},
		{name: "closed topic name reusable", setup: []step{create("ann", "dev"), closeT("lead", "dev")}, op: create("bob", "dev")},
		{name: "duplicate open topic", setup: []step{create("ann", "dev")}, op: create("bob", "dev"),
			kind: apperr.Conflict, msg: "topic dev is already open"},
		{name: "create all", op: create("ann", "all"), kind: apperr.Conflict},
		{name: "invalid topic name", op: create("ann", "Bad Name"), kind: apperr.Invalid},
		{name: "subscribers get topic_closed", setup: []step{create("ann", "dev"), sub("bob", "dev"), sub("lead", "dev")},
			op:      closeT("lead", "dev"),
			inboxes: map[string][]string{"lead": {}, "ann": {"topic_closed:lead:dev:"}, "bob": {"topic_closed:lead:dev:"}}},
		{name: "close by non-creator member", setup: []step{create("ann", "dev")}, op: closeT("bob", "dev"),
			kind: apperr.Forbidden, msg: "only the creator or a leader may close topic dev"},
		{name: "close all", op: closeT("lead", "all"), kind: apperr.Forbidden, msg: "the all topic cannot be closed"},
		{name: "unsubscribe stops delivery", setup: []step{create("ann", "dev"), sub("bob", "dev"), unsub("bob", "dev")},
			op: sendTopic("ann", "dev", "m", false), inboxes: map[string][]string{"bob": {}}},
		{name: "subscribe idempotent", setup: []step{create("ann", "dev"), sub("bob", "dev")}, op: sub("bob", "dev")},
		{name: "unsubscribe idempotent", setup: []step{create("ann", "dev")}, op: unsub("bob", "dev")},
		{name: "subscribe all", op: sub("ann", "all"), kind: apperr.Invalid,
			msg: "the all topic cannot be subscribed or unsubscribed"},
		{name: "unsubscribe all", op: unsub("ann", "all"), kind: apperr.Invalid},
		{name: "departure drops subscriptions", setup: []step{create("ann", "dev"), sub("bob", "dev"), depart("bob")},
			op: sendTopic("ann", "dev", "m", false), inboxes: map[string][]string{"bob": {}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			w := newWorld(t)
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
		})
	}
}

func TestListTopics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newWorld(t)
	for _, s := range []step{create("ann", "zeta"), create("bob", "beta"), create("ann", "gone"), closeT("ann", "gone")} {
		if err := s(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	got, err := w.svc.ListTopics(ctx, w.p["ann"], space)
	if err != nil {
		t.Fatal(err)
	}
	want := []TopicInfo{
		{Name: "all", CreatedBy: "lead", Subscribed: true},
		{Name: "beta", CreatedBy: "bob", Subscribed: false},
		{Name: "zeta", CreatedBy: "ann", Subscribed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveGate(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	err := w.svc.SendDirect(context.Background(), w.p["ann"], "nope", "bob", "x", false)
	apperrtest.WantKind(t, err, apperr.NotFound, "space nope not found")
}

func TestOnSpaceCreated(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	got, err := w.svc.ListTopics(context.Background(), w.p["bob"], space)
	if err != nil {
		t.Fatal(err)
	}
	want := []TopicInfo{{Name: "all", CreatedBy: "lead", Subscribed: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
