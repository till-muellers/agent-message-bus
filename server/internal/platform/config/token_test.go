package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAdminToken(t *testing.T) {
	t.Parallel()
	env := strings.Repeat("e", 32)
	tests := []struct {
		name, override, file, want, wantErr string
	}{
		{name: "env wins", override: env, file: strings.Repeat("f", 32), want: env},
		{name: "file", file: strings.Repeat("f", 32) + "\n", want: strings.Repeat("f", 32)},
		{name: "short file", file: "short\n", wantErr: "at least 32 characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := Config{DBPath: filepath.Join(t.TempDir(), "x.db"), AdminToken: tc.override}
			if err := os.WriteFile(c.TokenPath(), []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := c.ResolveAdminToken()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestResolveAdminTokenGenerates(t *testing.T) {
	t.Parallel()
	c := Config{DBPath: filepath.Join(t.TempDir(), "sub", "x.db")}
	tok, err := c.ResolveAdminToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "amb_admin_") || len(tok) != len("amb_admin_")+43 {
		t.Fatalf("token %q", tok)
	}
	b, err := os.ReadFile(c.TokenPath())
	if err != nil || string(b) != tok+"\n" {
		t.Fatalf("file %q, %v", b, err)
	}
	again, err := c.ResolveAdminToken()
	if err != nil || again != tok {
		t.Fatalf("second resolve %q, %v", again, err)
	}
}
