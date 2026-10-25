package domain

import (
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

func TestValidName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		v  string
		ok bool
	}{
		{"a", true}, {"a-b_9", true}, {strings.Repeat("a", 63), true},
		{"", false}, {"-a", false}, {"A", false}, {"a b", false}, {strings.Repeat("a", 64), false},
	}
	for _, tc := range tests {
		err := ValidName("space", tc.v)
		if tc.ok && err != nil {
			t.Fatalf("ValidName(%q) = %v", tc.v, err)
		}
		if !tc.ok {
			apperrtest.WantKind(t, err, "invalid", "space must match ^[a-z0-9][a-z0-9_-]{0,62}$")
		}
	}
}

func TestValidBodyAndTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"body empty", ValidBody(""), "body must be 1..65536 bytes"},
		{"body too long", ValidBody(strings.Repeat("x", MaxBodyBytes+1)), "body must be 1..65536 bytes"},
		{"body max", ValidBody(strings.Repeat("x", MaxBodyBytes)), ""},
		{"title empty", ValidTitle(""), "title must be 1..200 characters"},
		{"title too long", ValidTitle(strings.Repeat("é", MaxTitleChars+1)), "title must be 1..200 characters"},
		{"title max multibyte", ValidTitle(strings.Repeat("é", MaxTitleChars)), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.msg == "" {
				if tc.err != nil {
					t.Fatalf("unexpected err: %v", tc.err)
				}
				return
			}
			apperrtest.WantKind(t, tc.err, "invalid", tc.msg)
		})
	}
}
