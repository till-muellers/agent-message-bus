package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/tools"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

// TestUnknownAPIRoute: an unknown GET /web/api/<path> answers with the §0 envelope like every
// other /web/api error; other methods keep the mux's 405, documented in contracts.md §10.
func TestUnknownAPIRoute(t *testing.T) {
	t.Parallel()
	c := &clock{t: time.Now()}
	mux := http.NewServeMux()
	web.New(web.Deps{Auth: auth.NewService(dbtest.Open(t), adminToken, c.now), Spaces: adminWho{},
		Tools: tools.NewRegistry(nil), Dist: fstest.MapFS{}, Now: c.now}).Mount(mux)
	h := web.Headers(mux)
	signIn := serve(h, http.MethodPost, "/web/session", "192.0.2.1:4000", "Bearer "+adminToken)
	if signIn.Code != http.StatusOK || len(signIn.Result().Cookies()) == 0 {
		t.Fatalf("sign-in: %d %s", signIn.Code, signIn.Body)
	}
	const notFound = `{"error":{"code":"not_found","message":"route not found"}}`
	tests := []struct {
		name, method, path string
		session            bool
		wantStatus         int
		wantBody           string
	}{
		{"unknown path", http.MethodGet, "/web/api/nope", true, http.StatusNotFound, notFound},
		{"unknown nested path", http.MethodGet, "/web/api/spaces/demo", true, http.StatusNotFound, notFound},
		{"no session first", http.MethodGet, "/web/api/nope", false, http.StatusUnauthorized, `"code":"unauthenticated"`},
		{"other method keeps the mux 405", http.MethodPost, "/web/api/nope", true, http.StatusMethodNotAllowed, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.session {
				req.AddCookie(signIn.Result().Cookies()[0])
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}
