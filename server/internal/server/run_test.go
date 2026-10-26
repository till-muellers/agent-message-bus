package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/server"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func says(text string) *http.Server {
	return server.NewHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, text)
	}))
}

// get returns the body of GET http://addr/ or the dial error.
func get(addr string) (string, error) {
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	res, err := c.Get("http://" + addr + "/")
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

func TestRunServesBothAndShutsDownBoth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		web, closeWeb      bool
		wantErr            string
		wantStreamsStopped bool
	}{
		{name: "ctx cancel ends both", web: true, wantStreamsStopped: true},
		{name: "failing GUI listener ends the API", web: true, closeWeb: true, wantErr: "web listener: serve:", wantStreamsStopped: true},
		{name: "disabled GUI serves the API only", wantStreamsStopped: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			apiLn := listen(t)
			var web *http.Server
			var webLn net.Listener
			if tc.web {
				web, webLn = says("web"), listen(t)
			}
			if tc.closeWeb {
				_ = webLn.Close()
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stopped atomic.Bool
			done := make(chan error, 1)
			go func() { done <- server.Run(ctx, says("api"), web, apiLn, webLn, func() { stopped.Store(true) }) }()
			if !tc.closeWeb {
				expectServing(t, apiLn.Addr().String(), "api")
				if tc.web {
					expectServing(t, webLn.Addr().String(), "web")
				}
				cancel()
			}
			err := waitRun(t, done)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("Run() = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("Run() = %v, want it to contain %q", err, tc.wantErr)
			}
			if stopped.Load() != tc.wantStreamsStopped {
				t.Fatalf("cancelStreams called = %v, want %v", stopped.Load(), tc.wantStreamsStopped)
			}
			if _, err := get(apiLn.Addr().String()); err == nil {
				t.Fatal("API listener still serves after Run returned")
			}
			if tc.web && !tc.closeWeb {
				if _, err := get(webLn.Addr().String()); err == nil {
					t.Fatal("GUI listener still serves after Run returned")
				}
			}
		})
	}
}

func expectServing(t *testing.T, addr, want string) {
	t.Helper()
	got, err := get(addr)
	if err != nil || got != want {
		t.Fatalf("GET %s = %q, %v; want %q", addr, got, err, want)
	}
}

func waitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}
