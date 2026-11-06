package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// countingBody records how many bytes were read.
type countingBody struct {
	r    io.Reader
	read int
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.read += n
	return n, err
}

func (b *countingBody) Close() error { return nil }

func rejectUnread(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "no", http.StatusUnauthorized)
}

func readAll(w http.ResponseWriter, r *http.Request) {
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// TestNewHTTPServerDrainsUnreadBodyOnClose: the answer to a Connection: close request whose
// body the handler ignored is sent first, then at most maxDrainBytes+1 of the body are read
// (http.MaxBytesReader reads one byte past its limit to detect it).
func TestNewHTTPServerDrainsUnreadBodyOnClose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		close       bool
		handler     http.HandlerFunc
		body        string
		wantRead    int
		wantFlushed bool
	}{
		{name: "unread body", close: true, handler: rejectUnread, body: "hello", wantRead: 5, wantFlushed: true},
		{name: "read body left alone", close: true, handler: readAll, body: "hello", wantRead: 5},
		{name: "keep-alive left to net/http", handler: rejectUnread, body: "hello"},
		{name: "drain capped", close: true, handler: rejectUnread, body: strings.Repeat("x", 2*maxDrainBytes),
			wantRead: maxDrainBytes + 1, wantFlushed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &countingBody{r: strings.NewReader(tc.body)}
			req := httptest.NewRequest(http.MethodPost, "/v1/tools/x", body)
			req.Close = tc.close
			rec := httptest.NewRecorder()
			NewHTTPServer(tc.handler).Handler.ServeHTTP(rec, req)
			if body.read != tc.wantRead || rec.Flushed != tc.wantFlushed {
				t.Fatalf("body read %d bytes, flushed %v; want %d, %v", body.read, rec.Flushed, tc.wantRead, tc.wantFlushed)
			}
		})
	}
}

// TestNewHTTPServerAnswersBeforeLateBody: headers first; the client reads the status line,
// then sends the body and reads the rest of the answer without a connection reset.
func TestNewHTTPServerAnswersBeforeLateBody(t *testing.T) {
	t.Parallel()
	srv := NewHTTPServer(http.HandlerFunc(rejectUnread))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	status, err := lateBodyRequest(ln.Addr().String())
	if err != nil || status != "HTTP/1.1 401 Unauthorized" {
		t.Fatalf("status line %q, %v; want 401 and a clean close", status, err)
	}
}

func lateBodyRequest(addr string) (string, error) {
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
	answer := bufio.NewReader(conn)
	line, err := answer.ReadString('\n')
	if err != nil {
		return "", err
	}
	if _, err := io.WriteString(conn, "hello"); err != nil {
		return "", err
	}
	_, err = io.Copy(io.Discard, answer)
	return strings.TrimSpace(line), err
}
