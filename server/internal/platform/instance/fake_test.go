package instance

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand/v2"
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
	srv      *httptest.Server
	healthy  atomic.Bool
	failures atomic.Int32  // /healthz probes to fail before answering
	obey     bool          // whether shutdown actually stops it
	takeover bool          // on an ignored shutdown, another instance's state replaces its own
	token    string        // the admin token it accepts, swapped by POST /v1/admin/admin-token; empty: any
	swapLag  time.Duration // between a swap's token check and the swap, like a server under load
	dir      string
	mu       sync.Mutex
	calls    []string // "path auth body"
	lock     *Lock
	id       string
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
		if !f.healthy.Load() || f.failures.Add(-1) >= 0 {
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
	authorized := f.token == "" || r.Header.Get("Authorization") == "Bearer "+f.token
	f.mu.Unlock()
	if !authorized {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"invalid token"}}`))
		return
	}
	switch r.URL.Path {
	case "/v1/admin/shutdown":
		f.shutDown()
	case "/v1/admin/admin-token":
		f.swapToken(b)
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// swapToken installs the token of body after swapLag: the check and the swap are not one step,
// and the answers come back in no fixed order.
func (f *fakeBus) swapToken(body []byte) {
	var in struct{ Token string }
	if err := json.Unmarshal(body, &in); err != nil {
		return
	}
	time.Sleep(f.swapLag)
	f.mu.Lock()
	if f.token != "" {
		f.token = in.Token
	}
	f.mu.Unlock()
	if f.swapLag > 0 {
		time.Sleep(rand.N(f.swapLag)) // the answer reaches the caller in no fixed order
	}
}

func (f *fakeBus) liveToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.token
}

func (f *fakeBus) shutDown() {
	switch {
	case f.obey:
		f.exit()
	case f.takeover:
		_ = writeState(f.dir, State{PID: 8, URL: f.srv.URL, InstanceID: "successor"})
	}
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
