package inbox

import (
	"net/http/httptest"
	"testing"
	"time"
)

// looks are the ways an agent can look at its "alpha" mailbox; each returns the bodies it saw.
var looks = map[string]func(t *testing.T, w *world, after int64) []string{
	"Read": func(t *testing.T, w *world, after int64) []string {
		return mustEntries(t, "Read")(w.svc.Read(t.Context(), w.principals["memberA"], after, 0))
	},
	"ReadMarking": func(t *testing.T, w *world, after int64) []string {
		return mustEntries(t, "ReadMarking")(w.svc.ReadMarking(t.Context(), w.principals["memberA"], after, 0))
	},
	"Wait": func(t *testing.T, w *world, after int64) []string {
		q := WaitQuery{After: after, Timeout: time.Second}
		return mustEntries(t, "Wait")(w.svc.Wait(t.Context(), w.principals["memberA"], q))
	},
	"stream": func(t *testing.T, w *world, after int64) []string {
		sse, err := openSSE(httptest.NewRecorder())
		if err != nil {
			t.Fatalf("openSSE: %v", err)
		}
		if _, err := w.svc.drain(t.Context(), sse, w.principals["memberA"], after); err != nil {
			t.Fatalf("drain: %v", err)
		}
		return nil
	},
}

func mustEntries(t *testing.T, what string) func([]Entry, error) []string {
	return func(es []Entry, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return bodies(es)
	}
}

// TestWebReadDoesNotMark: Read (the web GUI's listing) leaves the edge unnoticed, while
// the marking paths (bus_inbox_read, bus_wait, the SSE stream) mark the edge they return.
func TestWebReadDoesNotMark(t *testing.T) {
	t.Parallel()
	for name, look := range looks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := emptyWorld(t)
			w.sendEach(t, "worker", "alpha", "a1")
			look(t, w, 0)
			if due := w.take(t, "memberA") != nil; due != (name == "Read") {
				t.Fatalf("notice due after %s = %v", name, due)
			}
		})
	}
}

// TestMarkingOnlyReturnedEdges: a look that does not return the edge leaves it unnoticed.
func TestMarkingOnlyReturnedEdges(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ReadMarking", "Wait", "stream"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := emptyWorld(t)
			w.sendEach(t, "worker", "alpha", "a1", "a2")
			looks[name](t, w, w.entries["a1"].ID)
			if w.take(t, "memberA") == nil {
				t.Fatalf("%s after the edge marked it noticed", name)
			}
		})
	}
}
