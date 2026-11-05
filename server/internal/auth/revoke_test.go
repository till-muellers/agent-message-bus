package auth

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func TestRevokeScopes(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	leadTok, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
	dbtest.SeedSpace(t, db, 200, "other", lead.AgentID)
	memberID, memberTok := mustAdmit(t, s, spaceID, "worker")
	_, otherTok := mustAdmit(t, s, 200, "worker")
	wantRevoked(t, s, memberID, InSpace(spaceID), 1)
	wantAlive(t, s, map[string]bool{memberTok: false, otherTok: true, leadTok: true})
	wantRevoked(t, s, memberID, InSpace(spaceID), 0)
	wantRevoked(t, s, memberID, Everywhere(), 1)
	wantRevoked(t, s, lead.AgentID, Everywhere(), 1)
	wantAlive(t, s, map[string]bool{otherTok: false, leadTok: false})
}

// mustAdmit admits member name into spaceID and returns its agent id and token.
func mustAdmit(t *testing.T, s *Service, spaceID int64, name string) (int64, string) {
	t.Helper()
	id, tok, err := s.AdmitMember(context.Background(), s.db, spaceID, name)
	if err != nil {
		t.Fatalf("AdmitMember(%s): %v", name, err)
	}
	return id, tok
}

// wantRevoked revokes agentID's tokens in scope and checks how many were removed.
func wantRevoked(t *testing.T, s *Service, agentID int64, scope Scope, want int64) {
	t.Helper()
	n, err := s.Revoke(context.Background(), s.db, agentID, scope)
	if err != nil || n != want {
		t.Fatalf("Revoke(%d, %+v) = %d, %v; want %d", agentID, scope, n, err, want)
	}
}

// wantAlive checks for each token whether it still authenticates.
func wantAlive(t *testing.T, s *Service, tokens map[string]bool) {
	t.Helper()
	for tok, alive := range tokens {
		_, err := s.Authenticate(context.Background(), tok)
		if alive && err != nil {
			t.Fatalf("Authenticate(%s): %v", tok, err)
		}
		if !alive {
			apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
		}
	}
}
