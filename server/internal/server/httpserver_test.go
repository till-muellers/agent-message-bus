package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// countingBody records how many bytes were read and whether EOF was reached.
type countingBody struct {
	r    io.Reader
	read int
	eof  bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.read += n
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

func (b *countingBody) Close() error { return nil }

func rejectUnread(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "no", http.StatusUnauthorized)
}

// TestNewHTTPServerDrainsUnreadBodyOnClose: a Connection: close request whose body the
// handler ignored is read to EOF, so the late body never meets a closed socket.
func TestNewHTTPServerDrainsUnreadBodyOnClose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		close    bool
		wantRead int
	}{
		{name: "connection close", close: true, wantRead: 5},
		{name: "keep-alive left to net/http", close: false, wantRead: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &countingBody{r: strings.NewReader("hello")}
			req := httptest.NewRequest(http.MethodPost, "/v1/tools/x", body)
			req.Close = tc.close
			NewHTTPServer(http.HandlerFunc(rejectUnread)).Handler.ServeHTTP(httptest.NewRecorder(), req)
			if body.read != tc.wantRead || body.eof != tc.close {
				t.Fatalf("body read %d bytes, eof %v; want %d, %v", body.read, body.eof, tc.wantRead, tc.close)
			}
		})
	}
}

// TestNewHTTPServerAnswersBeforeLateBody: headers first, the body only after the handler
// answered without reading it; the client still gets the status line instead of a reset.
func TestNewHTTPServerAnswersBeforeLateBody(t *testing.T) {
	t.Parallel()
	answered := make(chan struct{})
	srv := NewHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rejectUnread(w, r)
		close(answered)
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	status, err := lateBodyRequest(ln.Addr().String(), answered)
	if err != nil || status != "HTTP/1.1 401 Unauthorized" {
		t.Fatalf("status line %q, %v; want 401", status, err)
	}
}

func lateBodyRequest(addr string, answered <-chan struct{}) (string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", err
	}
	head := "POST /v1/tools/x HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(conn, head); err != nil {
		return "", err
	}
	<-answered
	if _, err := io.WriteString(conn, "hello"); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	return strings.TrimSpace(line), err
}
