package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultDataDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, goos, home, want string
		env                    map[string]string
		wantErr                bool
	}{
		{name: "windows", goos: "windows", env: map[string]string{"LOCALAPPDATA": "L"}, want: filepath.Join("L", "agent-message-bus")},
		{name: "windows no localappdata", goos: "windows", home: "H", wantErr: true},
		{name: "darwin", goos: "darwin", home: "H", want: filepath.Join("H", "Library", "Application Support", "agent-message-bus")},
		{name: "darwin no home", goos: "darwin", wantErr: true},
		{name: "linux xdg", goos: "linux", home: "H", env: map[string]string{"XDG_DATA_HOME": "X"}, want: filepath.Join("X", "agent-message-bus")},
		{name: "linux fallback", goos: "linux", home: "H", want: filepath.Join("H", ".local", "share", "agent-message-bus")},
		{name: "freebsd fallback", goos: "freebsd", home: "H", want: filepath.Join("H", ".local", "share", "agent-message-bus")},
		{name: "linux nothing", goos: "linux", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := defaultDataDir(tc.goos, func(k string) string { return tc.env[k] }, tc.home)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got %q, %v; want %q, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestLoadDefaultDBPath(t *testing.T) {
	t.Parallel()
	env := map[string]string{"AMB_ADMIN_TOKEN": strings.Repeat("a", 32), "XDG_DATA_HOME": "X"}
	c, err := load(func(k string) string { return env[k] }, "linux", func() (string, error) { return "H", nil })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("X", "agent-message-bus", "agent-message-bus.db")
	if c.DBPath != want || c.DataDir() != filepath.Join("X", "agent-message-bus") {
		t.Fatalf("DBPath %q DataDir %q", c.DBPath, c.DataDir())
	}
}
