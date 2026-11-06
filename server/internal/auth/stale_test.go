package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// TestStaleAdminCannotMint: a mint authenticated before the caller's credential was
// revoked or rotated, and run after it, fails unauthenticated and issues no token. A
// revoked admin cannot hand itself a fresh token.
func TestStaleAdminCannotMint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, root := bootstrapped(t)
	ops, err := s.Authenticate(ctx, mustMintAdmin(t, s, root, "ops"))
	if err != nil {
		t.Fatalf("Authenticate ops: %v", err)
	}
	if _, err := s.Revoke(ctx, s.db, ops.AgentID, Everywhere()); err != nil {
		t.Fatalf("Revoke ops: %v", err)
	}
	if err := s.SetAdminToken(strings.Repeat("n", domain.MinAdminTokenLen)); err != nil {
		t.Fatalf("SetAdminToken: %v", err)
	}
	for _, p := range []Principal{ops, root} {
		_, err := s.MintAdmin(ctx, p, "ops")
		apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
		_, err = s.MintLeader(ctx, p, "late")
		apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
	}
	admins, err := listAdmins(ctx, s.db)
	if err != nil || len(admins) != 2 || admins[1].Name != "ops" || admins[1].Tokens != 0 {
		t.Fatalf("admins = %+v, %v; want ops without tokens", admins, err)
	}
	if _, _, found, err := findAgent(ctx, s.db, "late"); found || err != nil {
		t.Fatalf("leader late created: found %v, %v", found, err)
	}
}
