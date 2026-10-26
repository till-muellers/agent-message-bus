package web_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

const adminToken = "admin-token-0123456789-0123456789-abcdef"

// clock is a settable test clock.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// newHandler mounts the GUI routes over dist with a real auth service, the way
// server.WebRoutes does. Who stays nil: these tests never complete a sign-in.
func newHandler(t *testing.T, dist fs.FS, c *clock) http.Handler {
	t.Helper()
	a := auth.NewService(dbtest.Open(t), adminToken, c.now)
	mux := http.NewServeMux()
	web.New(web.Deps{Auth: a, Dist: dist, Now: c.now}).Mount(mux)
	return web.Headers(mux)
}

func serve(h http.Handler, method, path, remote, authz string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remote
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSignInRateLimit(t *testing.T) {
	t.Parallel()
	c := &clock{t: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
	h := newHandler(t, fstest.MapFS{}, c)
	const ip = "192.0.2.7:4000"
	for i := range 5 {
		if rec := serve(h, http.MethodPost, "/web/session", ip, "Bearer amb_nope"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	const want = `{"error":{"code":"rate_limited","message":"too many failed sign-ins; try again later"}}`
	for _, authz := range []string{"Bearer amb_nope", "Bearer " + adminToken} {
		rec := serve(h, http.MethodPost, "/web/session", "192.0.2.7:4001", authz)
		if rec.Code != http.StatusTooManyRequests || strings.TrimSpace(rec.Body.String()) != want {
			t.Fatalf("after 5 failures: %d %s", rec.Code, rec.Body)
		}
		if rec.Header().Get("Set-Cookie") != "" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("429 headers: %v", rec.Header())
		}
	}
	if rec := serve(h, http.MethodPost, "/web/session", "198.51.100.1:4000", "Bearer amb_nope"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("another IP: %d %s", rec.Code, rec.Body)
	}
	c.t = c.t.Add(time.Minute)
	if rec := serve(h, http.MethodPost, "/web/session", ip, "Bearer amb_nope"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a minute later: %d %s", rec.Code, rec.Body)
	}
}

const csp = "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func TestAssets(t *testing.T) {
	t.Parallel()
	c := &clock{t: time.Now()}
	notBuilt := newHandler(t, fstest.MapFS{".gitkeep": {}}, c)
	built := newHandler(t, fstest.MapFS{
		".gitkeep":         {},
		"index.html":       {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app-1.js":  {Data: []byte("console.log(1)")},
		"assets/app-1.css": {Data: []byte("body{}")},
	}, c)
	tests := []struct {
		name               string
		h                  http.Handler
		path               string
		wantCode           int
		wantCache, wantHas string
	}{
		{"fallback page", notBuilt, "/web/", 200, "no-cache", "The web GUI was not built; run the GUI build."},
		{"built index", built, "/web/", 200, "no-cache", "<title>app</title>"},
		{"gitkeep", notBuilt, "/web/.gitkeep", 404, "", ""},
		{"hashed bundle", built, "/web/assets/app-1.js", 200, "public, max-age=31536000, immutable", "console.log(1)"},
		{"index by name", built, "/web/index.html", 404, "", ""},
		{"unknown path", built, "/web/unknown", 404, "", ""},
		{"missing asset", built, "/web/assets/nope.js", 404, "", ""},
		{"asset directory", built, "/web/assets/", 404, "", ""},
		{"outside /web", built, "/", 404, "", ""},
		{"session", built, "/web/session", 401, "no-store", "session required"},
		{"api", built, "/web/api/state", 401, "no-store", "session required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := serve(tc.h, http.MethodGet, tc.path, "192.0.2.1:1", "")
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantHas) {
				t.Fatalf("GET %s = %d %q", tc.path, rec.Code, rec.Body)
			}
			hd := rec.Header()
			if hd.Get("Content-Security-Policy") != csp || hd.Get("X-Content-Type-Options") != "nosniff" ||
				hd.Get("Referrer-Policy") != "no-referrer" {
				t.Fatalf("security headers %v", hd)
			}
			if tc.wantCache != "" && hd.Get("Cache-Control") != tc.wantCache {
				t.Fatalf("Cache-Control = %q, want %q", hd.Get("Cache-Control"), tc.wantCache)
			}
		})
	}
}

func TestFallbackPageIsCSPClean(t *testing.T) {
	t.Parallel()
	rec := serve(newHandler(t, fstest.MapFS{}, &clock{t: time.Now()}), http.MethodGet, "/web/", "192.0.2.1:1", "")
	body := strings.ToLower(rec.Body.String())
	for _, banned := range []string{"<script", "<style", "style=", " on"} {
		if strings.Contains(body, banned) {
			t.Fatalf("fallback page contains %q: %s", banned, body)
		}
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("Content-Type %q", rec.Header().Get("Content-Type"))
	}
}

func TestEmbeddedDist(t *testing.T) {
	t.Parallel()
	dist, err := web.Dist()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(dist, ".gitkeep"); err != nil {
		t.Fatalf("embedded dist lacks .gitkeep: %v", err)
	}
}
