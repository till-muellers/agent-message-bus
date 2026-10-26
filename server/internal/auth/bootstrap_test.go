package auth

import (
	"context"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func TestBootstrap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("creates the admin once and reuses it", func(t *testing.T) {
		t.Parallel()
		s, db := newTestService(t)
		if err := s.Bootstrap(ctx); err != nil {
			t.Fatalf("first Bootstrap: %v", err)
		}
		first := s.bootstrapID.Load()
		again := NewService(db, testAdmin, s.now)
		if err := again.Bootstrap(ctx); err != nil {
			t.Fatalf("second Bootstrap: %v", err)
		}
		if first == 0 || again.bootstrapID.Load() != first {
			t.Fatalf("bootstrap ids = %d, %d; want the same non-zero id", first, again.bootstrapID.Load())
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents WHERE name = 'admin' AND kind = 'admin'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Fatalf("admin agents = %d, want 1", n)
		}
	})
	t.Run("an existing leader named admin is refused", func(t *testing.T) {
		t.Parallel()
		s, db := newTestService(t)
		dbtest.SeedAgent(t, db, 1, BootstrapName, "leader")
		apperrtest.WantKind(t, s.Bootstrap(ctx), apperr.Conflict,
			"bootstrapping admin agent: agent admin is a leader agent")
	})
}

func TestAuthenticateInstanceToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := newTestService(t)
	if err := s.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	id := s.bootstrapID.Load()
	want := Principal{Kind: Admin, AgentID: id, AgentName: BootstrapName, Bootstrap: true}
	for _, tc := range []struct{ name, token string }{
		{"instance token", testAdmin},
		{"rotated token", "rotated-token-rotated-token-rotated"},
	} {
		if tc.token != testAdmin {
			if err := s.SetAdminToken(tc.token); err != nil {
				t.Fatalf("SetAdminToken: %v", err)
			}
		}
		got, err := s.Authenticate(ctx, tc.token)
		if err != nil || got != want {
			t.Fatalf("%s: Authenticate = %+v, %v; want %+v", tc.name, got, err, want)
		}
	}
}

func TestKindConflictArticle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, db := newTestService(t)
	dbtest.SeedAgent(t, db, 1, "ops", "admin")
	dbtest.SeedAgent(t, db, 2, "lead", "leader")
	tests := []struct{ name, msg string }{
		{"ops", "agent ops is an admin agent"},
		{"lead", "agent lead is a leader agent"},
	}
	for _, tc := range tests {
		_, _, err := s.MintMember(ctx, db, tc.name, 0)
		apperrtest.WantKind(t, err, apperr.Conflict, tc.msg)
	}
}

func TestMintLeaderNamedAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := newTestService(t)
	if err := s.Bootstrap(ctx); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	_, err := s.MintLeader(ctx, admin, BootstrapName)
	apperrtest.WantKind(t, err, apperr.Conflict, "agent admin is an admin agent")
}
