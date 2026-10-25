package config

import (
	"path/filepath"
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
			env:  map[string]string{"AMB_ADMIN_TOKEN": valid, "XDG_DATA_HOME": "X"},
			want: Config{Addr: "127.0.0.1:8080", DBPath: filepath.Join("X", "agent-message-bus", "agent-message-bus.db"), AdminToken: valid},
		},
		{
			name: "overrides",
			env:  map[string]string{"AMB_ADMIN_TOKEN": valid, "AMB_ADDR": ":9", "AMB_DB_PATH": "x.db"},
			want: Config{Addr: ":9", DBPath: "x.db", AdminToken: valid},
		},
		{
			name: "token optional",
			env:  map[string]string{"AMB_DB_PATH": "x.db"},
			want: Config{Addr: "127.0.0.1:8080", DBPath: "x.db"},
		},
		{
			name:    "short token",
			env:     map[string]string{"AMB_ADMIN_TOKEN": "short"},
			wantErr: "AMB_ADMIN_TOKEN must be at least 32 characters",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := load(func(k string) string { return tc.env[k] }, "linux", func() (string, error) { return "H", nil })
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
