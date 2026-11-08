package guide_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/till-muellers/agent-message-bus/server/internal/guide"
)

func TestGuideBudget(t *testing.T) {
	n := utf8.RuneCountInString(guide.Text)
	if n == 0 || n > 4000 {
		t.Fatalf("guide has %d characters, want 1..4000", n)
	}
}

func TestGuideErrorCodes(t *testing.T) {
	for _, code := range []string{"invalid", "forbidden", "not_found", "conflict", "unauthenticated"} {
		if !strings.Contains(guide.Text, "`"+code+"`: ") {
			t.Fatalf("guide does not say what to do about %s", code)
		}
	}
}

func TestGuideNeverSaysSkill(t *testing.T) {
	if strings.Contains(strings.ToLower(guide.Text), "skill") {
		t.Fatal(`guide contains "skill"`)
	}
}
