package invites

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	inv := mustCreate(t, e, "worker", nil)
	if inv.Name != "worker" || inv.Space != "demo" || !inv.ExpiresAt.Equal(start.Add(10*time.Minute)) {
		t.Fatalf("invite = %+v", inv)
	}
	var hash string
	if err := e.db.QueryRowContext(context.Background(), `SELECT hash FROM invites`).Scan(&hash); err != nil {
		t.Fatalf("reading hash: %v", err)
	}
	if hash != auth.Hash(normalize(inv.Code)) {
		t.Fatalf("stored hash %q for code %q", hash, inv.Code)
	}
}

func TestCreateErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	dbtest.SeedAgent(t, e.db, 2, "boss", "leader")
	empty, long := "", strings.Repeat("x", domain.MaxBodyBytes+1)
	tests := []struct {
		name, space, invitee string
		task                 *string
		kind                 apperr.Kind
		msg                  string
	}{
		{name: "leader name", space: "demo", invitee: "boss", kind: apperr.Conflict, msg: "agent boss is a leader agent"},
		{name: "unknown space", space: "nope", invitee: "worker", kind: apperr.NotFound},
		{name: "empty task", space: "demo", invitee: "worker", task: &empty, kind: apperr.Invalid, msg: "task must be 1..65536 bytes"},
		{name: "long task", space: "demo", invitee: "worker", task: &long, kind: apperr.Invalid, msg: "task must be 1..65536 bytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Create(context.Background(), e.lead, tc.space, tc.invitee, tc.task)
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
		})
	}
	if n := countInvites(t, e); n != 0 {
		t.Fatalf("invites = %d", n)
	}
}

func TestCreateDeletesExpired(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	mustCreate(t, e, "old", nil)
	*e.clock = start.Add(TTL)
	mustCreate(t, e, "new", nil)
	if n := countInvites(t, e); n != 1 {
		t.Fatalf("invites = %d, want 1", n)
	}
}

func TestAcceptOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	inv := mustCreate(t, e, "worker", nil)
	got, err := e.svc.Accept(ctx, inv.Code)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got.Name != "worker" || got.Space != "demo" || got.Token == "" {
		t.Fatalf("admission = %+v", got)
	}
	p, err := e.auth.Authenticate(ctx, got.Token)
	if err != nil || p.Kind != auth.Member || p.AgentName != "worker" {
		t.Fatalf("token principal = %+v, %v", p, err)
	}
	ps, err := e.spaces.Participants(ctx, e.lead, "demo")
	if err != nil || len(ps) != 2 || ps[1].Name != "worker" {
		t.Fatalf("participants = %+v, %v", ps, err)
	}
	_, err = e.svc.Accept(ctx, inv.Code)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid or expired invite code")
}

func TestAcceptRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		code  func(Invite) string
		after time.Duration
	}{
		{name: "expired", code: func(i Invite) string { return i.Code }, after: TTL},
		{name: "unknown", code: func(Invite) string { return "2345-6789" }},
		{name: "too short", code: func(Invite) string { return "2345" }},
		{name: "empty", code: func(Invite) string { return "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			inv := mustCreate(t, e, "worker", nil)
			*e.clock = start.Add(tc.after)
			_, err := e.svc.Accept(context.Background(), tc.code(inv))
			apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid or expired invite code")
		})
	}
}

func TestAcceptNormalizes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	inv := mustCreate(t, e, "worker", nil)
	if _, err := e.svc.Accept(context.Background(), strings.ToUpper(normalize(inv.Code))); err != nil {
		t.Fatalf("accept upper case without dash: %v", err)
	}
}

func TestDeletingSpaceRemovesInvites(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	mustCreate(t, e, "worker", nil)
	if err := e.spaces.Delete(context.Background(), e.lead, "demo"); err != nil {
		t.Fatalf("delete space: %v", err)
	}
	if n := countInvites(t, e); n != 0 {
		t.Fatalf("invites = %d", n)
	}
}

func TestAcceptConflictKeepsCode(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	inv := mustCreate(t, e, "x", nil)
	if _, err := e.auth.MintLeader(ctx, auth.Principal{Kind: auth.Admin}, "x"); err != nil {
		t.Fatalf("mint leader x: %v", err)
	}
	_, err := e.svc.Accept(ctx, inv.Code)
	apperrtest.WantKind(t, err, apperr.Conflict, "agent x is a leader agent")
	if n := countInvites(t, e); n != 1 {
		t.Fatalf("invites = %d, want the code kept", n)
	}
}
