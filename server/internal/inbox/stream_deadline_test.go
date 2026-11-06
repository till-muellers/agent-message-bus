package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// stalledClient stores more backlog for worker than the connection buffers hold and opens
// worker's stream from a client that never reads, so the stream blocks in a frame write.
// root is the stream root; the call returns once the stream holds the slot.
func stalledClient(root context.Context, t *testing.T, w *world) {
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
	p := w.principals["memberA"]
	h := w.svc.StreamHandler(root)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(rw, r.WithContext(auth.WithPrincipal(r.Context(), p)))
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
	eventually(t, "the stream holds the slot", func() bool { return w.svc.Connected(w.agents["worker"]) })
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
	stalledClient(root, t, w)
	time.Sleep(500 * time.Millisecond) // let the writes fill the buffers and block
	cancel()
	slotFreed(t, w)
}
