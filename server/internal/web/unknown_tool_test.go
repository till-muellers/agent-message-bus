package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

// adminWho answers the sign-in's Whoami as the bootstrap admin.
type adminWho struct{}

func (adminWho) Whoami(context.Context, auth.Principal) (spaces.WhoamiResult, error) {
	return spaces.WhoamiResult{Kind: string(auth.Admin), Name: "admin"}, nil
}

func (adminWho) List(context.Context, auth.Principal) ([]spaces.SpaceInfo, error) { return nil, nil }

// adminSession mounts the GUI over an empty registry and signs in with the admin token; it
// returns the wrapped handler and the session cookie.
func adminSession(t *testing.T) (http.Handler, *http.Cookie) {
	t.Helper()
	c := &clock{t: time.Now()}
	mux := http.NewServeMux()
	web.New(web.Deps{Auth: auth.NewService(dbtest.Open(t), adminToken, c.now), Spaces: adminWho{},
		Tools: tools.NewRegistry(nil), Dist: fstest.MapFS{}, Now: c.now}).Mount(mux)
	h := web.Headers(mux)
	signIn := serve(h, http.MethodPost, "/web/session", "192.0.2.1:4000", "Bearer "+adminToken)
	cookies := signIn.Result().Cookies()
	if signIn.Code != http.StatusOK || len(cookies) == 0 {
		t.Fatalf("sign-in: %d %s", signIn.Code, signIn.Body)
	}
	return h, cookies[0]
}

// TestUnregisteredToolIsNotFound: a GUI endpoint whose tool the registry lacks answers like
// POST /v1/tools/<name> does, 404 not_found, not 500.
func TestUnregisteredToolIsNotFound(t *testing.T) {
	t.Parallel()
	h, session := adminSession(t)
	req := httptest.NewRequest(http.MethodGet, "/web/api/admins", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	const want = `{"error":{"code":"not_found","message":"tool bus_list_admins not found"}}`
	if rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("got %d %s, want 404 %s", rec.Code, rec.Body, want)
	}
}
