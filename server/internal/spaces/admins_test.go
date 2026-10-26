package spaces

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

// mintedAdmin mints admin name through the auth service and returns its token and principal.
func mintedAdmin(t *testing.T, e env, root auth.Principal, name string) (string, auth.Principal) {
	t.Helper()
	ctx := context.Background()
	tok, err := e.auth.MintAdmin(ctx, root, name)
	if err != nil {
		t.Fatalf("MintAdmin(%s): %v", name, err)
	}
	p, err := e.auth.Authenticate(ctx, tok)
	if err != nil {
		t.Fatalf("Authenticate %s: %v", name, err)
	}
	return tok, p
}

func bootstrapRoot(t *testing.T, e env) auth.Principal {
	t.Helper()
	if err := e.auth.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	p, err := e.auth.Authenticate(context.Background(), "admin-secret")
	if err != nil {
		t.Fatalf("Authenticate bootstrap: %v", err)
	}
	return p
}

func TestRevokeAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	root := bootstrapRoot(t, e)
	lead := leader(t, e, 50, "lead")
	webTok, web := mintedAdmin(t, e, root, "web")
	_, ops := mintedAdmin(t, e, root, "ops")
	mustCreate(t, e, web, "gamma")
	mustCreate(t, e, lead, "demo")
	mustJoin(t, e, web, "demo")
	seedInbox(t, e, auth.Principal{AgentID: web.AgentID, SpaceID: spaceID(t, e, "demo")})

	deleted, err := e.svc.RevokeAdmin(ctx, ops, "web")
	if err != nil || !deleted {
		t.Fatalf("RevokeAdmin(web) = %v, %v; want true", deleted, err)
	}
	_, err = e.auth.Authenticate(ctx, webTok)
	apperrtest.WantKind(t, err, apperr.Unauthenticated, "invalid token")
	wantGone(t, e, "gamma")
	wantParticipants(t, e, "demo", []Participant{{"lead", RoleLeader, false}})
	wantNoInbox(t, e, auth.Principal{AgentID: web.AgentID, SpaceID: spaceID(t, e, "demo")})

	tests := []struct {
		name, target string
		p            auth.Principal
		deleted      bool
		kind         apperr.Kind
		msg          string
	}{
		{name: "already revoked", p: root, target: "web"},
		{name: "unknown name", p: root, target: "ghost"},
		{name: "bootstrap admin", p: ops, target: auth.BootstrapName, kind: apperr.Forbidden,
			msg: "admin is the bootstrap admin; rotate its token with agent-message-bus rotate-token"},
		{name: "a leader", p: root, target: "lead", kind: apperr.Conflict, msg: "agent lead is a leader agent"},
		{name: "leader caller", p: lead, target: "ops", kind: apperr.Forbidden, msg: "admin token required"},
		{name: "self revoke", p: ops, target: "ops", deleted: true},
	}
	for _, tc := range tests {
		got, err := e.svc.RevokeAdmin(ctx, tc.p, tc.target)
		if tc.kind != "" {
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
			continue
		}
		if err != nil || got != tc.deleted {
			t.Fatalf("%s: RevokeAdmin = %v, %v; want %v", tc.name, got, err, tc.deleted)
		}
	}
	_, again := mintedAdmin(t, e, root, "web")
	if again.AgentID != web.AgentID {
		t.Fatalf("re-minted web has agent id %d, want %d", again.AgentID, web.AgentID)
	}
}

func TestAuditLineRevoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t, nil, nil)
	root := bootstrapRoot(t, e)
	var buf bytes.Buffer
	e.svc.log = slog.New(slog.NewJSONHandler(&buf, nil))
	mintedAdmin(t, e, root, "web")
	lead := leader(t, e, 50, "lead")
	if _, err := e.svc.RevokeAdmin(ctx, lead, "web"); err == nil {
		t.Fatal("leader revoked an admin")
	}
	for _, name := range []string{"web", "ghost"} {
		if _, err := e.svc.RevokeAdmin(ctx, root, name); err != nil {
			t.Fatalf("RevokeAdmin(%s): %v", name, err)
		}
	}
	dec := json.NewDecoder(&buf)
	for _, want := range []struct {
		name    string
		deleted bool
	}{{"web", true}, {"ghost", false}} {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatalf("decoding audit line: %v", err)
		}
		if line["msg"] != "admin revoked" || line["by"] != "admin" || line["name"] != want.name || line["deleted"] != want.deleted {
			t.Fatalf("audit line = %v, want admin revoked %s deleted %v", line, want.name, want.deleted)
		}
	}
	if dec.More() {
		t.Fatal("more audit lines than revokes")
	}
}
