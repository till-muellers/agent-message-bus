package web_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/web"
)

// gatedAuth rejects every token, but only once gate closes; entered reports each call.
type gatedAuth struct {
	entered chan struct{}
	gate    chan struct{}
}

func (g gatedAuth) AuthenticateHash(ctx context.Context, _ string) (auth.Principal, error) {
	g.entered <- struct{}{}
	select {
	case <-g.gate:
	case <-ctx.Done():
	}
	return auth.Principal{}, apperr.New(apperr.Unauthenticated, "invalid token")
}

// Concurrent failed sign-ins from one IP get at most 5 token checks (401); the rest answer
// 429. Every token check is held open until all requests are admitted or refused, so a
// check-then-count limiter would admit them all.
func TestSignInRateLimitHoldsUnderConcurrency(t *testing.T) {
	t.Parallel()
	const n = 150
	a := gatedAuth{entered: make(chan struct{}, n), gate: make(chan struct{})}
	mux := http.NewServeMux()
	web.New(web.Deps{Auth: a, Dist: fstest.MapFS{}, Now: time.Now}).Mount(mux)
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			codes <- serve(mux, http.MethodPost, "/web/session", "192.0.2.7:4000", "Bearer amb_nope").Code
		})
	}
	awaitAdmittedOrRefused(t, a.entered, codes, n)
	close(a.gate)
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[http.StatusUnauthorized] != 5 || counts[http.StatusTooManyRequests] != n-5 {
		t.Fatalf("answers = %v, want 5 × 401 and %d × 429", counts, n-5)
	}
}

// awaitAdmittedOrRefused waits until n requests either reached the token check or
// answered; it consumes only the entered signals.
func awaitAdmittedOrRefused(t *testing.T, entered <-chan struct{}, codes chan int, n int) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	admitted := 0
	for admitted+len(codes) < n {
		select {
		case <-entered:
			admitted++
		case <-deadline:
			t.Fatalf("%d admitted and %d answered of %d", admitted, len(codes), n)
		case <-time.After(time.Millisecond):
		}
	}
}
