package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		cancel     bool
		wantErr    string
		wantCancel bool
	}{
		{name: "graceful shutdown", addr: "127.0.0.1:0", cancel: true, wantCancel: true},
		{name: "unusable addr", addr: "bad:addr:x", wantErr: "serve:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			called := false
			srv := &http.Server{Addr: tc.addr, Handler: http.NotFoundHandler()}
			err := Serve(ctx, srv, func() { called = true })
			if tc.wantErr == "" && err != nil {
				t.Fatalf("Serve() error = %v, want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), tc.wantErr)) {
				t.Fatalf("Serve() error = %v, want prefix %q", err, tc.wantErr)
			}
			if called != tc.wantCancel {
				t.Fatalf("cancelStreams called = %v, want %v", called, tc.wantCancel)
			}
		})
	}
}
