package inbox

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"
)

func TestStreamSlots(t *testing.T) {
	t.Parallel()
	type step struct {
		op      string // "acquire" or "release"
		agentID int64
		want    bool // acquire result
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"first acquire wins", []step{{"acquire", 1, true}}},
		{"second acquire of same agent fails", []step{{"acquire", 1, true}, {"acquire", 1, false}}},
		{"release frees the slot", []step{{"acquire", 1, true}, {"release", 1, false}, {"acquire", 1, true}}},
		{"agents have their own slots", []step{{"acquire", 1, true}, {"acquire", 2, true}}},
		{"releasing one agent keeps another held", []step{
			{"acquire", 1, true}, {"acquire", 2, true}, {"release", 1, false}, {"acquire", 2, false},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newPresence()
			for i, st := range tc.steps {
				if st.op == "release" {
					s.release(st.agentID)
					continue
				}
				if got := s.acquire(st.agentID); got != st.want {
					t.Fatalf("step %d: acquire(%d) = %v, want %v", i, st.agentID, got, st.want)
				}
			}
		})
	}
}

func TestPresenceConnected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		streams []int64
		waits   []int64 // enterWait per id
		leaves  []int64 // leaveWait per id
		want    bool    // connected(1)
	}{
		{name: "nothing", want: false},
		{name: "stream slot", streams: []int64{1}, want: true},
		{name: "other agent's stream", streams: []int64{2}, want: false},
		{name: "one waiter", waits: []int64{1}, want: true},
		{name: "waiter left", waits: []int64{1}, leaves: []int64{1}, want: false},
		{name: "two waiters one left", waits: []int64{1, 1}, leaves: []int64{1}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newPresence()
			for _, id := range tc.streams {
				p.acquire(id)
			}
			for _, id := range tc.waits {
				p.enterWait(id)
			}
			for _, id := range tc.leaves {
				p.leaveWait(id)
			}
			if got := p.connected(1); got != tc.want {
				t.Fatalf("connected(1) = %v, want %v", got, tc.want)
			}
		})
	}
}

// deliverTo delivers one entry per named agent and returns the receipts.
func (w *world) deliverTo(t *testing.T, during func(), agents ...string) []Receipt {
	t.Helper()
	rs, err := w.svc.DeliverReceipts(t.Context(), func(*sql.Tx) ([]Entry, error) {
		es := make([]Entry, 0, len(agents))
		for _, a := range agents {
			es = append(es, w.entry(a, "alpha", "r-"+a))
		}
		if during != nil {
			during()
		}
		return es, nil
	})
	if err != nil {
		t.Fatalf("DeliverReceipts: %v", err)
	}
	return rs
}

func TestReceipts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		streams []string
		to      []string
		want    []Receipt
	}{
		{name: "none", to: nil, want: []Receipt{}},
		{name: "queued", to: []string{"worker"}, want: []Receipt{{"worker", Queued}}},
		{name: "live", streams: []string{"worker"}, to: []string{"worker"}, want: []Receipt{{"worker", Live}}},
		{name: "sorted by name, mixed", streams: []string{"lead"}, to: []string{"worker", "lead"},
			want: []Receipt{{"lead", Live}, {"worker", Queued}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			for _, a := range tc.streams {
				w.svc.presence.acquire(w.agents[a])
			}
			if got := w.deliverTo(t, nil, tc.to...); !slices.Equal(got, tc.want) {
				t.Fatalf("receipts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReceiptsWaiterIsLive(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx, cancel := context.WithCancel(t.Context())
	ch := startWait(ctx, w.svc, w.principals["memberA"], WaitQuery{After: 1 << 40, Timeout: longWait})
	deadline := time.Now().Add(waitTimeout)
	for !w.svc.Connected(w.agents["worker"]) {
		if time.Now().After(deadline) {
			t.Fatal("waiting agent never counted as connected")
		}
		time.Sleep(time.Millisecond)
	}
	got := w.deliverTo(t, nil, "worker")
	if want := []Receipt{{"worker", Live}}; !slices.Equal(got, want) {
		t.Fatalf("receipts = %v, want %v", got, want)
	}
	cancel()
	if got := result(t, ch); len(got) != 0 {
		t.Fatalf("Wait = %v, want []", got)
	}
	if w.svc.Connected(w.agents["worker"]) {
		t.Fatal("agent still connected after its Wait returned")
	}
}

func TestReceiptAtCommit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		before bool // slot held before the transaction
		during func(p *presence, id int64)
		want   Delivery
	}{
		{name: "stream opened before commit", during: func(p *presence, id int64) { p.acquire(id) }, want: Live},
		{name: "stream ended before commit", before: true, during: func(p *presence, id int64) { p.release(id) }, want: Queued},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			id := w.agents["worker"]
			if tc.before {
				w.svc.presence.acquire(id)
			}
			got := w.deliverTo(t, func() { tc.during(w.svc.presence, id) }, "worker")
			if want := []Receipt{{"worker", tc.want}}; !slices.Equal(got, want) {
				t.Fatalf("receipts = %v, want %v", got, want)
			}
		})
	}
}
