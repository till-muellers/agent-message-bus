package invites

import (
	"context"
	"regexp"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

// cancelCase cancels the id that setup returns in space demo as the principal of as (nil =
// leader lead) and expects cancelled; check inspects the outcome with setup's code.
type cancelCase struct {
	name  string
	as    func(t *testing.T, e env) auth.Principal
	setup func(t *testing.T, e env) (id, code string)
	want  bool
	check func(t *testing.T, e env, code string)
}

// TestCancelByID follows gui-invite-cancel.feature "The GUI cancels one pending invite".
func TestCancelByID(t *testing.T) {
	t.Parallel()
	for _, tc := range cancelCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			id, code := tc.setup(t, e)
			p := e.lead
			if tc.as != nil {
				p = tc.as(t, e)
			}
			name, cancelled, err := e.svc.Cancel(context.Background(), p, "demo", id)
			if err != nil || cancelled != tc.want || (cancelled && name != "worker") {
				t.Fatalf("Cancel(%q) = %q, %v, %v; want worker, %v", id, name, cancelled, err, tc.want)
			}
			if tc.check != nil {
				tc.check(t, e, code)
			}
		})
	}
}

func cancelCases() []cancelCase {
	return []cancelCase{
		{name: "pending invite", setup: pending, want: true, check: refused},
		{name: "unjoined admin", setup: pending, want: true, as: admin, check: refused},
		{name: "already cancelled", setup: func(t *testing.T, e env) (string, string) {
			id, code := pending(t, e)
			mustCancel(t, e, id)
			return id, code
		}},
		{name: "unknown id", setup: func(*testing.T, env) (string, string) { return "nope", "" }},
		{name: "malformed id", setup: func(*testing.T, env) (string, string) { return "' OR 1=1 --", "" }},
		{name: "redeemed", setup: func(t *testing.T, e env) (string, string) {
			id, code := pending(t, e)
			accepted(t, e, code)
			return id, code
		}},
		{name: "expired", setup: func(t *testing.T, e env) (string, string) {
			id, code := pending(t, e)
			*e.clock = start.Add(TTL)
			return id, code
		}},
		{name: "id of another space", setup: otherSpaceInvite, check: accepted},
		{name: "another code of the same name stays", want: true, check: accepted,
			setup: func(t *testing.T, e env) (string, string) {
				id, _ := pending(t, e)
				return id, mustCreate(t, e, "worker", nil).Code
			}},
		{name: "a joined member stays", want: true, check: stillMember,
			setup: func(t *testing.T, e env) (string, string) {
				accepted(t, e, mustCreate(t, e, "worker", nil).Code)
				return pending(t, e)
			}},
	}
}

// pending creates an invite for worker in demo and returns its id and code.
func pending(t *testing.T, e env) (id, code string) {
	t.Helper()
	code = mustCreate(t, e, "worker", nil).Code
	return idOf(t, e, "demo", 0), code
}

// otherSpaceInvite makes lead create space "other" with an invite for worker and returns
// its id and code.
func otherSpaceInvite(t *testing.T, e env) (id, code string) {
	t.Helper()
	ctx := context.Background()
	if err := e.spaces.Create(ctx, e.lead, "other"); err != nil {
		t.Fatalf("creating other: %v", err)
	}
	inv, err := e.svc.Create(ctx, e.lead, "other", "worker", nil)
	if err != nil {
		t.Fatalf("inviting into other: %v", err)
	}
	return idOf(t, e, "other", 0), inv.Code
}

func admin(t *testing.T, e env) auth.Principal {
	t.Helper()
	dbtest.SeedAgent(t, e.db, 9, "ops", "admin")
	return auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
}

// idOf returns the id of the i-th open invite of space.
func idOf(t *testing.T, e env, space string, i int) string {
	t.Helper()
	ps := listPending(t, e, e.lead, space)
	if len(ps) <= i {
		t.Fatalf("open invites of %s = %+v, want more than %d", space, ps, i)
	}
	return ps[i].ID
}

func mustCancel(t *testing.T, e env, id string) {
	t.Helper()
	if _, ok, err := e.svc.Cancel(context.Background(), e.lead, "demo", id); err != nil || !ok {
		t.Fatalf("cancel %s = %v, %v", id, ok, err)
	}
}

func refused(t *testing.T, e env, code string) {
	t.Helper()
	_, err := e.svc.Accept(context.Background(), code)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, invalidCode)
}

func accepted(t *testing.T, e env, code string) {
	t.Helper()
	if _, err := e.svc.Accept(context.Background(), code); err != nil {
		t.Fatalf("accept %s: %v", code, err)
	}
}

func stillMember(t *testing.T, e env, _ string) {
	t.Helper()
	var n int
	err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM participants p JOIN agents a ON a.id = p.agent_id
		WHERE a.name = 'worker' AND p.role = 'member'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("worker memberships = %d, %v; want 1", n, err)
	}
}

// TestCancelAccess: only leaders of the space and admins may cancel.
func TestCancelAccess(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	code := mustCreate(t, e, "worker", nil).Code
	id := idOf(t, e, "demo", 0)
	member, err := e.svc.Accept(ctx, mustCreate(t, e, "helper", nil).Code)
	if err != nil {
		t.Fatalf("accept helper: %v", err)
	}
	memberP, err := e.auth.Authenticate(ctx, member.Token)
	if err != nil {
		t.Fatalf("authenticate helper: %v", err)
	}
	_, _, err = e.svc.Cancel(ctx, memberP, "demo", id)
	apperrtest.WantKind(t, err, apperr.Forbidden, "only leaders may list or cancel invites")
	_, _, err = e.svc.Cancel(ctx, e.lead, "nope", id)
	apperrtest.WantKind(t, err, apperr.NotFound, "space nope not found")
	accepted(t, e, code)
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TestListPendingHasID: every open invite is listed with its own opaque id.
func TestListPendingHasID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	first := mustCreate(t, e, "worker", nil)
	second := mustCreate(t, e, "worker", nil)
	ps := listPending(t, e, e.lead, "demo")
	if len(ps) != 2 || ps[0].ID == ps[1].ID {
		t.Fatalf("open invites = %+v, want two with distinct ids", ps)
	}
	for _, p := range ps {
		if !idPattern.MatchString(p.ID) || p.ID == normalize(first.Code) || p.ID == normalize(second.Code) {
			t.Fatalf("invite id %q, want 16 lowercase hex chars, not the code", p.ID)
		}
	}
}
