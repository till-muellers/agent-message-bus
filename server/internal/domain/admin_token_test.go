package domain

import (
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

func TestAdminToken(t *testing.T) {
	t.Parallel()
	ok := strings.Repeat("a", MinAdminTokenLen)
	tests := []struct{ name, in, want string }{
		{"minimum", ok, ok},
		{"maximum", strings.Repeat("a", MaxAdminTokenLen), strings.Repeat("a", MaxAdminTokenLen)},
		{"generated shape", "amb_admin_" + strings.Repeat("Zz-_9", 8) + "abc", "amb_admin_" + strings.Repeat("Zz-_9", 8) + "abc"},
		{"surrounding whitespace trimmed", " \t" + ok + "\r\n", ok},
		{"too short", "short", ""},
		{"blank", strings.Repeat(" ", 40), ""},
		{"too long", strings.Repeat("a", MaxAdminTokenLen+1), ""},
		{"inner space", ok[:16] + " " + ok[16:], ""},
		{"control character", ok + "\x01", ""},
		{"non-ASCII", ok + "é", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := AdminToken(tc.in)
			if tc.want == "" {
				apperrtest.WantKind(t, err, "invalid", "admin token must be 32..512 visible ASCII characters")
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("AdminToken(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}
}
