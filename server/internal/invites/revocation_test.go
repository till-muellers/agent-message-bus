package invites

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

func TestRevokeCancelsPendingCodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	first := mustCreate(t, e, "worker", nil)
	second := mustCreate(t, e, "worker", nil)
	other := mustCreate(t, e, "other", nil)
	if deleted, err := e.spaces.Revoke(ctx, e.lead, "demo", "worker"); err != nil || !deleted {
		t.Fatalf("revoke pending worker = %v, %v; want true", deleted, err)
	}
	for _, inv := range []Invite{first, second} {
		_, err := e.svc.Accept(ctx, inv.Code)
		apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid or expired invite code")
	}
	if _, err := e.svc.Accept(ctx, other.Code); err != nil {
		t.Fatalf("accept other: %v", err)
	}
	if deleted, err := e.spaces.Revoke(ctx, e.lead, "demo", "worker"); err != nil || deleted {
		t.Fatalf("repeated revoke = %v, %v; want false", deleted, err)
	}
}

func TestRevokeMemberCancelsPendingCodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Accept(ctx, mustCreate(t, e, "worker", nil).Code); err != nil {
		t.Fatalf("accept: %v", err)
	}
	pending := mustCreate(t, e, "worker", nil)
	if deleted, err := e.spaces.Revoke(ctx, e.lead, "demo", "worker"); err != nil || !deleted {
		t.Fatalf("revoke worker = %v, %v; want true", deleted, err)
	}
	_, err := e.svc.Accept(ctx, pending.Code)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid or expired invite code")
}
