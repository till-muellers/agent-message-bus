package inbox

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// watchedWriter records when its pending write or flush started (unix nanoseconds, 0
// while none is pending), so a test can tell that the stream is blocked writing.
type watchedWriter struct {
	http.ResponseWriter
	pendingSince *atomic.Int64
}

func (w watchedWriter) Write(b []byte) (int, error) {
	w.pendingSince.Store(time.Now().UnixNano())
	defer w.pendingSince.Store(0)
	return w.ResponseWriter.Write(b)
}

// FlushError is what http.ResponseController.Flush calls.
func (w watchedWriter) FlushError() error {
	w.pendingSince.Store(time.Now().UnixNano())
	defer w.pendingSince.Store(0)
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the connection for its write deadline.
func (w watchedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// stalledClient stores more backlog for worker than the connection buffers hold and opens
// worker's stream from a client that never reads, so the stream blocks in a frame write.
// root is the stream root; the call returns once the stream holds the slot, with the start
// of the stream's pending write (see watchedWriter).
func stalledClient(root context.Context, t *testing.T, w *world) *atomic.Int64 {
	t.Helper()
	storeBigBacklog(t, w)
	p := w.principals["memberA"]
	h := w.svc.StreamHandler(root)
	pending := &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(watchedWriter{rw, pending}, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}))
	t.Cleanup(srv.Close)
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dialing: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.(*net.TCPConn).SetReadBuffer(4096); err != nil {
		t.Fatalf("shrinking the read buffer: %v", err)
	}
	if _, err := fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
		t.Fatalf("requesting the stream: %v", err)
	}
	eventually(t, "the stream holds the slot", func() bool { return w.svc.presence.connected(w.agents["worker"]) })
	return pending
}

// storeBigBacklog stores 600 entries of 60 KB each for worker in alpha.
func storeBigBacklog(t *testing.T, w *world) {
	t.Helper()
	body := strings.Repeat("y", 60000)
	err := db.InTx(t.Context(), w.db, func(tx *sql.Tx) error {
		backlog := make([]Entry, 0, 600)
		for i := range 600 {
			backlog = append(backlog, w.entry("worker", "alpha", fmt.Sprintf("%d %s", i, body)))
		}
		_, err := w.svc.insert(t.Context(), tx, backlog)
		return err
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

// writeBlocked waits until one write of the stream has been pending for 100 ms.
func writeBlocked(t *testing.T, pending *atomic.Int64) {
	t.Helper()
	eventually(t, "a frame write blocks", func() bool {
		since := pending.Load()
		return since != 0 && time.Since(time.Unix(0, since)) > 100*time.Millisecond
	})
}

func TestStreamDropsAClientThatStopsReading(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.frameTimeout = 200 * time.Millisecond
	stalledClient(t.Context(), t, w)
	slotFreed(t, w)
}

func TestShutdownInterruptsAStreamBlockedInAWrite(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.frameTimeout = time.Hour
	root, cancel := context.WithCancel(t.Context())
	writeBlocked(t, stalledClient(root, t, w))
	start := time.Now()
	cancel()
	slotFreed(t, w)
	if took := time.Since(start); took > time.Second {
		t.Fatalf("the slot was freed %v after shutdown, want within a second", took)
	}
}

// Not parallel: it captures the default logger.
func TestStreamEndLogsAClientThatStopsReadingButNotAShutdown(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	stalled := newWorld(t)
	stalled.svc.frameTimeout = 200 * time.Millisecond
	stalledClient(t.Context(), t, stalled)
	slotFreed(t, stalled)
	if !strings.Contains(buf.String(), "inbox stream dropped: client not reading") {
		t.Fatalf("log after a stalled client = %q, want the client-not-reading line", buf.String())
	}

	buf.Reset()
	shut := newWorld(t)
	shut.svc.frameTimeout = time.Hour
	root, cancel := context.WithCancel(t.Context())
	writeBlocked(t, stalledClient(root, t, shut))
	cancel()
	slotFreed(t, shut)
	if buf.Len() != 0 {
		t.Fatalf("log after a shutdown = %q, want nothing", buf.String())
	}
}
