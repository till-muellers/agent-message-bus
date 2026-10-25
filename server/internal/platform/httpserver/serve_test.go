package httpserver

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	tests := []struct {
		name        string
		cancel      bool
		closeListen bool
		wantErr     string
		wantCancel  bool
	}{
		{name: "graceful shutdown", cancel: true, wantCancel: true},
		{name: "closed listener", closeListen: true, wantErr: "serve:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			if tc.closeListen {
				_ = ln.Close()
			}
			called := false
			srv := &http.Server{Handler: http.NotFoundHandler()}
			err = Serve(ctx, srv, ln, func() { called = true })
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
