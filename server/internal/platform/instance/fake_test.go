package instance

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/config"
)

const testToken = "test-admin-token-0123456789-0123456789"

// fakeBus imitates a serving instance.
type fakeBus struct {
	srv      *httptest.Server
	healthy  atomic.Bool
	obey     bool // whether shutdown actually stops it
	dir      string
	mu       sync.Mutex
	calls    []string // "path auth body"
}

func newFakeBus(t *testing.T, dir string, obey bool) *fakeBus {
	t.Helper()
	f := &fakeBus{obey: obey, dir: dir}
	f.healthy.Store(true)
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBus) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if !f.healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
		return
	}
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, r.URL.Path+" "+r.Header.Get("Authorization")+" "+string(b))
	f.mu.Unlock()
	if r.URL.Path == "/v1/admin/shutdown" && f.obey {
		f.healthy.Store(false)
		_ = RemoveState(f.dir)
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (f *fakeBus) publish(t *testing.T, pid int) {
	t.Helper()
	if err := WriteState(f.dir, State{PID: pid, URL: f.srv.URL, StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeBus) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// newManager returns a Manager on a temp data dir with output captured in the buffer.
func newManager(t *testing.T, envToken string) (Manager, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "bus.db"), AdminToken: envToken}
	m := Manager{
		Config: cfg, Client: http.DefaultClient, Out: out, Timeout: 500 * time.Millisecond, Poll: 10 * time.Millisecond,
		Spawn: func(string) (int, error) { t.Fatal("unexpected spawn"); return 0, nil },
		Kill:  func(int) error { t.Fatal("unexpected kill"); return nil },
	}
	return m, out
}

func wantOut(t *testing.T, out *bytes.Buffer, want string) {
	t.Helper()
	if got := out.String(); !strings.Contains(got, want) {
		t.Fatalf("output %q, want %q", got, want)
	}
}
