package instance

import (
	"context"
	"os"
	"strings"
	"testing"
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
