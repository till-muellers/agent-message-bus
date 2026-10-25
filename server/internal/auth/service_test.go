package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

const testAdmin = "admin-token-admin-token-admin-token"

var admin = Principal{Kind: Admin}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	db := dbtest.Open(t)
	now := func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	return NewService(db, testAdmin, now), db
}

func insertSpace(t *testing.T, db *sql.DB, name string, createdBy int64) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO spaces (name, created_by, created_at) VALUES (?, ?, ?)`,
		name, createdBy, "2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("insert space: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("space id: %v", err)
	}
	return id
}

func mustLeader(t *testing.T, s *Service, name string) (string, Principal) {
	t.Helper()
	ctx := context.Background()
	tok, err := s.MintLeader(ctx, admin, name)
	if err != nil {
		t.Fatalf("MintLeader(%s): %v", name, err)
	}
	p, err := s.Authenticate(ctx, tok)
	if err != nil {
		t.Fatalf("Authenticate leader: %v", err)
	}
	return tok, p
}

func wantKind(t *testing.T, err error, want apperr.Kind, msg string) {
	t.Helper()
	k, ok := apperr.KindOf(err)
	if !ok || k != want {
		t.Fatalf("err = %v, want kind %q", err, want)
	}
	if msg != "" && err.Error() != msg {
		t.Errorf("msg = %q, want %q", err.Error(), msg)
	}
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := insertSpace(t, db, "demo", lead.AgentID)
	memberID, memberTok, err := s.MintMember(ctx, db, "worker", spaceID)
	if err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	leadTok2, err := s.MintLeader(ctx, admin, "lead")
	if err != nil {
		t.Fatalf("second MintLeader: %v", err)
	}
	tests := []struct {
		name  string
		token string
		want  Principal
		err   apperr.Kind
	}{
		{"admin", testAdmin, Principal{Kind: Admin}, ""},
		{"leader additional token", leadTok2, lead, ""},
		{"member", memberTok, Principal{Kind: Member, AgentID: memberID, AgentName: "worker", SpaceID: spaceID}, ""},
		{"unknown", "amb_nope", Principal{}, apperr.Unauthenticated},
		{"empty", "", Principal{}, apperr.Unauthenticated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.Authenticate(ctx, tc.token)
			if tc.err != "" {
				wantKind(t, err, tc.err, "invalid token")
				return
			}
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if got != tc.want {
				t.Errorf("principal = %+v, want %+v", got, tc.want)
			}
		})
	}
	if lead.Kind != Leader || lead.AgentName != "lead" || lead.SpaceID != 0 {
		t.Errorf("leader principal = %+v", lead)
	}
}

func TestMintLeaderErrors(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := insertSpace(t, db, "demo", lead.AgentID)
	if _, _, err := s.MintMember(ctx, db, "worker", spaceID); err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	tests := []struct {
		name   string
		caller Principal
		agent  string
		kind   apperr.Kind
		msg    string
	}{
		{"leader caller", lead, "other", apperr.Forbidden, "admin token required"},
		{"member caller", Principal{Kind: Member, AgentID: 9, SpaceID: spaceID}, "other", apperr.Forbidden, "admin token required"},
		{"invalid name", admin, "Bad Name", apperr.Invalid, "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"},
		{"member name collision", admin, "worker", apperr.Conflict, "agent worker is a member agent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.MintLeader(ctx, tc.caller, tc.agent)
			wantKind(t, err, tc.kind, tc.msg)
		})
	}
}

func TestMintMemberErrors(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := insertSpace(t, db, "demo", lead.AgentID)
	_, _, err := s.MintMember(ctx, db, "lead", spaceID)
	wantKind(t, err, apperr.Conflict, "agent lead is a leader agent")
}

func TestMintMemberReinviteSameAgent(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := insertSpace(t, db, "demo", lead.AgentID)
	id1, tok1, err := s.MintMember(ctx, db, "worker", spaceID)
	if err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	id2, tok2, err := s.MintMember(ctx, db, "worker", spaceID)
	if err != nil {
		t.Fatalf("second MintMember: %v", err)
	}
	if id1 != id2 || tok1 == tok2 {
		t.Errorf("ids %d/%d tokens equal=%v", id1, id2, tok1 == tok2)
	}
}

func TestStoredHashIsNotPlaintext(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	tok, _ := mustLeader(t, s, "lead")
	var hash string
	if err := db.QueryRow(`SELECT hash FROM tokens`).Scan(&hash); err != nil {
		t.Fatalf("select hash: %v", err)
	}
	if hash == tok || hash != Hash(tok) {
		t.Errorf("stored %q, token %q", hash, tok)
	}
}
