package auth

import (
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

func TestValidName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		v    string
		ok   bool
	}{
		{"simple", "lead", true},
		{"digits and separators", "0a_b-c", true},
		{"max length", strings.Repeat("a", 63), true},
		{"too long", strings.Repeat("a", 64), false},
		{"empty", "", false},
		{"leading dash", "-a", false},
		{"uppercase", "Lead", false},
		{"space", "a b", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidName("name", tc.v)
			if tc.ok {
				if err != nil {
					t.Fatalf("ValidName(%q) = %v", tc.v, err)
				}
				return
			}
			if k, _ := apperr.KindOf(err); k != apperr.Invalid {
				t.Fatalf("kind = %q, want invalid", k)
			}
			if want := "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"; err.Error() != want {
				t.Errorf("msg = %q, want %q", err.Error(), want)
			}
		})
	}
}
