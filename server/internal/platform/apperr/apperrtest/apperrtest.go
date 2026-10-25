// Package apperrtest provides assertions on apperr errors for tests.
package apperrtest

import (
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
)

// WantKind fails the test unless err is an *apperr.Error of kind; msg is checked when non-empty.
func WantKind(t *testing.T, err error, kind apperr.Kind, msg string) {
	t.Helper()
	got, ok := apperr.KindOf(err)
	if !ok || got != kind {
		t.Fatalf("err = %v (kind %q), want kind %q", err, got, kind)
	}
	if msg != "" && err.Error() != msg {
		t.Fatalf("err = %q, want %q", err.Error(), msg)
	}
}
