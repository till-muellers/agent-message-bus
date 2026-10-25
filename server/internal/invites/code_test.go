package invites

import (
	"regexp"
	"testing"
)

func TestNewCodeFormat(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^[2-9a-km-np-z]{4}-[2-9a-km-np-z]{4}$`)
	for range 50 {
		code, err := newCode()
		if err != nil {
			t.Fatalf("newCode: %v", err)
		}
		if !re.MatchString(code) {
			t.Fatalf("code %q does not match %s", code, re)
		}
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := map[string]string{"K7M2-X9PQ": "k7m2x9pq", "k7m2x9pq": "k7m2x9pq", " k7m2 x9pq ": "k7m2x9pq", "": ""}
	for in, want := range tests {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
