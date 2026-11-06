package inbox

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// frameHook calls onWrite with the number of each write the stream makes (one per frame),
// in the stream's own goroutine, so a test can act at an exact point of a drain.
type frameHook struct {
	http.ResponseWriter
	writes  int
	onWrite func(n int)
}

func (f *frameHook) Write(b []byte) (int, error) {
	f.writes++
	f.onWrite(f.writes)
	return f.ResponseWriter.Write(b)
}

func (f *frameHook) Unwrap() http.ResponseWriter { return f.ResponseWriter }

// storeBacklog stores n entries for worker in alpha in one transaction.
func storeBacklog(t *testing.T, w *world, n int) {
	t.Helper()
	backlog := make([]Entry, 0, n)
	for i := range n {
		backlog = append(backlog, w.entry("worker", "alpha", fmt.Sprintf("bulk%d", i)))
	}
	err := db.InTx(t.Context(), w.db, func(tx *sql.Tx) error {
		_, err := w.svc.insert(t.Context(), tx, backlog)
		return err
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

// startHookedStream opens the stream of the named principal with onWrite hooked into
// its frame writes.
func startHookedStream(t *testing.T, w *world, who string, onWrite func(n int)) *sseReader {
	t.Helper()
	p := w.principals[who]
	h := w.svc.StreamHandler(t.Context())
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(&frameHook{ResponseWriter: rw, onWrite: onWrite}, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}))
	t.Cleanup(srv.Close)
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return readSSE(t, resp.Body)
}

// TestStreamSurvivesSignalsDuringALongDrain is a fast reader under a sender burst: wake-up
// signals keep arriving while one drain sends page after page. The world's broker holds
// 16 signals; 8 arrive while each page is sent, so only a drain that discards the queued
// signals after each page keeps its subscription.
func TestStreamSurvivesSignalsDuringALongDrain(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	const pages, perPage = 6, 8
	storeBacklog(t, w, pages*MaxLimit)
	stream := startHookedStream(t, w, "memberA", func(n int) {
		if n%MaxLimit == MaxLimit/5 {
			publishSignals(w, perPage)
		}
	})
	stream.entries(pages*MaxLimit + 2) // + the seeded a1 and a2
	w.send(t, "worker", "alpha", "after the burst")
	if got := stream.nextEntry().Body; got != "after the burst" {
		t.Fatalf("entry after the burst = %q", got)
	}
}

// TestStreamEndsCleanlyWhenTheSubscriptionOverflowsMidDrain overflows the world's
// 16-signal buffer while the backlog streams.
func TestStreamEndsCleanlyWhenTheSubscriptionOverflowsMidDrain(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	stream := startHookedStream(t, w, "memberA", func(n int) {
		if n == 1 {
			publishSignals(w, 17)
		}
	})
	stream.entries(2)
	stream.expectClosed()
}

func publishSignals(w *world, n int) {
	for range n {
		w.pub.Publish(topic(w.agents["worker"]), nil)
	}
}

// deadlineRecorder is a recorder whose writes accept deadlines, like a real connection.
type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (deadlineRecorder) SetWriteDeadline(time.Time) error { return nil }

// recordedSSE opens an SSE writer over a recorder.
func recordedSSE(t *testing.T) sseWriter {
	t.Helper()
	sse, err := openSSE(deadlineRecorder{httptest.NewRecorder()}, time.Minute, t.Context().Done())
	if err != nil {
		t.Fatalf("openSSE: %v", err)
	}
	return sse
}

// TestDrainEndsAfterThePageWhenTheSubscriptionDropped checks that a subscription closed
// during a drain ends it after the current page with errDropped, which the stream turns
// into a clean end.
func TestDrainEndsAfterThePageWhenTheSubscriptionDropped(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	storeBacklog(t, w, 2*MaxLimit)
	sse := recordedSSE(t)
	live := make(chan []byte)
	close(live)
	lastSent, err := w.svc.drain(t.Context(), sse, w.principals["memberA"], live, 0)
	if !errors.Is(err, errDropped) {
		t.Fatalf("drain error = %v, want errDropped", err)
	}
	page, err := w.svc.Read(t.Context(), w.principals["memberA"], 0, MaxLimit)
	if err != nil || lastSent != page[len(page)-1].ID {
		t.Fatalf("drain stopped at id %d (read err %v), want the end of the first page", lastSent, err)
	}
}
