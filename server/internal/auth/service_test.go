package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
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

// seedSpace inserts a space created by createdBy and returns its id.
func seedSpace(t *testing.T, db *sql.DB, createdBy int64) int64 {
	t.Helper()
	const id = 100
	dbtest.SeedSpace(t, db, id, "demo", createdBy)
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

func TestAuthenticate(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
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
		{"admin", testAdmin, Principal{Kind: Admin, AgentName: BootstrapName, Bootstrap: true}, ""},
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
				apperrtest.WantKind(t, err, tc.err, "invalid token")
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
	spaceID := seedSpace(t, db, lead.AgentID)
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
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
		})
	}
}

func TestMintMemberErrors(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
	_, _, err := s.MintMember(ctx, db, "lead", spaceID)
	apperrtest.WantKind(t, err, apperr.Conflict, "agent lead is a leader agent")
}

func TestMintMemberReinviteSameAgent(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
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

func TestRevokeSpaceTokens(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	leadTok, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
	dbtest.SeedSpace(t, db, 200, "other", lead.AgentID)
	memberID, memberTok, err := s.MintMember(ctx, db, "worker", spaceID)
	if err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	_, otherTok, err := s.MintMember(ctx, db, "worker", 200)
	if err != nil {
		t.Fatalf("MintMember other: %v", err)
	}
	if err := s.RevokeSpaceTokens(ctx, db, memberID, spaceID); err != nil {
		t.Fatalf("RevokeSpaceTokens: %v", err)
	}
	tests := []struct {
		name  string
		token string
		alive bool
	}{
		{"revoked member", memberTok, false},
		{"other space", otherTok, true},
		{"leader", leadTok, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.Authenticate(ctx, tc.token)
			if tc.alive && err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if !tc.alive {
				apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
			}
		})
	}
}

func TestCheckMemberName(t *testing.T) {
	t.Parallel()
	s, db := newTestService(t)
	ctx := context.Background()
	_, lead := mustLeader(t, s, "lead")
	spaceID := seedSpace(t, db, lead.AgentID)
	if _, _, err := s.MintMember(ctx, db, "worker", spaceID); err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	tests := []struct {
		name     string
		input    string
		wantKind apperr.Kind
		wantMsg  string
	}{
		{name: "new valid name", input: "fresh"},
		{name: "existing member", input: "worker"},
		{name: "leader name", input: "lead", wantKind: apperr.Conflict, wantMsg: "agent lead is a leader agent"},
		{name: "invalid name", input: "Bad Name", wantKind: apperr.Invalid,
			wantMsg: "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.CheckMemberName(ctx, db, tc.input)
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("CheckMemberName(%q) = %v", tc.input, err)
				}
				return
			}
			apperrtest.WantKind(t, err, tc.wantKind, tc.wantMsg)
		})
	}
	if n := countAgentsNamed(t, db, "fresh"); n != 0 {
		t.Fatalf("CheckMemberName inserted agent fresh (%d rows)", n)
	}
}

func countAgentsNamed(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM agents WHERE name = ?`, name).Scan(&n); err != nil {
		t.Fatalf("counting agents: %v", err)
	}
	return n
}
