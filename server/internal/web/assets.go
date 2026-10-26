package web

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/httpx"
)

const (
	contentSecurityPolicy = "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	cacheRevalidate       = "no-cache"
	cacheImmutable        = "public, max-age=31536000, immutable"
	cacheNever            = "no-store"
)

// fallbackPage answers GET /web/ when the GUI was not built. Static HTML without inline
// script or style, so it passes contentSecurityPolicy.
const fallbackPage = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>agent-message-bus</title></head>
<body><p>The web GUI was not built; run the GUI build.</p></body>
</html>
`

// Headers sets the security headers of every GUI response.
func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// noStore keeps responses that carry session data or bus state out of every cache.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheNever)
		next.ServeHTTP(w, r)
	})
}

// assets serves the GUI bundle: index.html at /web/ and the hashed files under /web/assets/.
// Nothing else of the bundle (such as .gitkeep) is reachable.
type assets struct{ dist fs.FS }

// index serves GET /web/ (index.html, or fallbackPage when not built); any other path
// that falls through to the /web/ pattern is 404.
func (a assets) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web/" {
		http.NotFound(w, r)
		return
	}
	page, err := fs.ReadFile(a.dist, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		page, err = []byte(fallbackPage), nil
	}
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("read index.html: %w", err))
		return
	}
	w.Header().Set("Cache-Control", cacheRevalidate)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

// file serves GET /web/assets/<name> from the bundle; directories and missing files are 404.
func (a assets) file(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/web/")
	st, err := fs.Stat(a.dist, name)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(a.dist, name)
	if err != nil {
		httpx.WriteError(w, fmt.Errorf("read %s: %w", name, err))
		return
	}
	w.Header().Set("Cache-Control", cacheImmutable)
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(data))
}
