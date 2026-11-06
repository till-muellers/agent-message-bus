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

// fakeBus imitates a serving instance: it answers /healthz with its instance id and, once it
// holds dir's lock, counts as that data dir's running instance.
type fakeBus struct {
	srv     *httptest.Server
	healthy atomic.Bool
	obey    bool // whether shutdown actually stops it
	dir     string
	mu      sync.Mutex
	calls   []string // "path auth body"
	lock    *Lock
	id      string
}

func newFakeBus(t *testing.T, dir string, obey bool) *fakeBus {
	t.Helper()
	f := &fakeBus{obey: obey, dir: dir, id: "foreign-instance"}
	f.healthy.Store(true)
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	t.Cleanup(f.exit)
	return f
}

// newRunningBus returns a fake that holds dir's lock and has published pid.
func newRunningBus(t *testing.T, dir string, obey bool, pid int) *fakeBus {
	t.Helper()
	f := newFakeBus(t, dir, obey)
	f.adopt(mustClaim(t, dir))
	f.publish(t, pid)
	return f
}

func (f *fakeBus) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if !f.healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.mu.Lock()
		w.Header().Set(IDHeader, f.id)
		f.mu.Unlock()
		_, _ = w.Write([]byte("ok"))
		return
	}
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, r.URL.Path+" "+r.Header.Get("Authorization")+" "+string(b))
	f.mu.Unlock()
	if r.URL.Path == "/v1/admin/shutdown" && f.obey {
		f.exit()
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// adopt makes l the fake's lock: it serves l's instance id.
func (f *fakeBus) adopt(l *Lock) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lock, f.id = l, l.ID()
}

// exit stops serving like a process that shuts down: state file removed, lock released.
func (f *fakeBus) exit() {
	f.mu.Lock()
	held := f.lock != nil
	f.mu.Unlock()
	if held {
		_ = removeState(f.dir)
	}
	f.crash()
}

// crash ends the fake like a killed process: the OS releases its lock, its state file stays.
func (f *fakeBus) crash() {
	f.healthy.Store(false)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lock != nil {
		_ = f.lock.Release()
		f.lock = nil
	}
}

func (f *fakeBus) publish(t *testing.T, pid int) {
	t.Helper()
	f.mu.Lock()
	st := State{PID: pid, URL: f.srv.URL, StartedAt: "2026-01-01T00:00:00Z", InstanceID: f.id}
	f.mu.Unlock()
	if err := writeState(f.dir, st); err != nil {
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
