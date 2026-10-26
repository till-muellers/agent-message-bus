package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

var adminTokenRE = regexp.MustCompile(`^amb_admin_[A-Za-z0-9_-]{43}$`)

// bootstrapped returns a Service whose bootstrap admin exists, and that admin's principal.
func bootstrapped(t *testing.T) (*Service, Principal) {
	t.Helper()
	s, _ := newTestService(t)
	if err := s.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return s, s.bootstrapPrincipal()
}

func TestMintAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, root := bootstrapped(t)
	_, lead := mustLeader(t, s, "lead")
	if _, _, err := s.MintMember(ctx, s.db, "worker", seedSpace(t, s.db, lead.AgentID)); err != nil {
		t.Fatalf("MintMember: %v", err)
	}
	tests := []struct {
		name, target string
		p            Principal
		kind         apperr.Kind
		msg          string
	}{
		{name: "leader before name check", p: lead, target: "Bad Name", kind: apperr.Forbidden, msg: "admin token required"},
		{name: "invalid name", p: root, target: "Bad Name", kind: apperr.Invalid, msg: "name must match ^[a-z0-9][a-z0-9_-]{0,62}$"},
		{name: "bootstrap name", p: root, target: BootstrapName, kind: apperr.Conflict,
			msg: "admin is the bootstrap admin; its token is the instance admin token"},
		{name: "leader name", p: root, target: "lead", kind: apperr.Conflict, msg: "agent lead is a leader agent"},
		{name: "member name", p: root, target: "worker", kind: apperr.Conflict, msg: "agent worker is a member agent"},
	}
	for _, tc := range tests {
		_, err := s.MintAdmin(ctx, tc.p, tc.target)
		apperrtest.WantKind(t, err, tc.kind, tc.msg)
	}
	first := mustMintAdmin(t, s, root, "ops")
	second := mustMintAdmin(t, s, root, "ops")
	p1, err1 := s.Authenticate(ctx, first)
	p2, err2 := s.Authenticate(ctx, second)
	if err1 != nil || err2 != nil || first == second || p1 != p2 || p1.Kind != Admin || p1.AgentName != "ops" || p1.Bootstrap {
		t.Fatalf("re-mint: %+v %v / %+v %v", p1, err1, p2, err2)
	}
	if _, err := s.MintAdmin(ctx, p1, "web"); err != nil {
		t.Fatalf("minted admin mints admin: %v", err)
	}
}

func mustMintAdmin(t *testing.T, s *Service, p Principal, name string) string {
	t.Helper()
	tok, err := s.MintAdmin(context.Background(), p, name)
	if err != nil {
		t.Fatalf("MintAdmin(%s): %v", name, err)
	}
	if !adminTokenRE.MatchString(tok) {
		t.Fatalf("token %q is not amb_admin_ + 43 base64url characters", tok)
	}
	return tok
}

func TestListAdmins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, root := bootstrapped(t)
	_, lead := mustLeader(t, s, "lead")
	mustMintAdmin(t, s, root, "web")
	mustMintAdmin(t, s, root, "ops")
	mustMintAdmin(t, s, root, "ops")
	_, err := s.ListAdmins(ctx, lead)
	apperrtest.WantKind(t, err, apperr.Forbidden, "admin token required")
	got, err := s.ListAdmins(ctx, root)
	want := []AdminInfo{
		{Name: "admin", Bootstrap: true, Tokens: 0, Spaces: []string{}},
		{Name: "ops", Tokens: 2, Spaces: []string{}},
		{Name: "web", Tokens: 1, Spaces: []string{}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ListAdmins = %+v, %v; want %+v", got, err, want)
	}
}

func TestAuditLineMint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, root := bootstrapped(t)
	var buf bytes.Buffer
	s.log = slog.New(slog.NewJSONHandler(&buf, nil))
	_, lead := mustLeader(t, s, "lead")
	if _, err := s.MintAdmin(ctx, lead, "x"); err == nil {
		t.Fatal("leader minted an admin")
	}
	if buf.Len() != 0 {
		t.Fatalf("refused call logged %q", buf.String())
	}
	tok := mustMintAdmin(t, s, root, "ops")
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("decoding log line %q: %v", buf.String(), err)
	}
	if line["level"] != "INFO" || line["msg"] != "admin minted" || line["by"] != "admin" || line["name"] != "ops" {
		t.Fatalf("log line = %v", line)
	}
	if strings.Contains(buf.String(), tok) || strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("log %q contains the token or more than one line", buf.String())
	}
}

func TestRotateKeepsMintedAdmins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, root := bootstrapped(t)
	tok := mustMintAdmin(t, s, root, "ops")
	if err := s.SetAdminToken(strings.Repeat("n", MinAdminTokenLen)); err != nil {
		t.Fatalf("SetAdminToken: %v", err)
	}
	if p, err := s.Authenticate(ctx, tok); err != nil || p.AgentName != "ops" {
		t.Fatalf("minted admin after rotation = %+v, %v", p, err)
	}
}
