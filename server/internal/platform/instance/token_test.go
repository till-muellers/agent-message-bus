package instance

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdminToken(t *testing.T) {
	t.Parallel()
	m, out := newManager(t, "")
	if err := m.AdminToken(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(m.Config.TokenPath())
	if out.String() != string(b) || !strings.HasPrefix(out.String(), "amb_admin_") {
		t.Fatalf("printed %q, file %q", out, b)
	}
}

func TestRotateToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		env     string
		kind    instanceKind
		wantErr string
	}{
		{name: "env override", env: testToken, wantErr: "AMB_ADMIN_TOKEN is set in the environment; rotate it there"},
		{name: "not running"},
		{name: "running", kind: serving},
		{name: "running, pending", kind: pending, wantErr: "agent-message-bus is running (state pending) but did not answer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, tc.env)
			f := setUp(t, m, tc.kind, true)
			old, _ := m.Config.ResolveAdminToken()
			err := m.RotateToken(context.Background())
			if tc.wantErr != "" {
				if b, _ := os.ReadFile(m.Config.TokenPath()); err == nil || !strings.Contains(err.Error(), tc.wantErr) ||
					(tc.env == "" && string(b) != old+"\n") {
					t.Fatalf("err = %v, token file %q", err, b)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			next := strings.TrimSpace(out.String())
			b, _ := os.ReadFile(m.Config.TokenPath())
			if next == old || string(b) != next+"\n" {
				t.Fatalf("new %q old %q file %q", next, old, b)
			}
			want := []string{}
			if tc.kind == serving {
				want = []string{`/v1/admin/admin-token Bearer ` + old + ` {"token":"` + next + `"}`}
			}
			if got := f.recorded(); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("calls %q, want %q", got, want)
			}
		})
	}
}

// TestRotateTokenSerializes: overlapping rotate-token runs on one data dir (an operator
// script, two consoles) each swap and save in turn, so the saved token is the live one.
func TestRotateTokenSerializes(t *testing.T) {
	t.Parallel()
	const runs = 5
	m, _ := newManager(t, "")
	m.Timeout = 5 * time.Second
	f := setUp(t, m, serving, true)
	f.swapLag = 20 * time.Millisecond
	f.token, _ = m.Config.ResolveAdminToken()
	errs := make(chan error, runs)
	var wg sync.WaitGroup
	for range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run := m
			run.Out = &bytes.Buffer{}
			errs <- run.RotateToken(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(m.Config.TokenPath()); string(b) != f.liveToken()+"\n" {
		t.Fatalf("token file %q, live token %q", b, f.liveToken())
	}
}

// TestRotateTokenGivesUpOnStuckRun: a run that never ends does not hang the next one, which
// leaves the token alone.
func TestRotateTokenGivesUpOnStuckRun(t *testing.T) {
	t.Parallel()
	m, _ := newManager(t, "")
	if err := os.MkdirAll(m.dir(), 0o750); err != nil {
		t.Fatal(err)
	}
	held, err := lockFile(m.rotateLockPath(), exclusive)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	err = m.RotateToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "another rotate-token still runs") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(m.Config.TokenPath()); !os.IsNotExist(err) {
		t.Fatalf("token file after a refused rotation: %v", err)
	}
}
