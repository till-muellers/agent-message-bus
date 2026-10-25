package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware(t *testing.T) {
	t.Parallel()
	s, _ := newTestService(t)
	leadTok, _ := mustLeader(t, s, "lead")
	h := s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok {
			t.Error("no principal in context")
		}
		_, _ = w.Write([]byte(p.Kind))
	}))
	tests := []struct {
		name   string
		header string
		status int
		body   string
	}{
		{"admin", "Bearer " + testAdmin, http.StatusOK, "admin"},
		{"leader", "Bearer " + leadTok, http.StatusOK, "leader"},
		{"missing", "", http.StatusUnauthorized, ""},
		{"wrong scheme", "Basic " + leadTok, http.StatusUnauthorized, ""},
		{"empty bearer", "Bearer ", http.StatusUnauthorized, ""},
		{"unknown", "Bearer amb_nope", http.StatusUnauthorized, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if tc.body != "" && rec.Body.String() != tc.body {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}
