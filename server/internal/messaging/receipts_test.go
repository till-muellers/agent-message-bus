package messaging

import (
	"context"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

func TestSendReceipts(t *testing.T) {
	t.Parallel()
	q := func(n string) inbox.Receipt { return inbox.Receipt{To: n, Delivery: inbox.Queued} }
	tests := []struct {
		name  string
		setup []step
		send  func(ctx context.Context, w *world) ([]inbox.Receipt, error)
		want  []inbox.Receipt
	}{
		{name: "direct", send: func(ctx context.Context, w *world) ([]inbox.Receipt, error) {
			return w.svc.SendDirect(ctx, w.p["ann"], space, "bob", "hi", false)
		}, want: []inbox.Receipt{q("bob")}},
		{name: "all topic: everyone but the sender, by name", send: func(ctx context.Context, w *world) ([]inbox.Receipt, error) {
			return w.svc.SendTopic(ctx, w.p["lead"], space, "all", "hi", false)
		}, want: []inbox.Receipt{q("ann"), q("bob")}},
		{name: "topic without other subscribers", setup: []step{create("ann", "solo")},
			send: func(ctx context.Context, w *world) ([]inbox.Receipt, error) {
				return w.svc.SendTopic(ctx, w.p["ann"], space, "solo", "hi", false)
			}, want: []inbox.Receipt{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			ctx := context.Background()
			for _, s := range tc.setup {
				if err := s(ctx, w); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			got, err := tc.send(ctx, w)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("receipts = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
