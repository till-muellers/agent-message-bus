package invites

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// TestPendingInviteDoesNotKeepSpace follows no-empty-spaces.feature "A pending invite does
// not keep a space alive".
func TestPendingInviteDoesNotKeepSpace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	ops := admin(t, e)
	if err := e.spaces.Create(ctx, ops, "gamma"); err != nil {
		t.Fatalf("creating gamma: %v", err)
	}
	if _, err := e.svc.Create(ctx, ops, "gamma", "late", nil); err != nil {
		t.Fatalf("inviting late: %v", err)
	}
	if err := e.spaces.Leave(ctx, ops, "gamma"); err != nil {
		t.Fatalf("admin leaves: %v", err)
	}
	_, err := e.spaces.Participants(ctx, ops, "gamma")
	apperrtest.WantKind(t, err, apperr.NotFound, "space gamma not found")
	_, err = e.svc.ListPending(ctx, ops, "gamma")
	apperrtest.WantKind(t, err, apperr.NotFound, "space gamma not found")
}
