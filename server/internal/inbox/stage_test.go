package inbox

import (
	"errors"
	"slices"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

func TestInTx(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name       string
		entries    []string // bodies delivered to worker in alpha
		fnErr      error
		wantLeft   []string
		wantSignal bool
	}{
		{name: "inserts and wakes subscriber", entries: []string{"n1", "n2"}, wantLeft: []string{"a1", "a2", "n1", "n2"}, wantSignal: true},
		{name: "fn error rolls back", entries: []string{"n1"}, fnErr: boom, wantLeft: []string{"a1", "a2"}},
		{name: "nil entries", wantLeft: []string{"a1", "a2"}},
		{name: "empty entries", entries: []string{}, wantLeft: []string{"a1", "a2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			live, cancel := w.pub.Subscribe(topic(w.agents["worker"]))
			t.Cleanup(cancel)
			_, err := InTx(t.Context(), w.db, w.svc, func(tx db.DBTX) ([]Entry, error) {
				if _, err := tx.ExecContext(t.Context(), `UPDATE spaces SET name = name`); err != nil {
					return nil, err
				}
				var es []Entry
				if tc.entries != nil {
					es = []Entry{}
				}
				for _, b := range tc.entries {
					es = append(es, w.entry("worker", "alpha", b))
				}
				return es, tc.fnErr
			})
			if !errors.Is(err, tc.fnErr) || (tc.fnErr == nil && err != nil) {
				t.Fatalf("InTx err = %v, want %v", err, tc.fnErr)
			}
			if got := w.read(t, "memberA"); !slices.Equal(got, tc.wantLeft) {
				t.Fatalf("inbox = %v, want %v", got, tc.wantLeft)
			}
			if got := len(live) > 0; got != tc.wantSignal {
				t.Fatalf("signalled = %v, want %v", got, tc.wantSignal)
			}
		})
	}
}

// TestStagePublishesOnlyOnPublish: Stage stores in the caller's transaction and wakes
// nobody; Publish, called after commit, wakes the recipient and returns its receipt.
func TestStagePublishesOnlyOnPublish(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	live, cancel := w.pub.Subscribe(topic(w.agents["worker"]))
	t.Cleanup(cancel)
	tx, err := w.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	st, err := w.svc.Stage(t.Context(), tx, []Entry{w.entry("worker", "alpha", "n1")})
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(live) != 0 {
		t.Fatal("Stage woke the recipient before Publish")
	}
	rs := w.svc.Publish(st)
	if want := []Receipt{{To: "worker", Delivery: Queued}}; !slices.Equal(rs, want) || len(live) != 1 {
		t.Fatalf("Publish = %+v, signals %d; want %+v and one signal", rs, len(live), want)
	}
}

// TestStageRolledBack: entries staged in a transaction that rolls back are gone.
func TestStageRolledBack(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	tx, err := w.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := w.svc.Stage(t.Context(), tx, []Entry{w.entry("worker", "alpha", "n1")}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got, want := w.read(t, "memberA"), []string{"a1", "a2"}; !slices.Equal(got, want) {
		t.Fatalf("inbox = %v, want %v", got, want)
	}
}
