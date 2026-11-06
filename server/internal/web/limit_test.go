package web

import (
	"testing"
	"time"
)

func TestLimiterReserve(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return at })
	for i := range maxFailures {
		if _, ok := l.reserve("ip"); !ok {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	if _, ok := l.reserve("ip"); ok {
		t.Fatal("attempt 6 admitted while 5 are counted")
	}
	if _, ok := l.reserve("other"); !ok {
		t.Fatal("another IP refused")
	}
	at = at.Add(failureWindow)
	release, ok := l.reserve("ip")
	if !ok {
		t.Fatal("refused a window later")
	}
	release()
	for i := range maxFailures {
		if _, ok := l.reserve("ip"); !ok {
			t.Fatalf("after a release, attempt %d refused", i+1)
		}
	}
}
