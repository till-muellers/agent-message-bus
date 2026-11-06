package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUnknownAPIRoute: GET or HEAD on an unknown /web/api/<path> answers with the §0
// envelope after the session check; other methods get the mux's 405 before it, documented in
// contracts.md §10.
func TestUnknownAPIRoute(t *testing.T) {
	t.Parallel()
	h, session := adminSession(t)
	const notFound = `{"error":{"code":"not_found","message":"route not found"}}`
	tests := []struct {
		name, method, path  string
		session             bool
		wantStatus          int
		wantBody, wantAllow string
	}{
		{"unknown path", http.MethodGet, "/web/api/nope", true, http.StatusNotFound, notFound, ""},
		{"unknown nested path", http.MethodGet, "/web/api/spaces/demo", true, http.StatusNotFound, notFound, ""},
		{"HEAD like GET", http.MethodHead, "/web/api/nope", true, http.StatusNotFound, "", ""},
		{"no session first", http.MethodGet, "/web/api/nope", false, http.StatusUnauthorized, `"code":"unauthenticated"`, ""},
		{"POST gets the mux 405", http.MethodPost, "/web/api/nope", true, http.StatusMethodNotAllowed, "Method Not Allowed", "GET, HEAD"},
		{"DELETE gets the mux 405", http.MethodDelete, "/web/api/nope", true, http.StatusMethodNotAllowed, "Method Not Allowed", "GET, HEAD"},
		{"405 before the session check", http.MethodPost, "/web/api/nope", false, http.StatusMethodNotAllowed, "Method Not Allowed", "GET, HEAD"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.session {
				req.AddCookie(session)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, tc.wantStatus, tc.wantBody)
			}
			if got := rec.Header().Get("Allow"); got != tc.wantAllow {
				t.Fatalf("Allow %q, want %q", got, tc.wantAllow)
			}
		})
	}
}
