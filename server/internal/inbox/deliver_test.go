package inbox

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
)

func TestDeliver(t *testing.T) {
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
			err := w.svc.Deliver(t.Context(), func(tx *sql.Tx) ([]Entry, error) {
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
				t.Fatalf("Deliver err = %v, want %v", err, tc.fnErr)
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
