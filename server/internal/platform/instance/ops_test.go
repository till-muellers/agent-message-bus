package instance

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		live, asJSON bool
		want         string
	}{
		{name: "text running", live: true, want: "running (pid 7) at http://"},
		{name: "text stopped", want: "not running\n"},
		{name: "json running", live: true, asJSON: true, want: `{"running":true,"pid":7,"url":"http://`},
		{name: "json stopped", asJSON: true, want: `{"running":false}` + "\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, "")
			if tc.live {
				newFakeBus(t, m.dir(), true).publish(t, 7)
			}
			if err := m.Status(context.Background(), tc.asJSON); err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
		})
	}
}

func TestStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		running, spawns bool
		wantErr, want   string
	}{
		{name: "already running", running: true, want: "agent-message-bus already running (pid 5) at http://"},
		{name: "started", spawns: true, want: "agent-message-bus started (pid 9) at http://"},
		{name: "timeout", wantErr: "did not become healthy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, "")
			f := newFakeBus(t, m.dir(), true)
			if tc.running {
				f.publish(t, 5)
			}
			m.Spawn = func(logPath string) (int, error) {
				_ = os.WriteFile(logPath, []byte("boom line\n"), 0o600)
				if tc.spawns {
					f.publish(t, 9)
				}
				return 9, nil
			}
			err := m.Start(context.Background())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "boom line") {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		running, obey, wantKill bool
		want                    string
	}{
		{name: "not running", want: "agent-message-bus is not running\n"},
		{name: "graceful", running: true, obey: true, want: "agent-message-bus stopped (pid 3)\n"},
		{name: "kill fallback", running: true, wantKill: true, want: "agent-message-bus stopped (pid 3)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, testToken)
			f := newFakeBus(t, m.dir(), tc.obey)
			if tc.running {
				f.publish(t, 3)
			}
			killed := 0
			m.Kill = func(pid int) error { killed = pid; return nil }
			if err := m.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			wantOut(t, out, tc.want)
			if (killed == 3) != tc.wantKill {
				t.Fatalf("killed = %d", killed)
			}
			if _, err := os.Stat(StatePath(m.dir())); !os.IsNotExist(err) {
				t.Fatalf("state file remains: %v", err)
			}
			if tc.running && f.recorded()[0] != "/v1/admin/shutdown Bearer "+testToken+" " {
				t.Fatalf("calls %q", f.recorded())
			}
		})
	}
}
