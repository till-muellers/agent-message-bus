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
			want: Config{Addr: "127.0.0.1:8080", WebAddr: "127.0.0.1:8081", DBPath: filepath.Join("X", "agent-message-bus", "agent-message-bus.db"), AdminToken: valid},
		},
		{
			name: "overrides",
			env:  map[string]string{"AMB_ADMIN_TOKEN": valid, "AMB_ADDR": ":9", "AMB_DB_PATH": "x.db"},
			want: Config{Addr: ":9", WebAddr: "127.0.0.1:8081", DBPath: "x.db", AdminToken: valid},
		},
		{
			name: "token optional",
			env:  map[string]string{"AMB_DB_PATH": "x.db"},
			want: Config{Addr: "127.0.0.1:8080", WebAddr: "127.0.0.1:8081", DBPath: "x.db"},
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
			got, err := load(lookupIn(tc.env), "linux", func() (string, error) { return "H", nil })
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

// lookupIn returns an os.LookupEnv-style lookup over env.
func lookupIn(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func TestWebAddrConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{name: "unset", env: map[string]string{}, want: "127.0.0.1:8081"},
		{name: "off", env: map[string]string{"AMB_WEB_ADDR": "off"}, want: ""},
		{name: "empty", env: map[string]string{"AMB_WEB_ADDR": ""}, want: ""},
		{name: "any host and port", env: map[string]string{"AMB_WEB_ADDR": "0.0.0.0:9000"}, want: "0.0.0.0:9000"},
		{
			name:    "same as AMB_ADDR",
			env:     map[string]string{"AMB_ADDR": "127.0.0.1:9000", "AMB_WEB_ADDR": "127.0.0.1:9000"},
			wantErr: "AMB_WEB_ADDR 127.0.0.1:9000 must differ from AMB_ADDR; set another address or off",
		},
		{
			name:    "default clashes with AMB_ADDR",
			env:     map[string]string{"AMB_ADDR": "127.0.0.1:8081"},
			wantErr: "AMB_WEB_ADDR 127.0.0.1:8081 must differ from AMB_ADDR; set another address or off",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.env["AMB_DB_PATH"] = "x.db"
			got, err := load(lookupIn(tc.env), "linux", func() (string, error) { return "H", nil })
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.WebAddr != tc.want {
				t.Fatalf("WebAddr = %q, want %q", got.WebAddr, tc.want)
			}
		})
	}
}
