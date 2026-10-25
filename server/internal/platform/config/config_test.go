package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()
	valid := strings.Repeat("a", 32)
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{
			name: "defaults",
			env:  map[string]string{"AMB_ADMIN_TOKEN": valid},
			want: Config{Addr: ":8080", DBPath: "/data/agent-message-bus.db", AdminToken: valid},
		},
		{
			name: "overrides",
			env:  map[string]string{"AMB_ADMIN_TOKEN": valid, "AMB_ADDR": ":9", "AMB_DB_PATH": "x.db"},
			want: Config{Addr: ":9", DBPath: "x.db", AdminToken: valid},
		},
		{name: "missing token", env: map[string]string{}, wantErr: "AMB_ADMIN_TOKEN is required"},
		{
			name:    "short token",
			env:     map[string]string{"AMB_ADMIN_TOKEN": "short"},
			wantErr: "AMB_ADMIN_TOKEN must be at least 32 characters",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Load(func(k string) string { return tc.env[k] })
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
