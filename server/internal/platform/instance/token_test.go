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
		running bool
		wantErr string
	}{
		{name: "env override", env: testToken, wantErr: "AMB_ADMIN_TOKEN is set in the environment; rotate it there"},
		{name: "not running"},
		{name: "running", running: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := newManager(t, tc.env)
			f := newFakeBus(t, m.dir(), true)
			if tc.running {
				f.publish(t, 1)
			}
			old, _ := m.Config.ResolveAdminToken()
			err := m.RotateToken(context.Background())
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v", err)
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
			if tc.running {
				want = []string{`/v1/admin/admin-token Bearer ` + old + ` {"token":"` + next + `"}`}
			}
			if got := f.recorded(); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("calls %q, want %q", got, want)
			}
		})
	}
}
