package web_test

import (
	"net/http"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// Concurrent failed sign-ins from one IP get at most 5 token evaluations (401); the
// rest answer 429, however the requests interleave.
func TestSignInRateLimitHoldsUnderConcurrency(t *testing.T) {
	t.Parallel()
	c := &clock{t: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	h := newHandler(t, fstest.MapFS{}, c)
	const n = 150
	codes := make(chan int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			<-start
			codes <- serve(h, http.MethodPost, "/web/session", "192.0.2.7:4000", "Bearer amb_nope").Code
		})
	}
	close(start)
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
