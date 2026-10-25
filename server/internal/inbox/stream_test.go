package inbox

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
)

// frame is one parsed server-sent event; comment holds a ":" line's text.
type frame struct {
	id, event, data, comment string
}

// sseReader yields stream lines to the test goroutine with a timeout per line.
type sseReader struct {
	t     *testing.T
	lines <-chan string
	errc  <-chan error
}

// startStream serves StreamHandler(root) as the named principal and opens the stream.
func startStream(root context.Context, t *testing.T, w *world, who string) *sseReader {
	t.Helper()
	p := w.principals[who]
	h := w.svc.StreamHandler(root)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(rw, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	}))
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	return readSSE(t, resp.Body)
}

func readSSE(t *testing.T, body io.Reader) *sseReader {
	lines := make(chan string)
	errc := make(chan error, 1)
	stop := t.Context().Done()
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(body)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			case <-stop:
				return
			}
		}
		errc <- sc.Err()
	}()
	return &sseReader{t: t, lines: lines, errc: errc}
}

func (r *sseReader) nextLine() (string, bool) {
	r.t.Helper()
	select {
	case line, ok := <-r.lines:
		return line, ok
	case <-time.After(waitTimeout):
		r.t.Fatal("timed out waiting for stream data")
		return "", false
	}
}

func (r *sseReader) nextFrame() frame {
	r.t.Helper()
	var f frame
	for {
		line, ok := r.nextLine()
		if !ok {
			r.t.Fatal("stream closed while waiting for a frame")
		}
		if line == "" {
			return f
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "":
			f.comment = value
		case "id":
			f.id = value
		case "event":
			f.event = value
		case "data":
			f.data = value
		default:
			r.t.Fatalf("unexpected stream line %q", line)
		}
	}
}

// nextEntry returns the next inbox event, skipping heartbeats.
func (r *sseReader) nextEntry() Entry {
	r.t.Helper()
	for {
		f := r.nextFrame()
		if f.comment == "ping" && f.event == "" {
			continue
		}
		if f.event != "inbox" {
			r.t.Fatalf("frame event = %q, want inbox", f.event)
		}
		var e Entry
		if err := json.Unmarshal([]byte(f.data), &e); err != nil {
			r.t.Fatalf("decoding frame data %q: %v", f.data, err)
		}
		if f.id != strconv.FormatInt(e.ID, 10) {
			r.t.Fatalf("frame id = %q, entry id = %d", f.id, e.ID)
		}
		return e
	}
}

// expectClosed waits for the server to end the stream cleanly.
func (r *sseReader) expectClosed() {
	r.t.Helper()
	for {
		if _, ok := r.nextLine(); !ok {
			break
		}
	}
	if err := <-r.errc; err != nil {
		r.t.Fatalf("stream ended with error: %v", err)
	}
}

func TestStreamDelivery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		who  string
		live func(t *testing.T, w *world) // runs after the backlog arrived
		want []string                     // backlog then live, in order
	}{
		{
			name: "backlog then live entry",
			who:  "memberA",
			live: func(t *testing.T, w *world) { w.send(t, "worker", "alpha", "a3") },
			want: []string{"a1", "a2", "a3"},
		},
		{
			name: "live copy of a backlog entry is not repeated",
			who:  "memberA",
			live: func(t *testing.T, w *world) {
				w.svc.Publish([]Entry{w.entries["a2"]})
				w.send(t, "worker", "alpha", "a3")
			},
			want: []string{"a1", "a2", "a3"},
		},
		{
			name: "member skips live entries of other spaces",
			who:  "memberA",
			live: func(t *testing.T, w *world) {
				w.send(t, "worker", "beta", "b2")
				w.send(t, "worker", "alpha", "a3")
			},
			want: []string{"a1", "a2", "a3"},
		},
		{
			name: "leader streams every space",
			who:  "leader",
			live: func(t *testing.T, w *world) { w.send(t, "lead", "beta", "l3") },
			want: []string{"l1", "l2", "l3"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			stream := startStream(t.Context(), t, w, tc.who)
			backlog := len(tc.want) - 1
			got := make([]string, 0, len(tc.want))
			for range backlog {
				got = append(got, stream.nextEntry().Body)
			}
			tc.live(t, w)
			got = append(got, stream.nextEntry().Body)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("streamed %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamClosesOnRootCancel(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	root, cancel := context.WithCancel(t.Context())
	stream := startStream(root, t, w, "memberB")
	if got := stream.nextEntry().Body; got != "b1" {
		t.Fatalf("backlog entry = %q, want b1", got)
	}
	cancel()
	stream.expectClosed()
}

func TestStreamHeartbeat(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Millisecond
	stream := startStream(t.Context(), t, w, "memberB")
	if got := stream.nextEntry().Body; got != "b1" {
		t.Fatalf("backlog entry = %q, want b1", got)
	}
	if f := stream.nextFrame(); f != (frame{comment: "ping"}) {
		t.Fatalf("frame after backlog = %+v, want ping comment", f)
	}
}

func TestStreamAdminForbidden(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/inbox/stream", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), w.principals["admin"]))
	rec := httptest.NewRecorder()
	w.svc.StreamHandler(t.Context()).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != "forbidden" {
		t.Fatalf("error code = %q, want forbidden", body.Error.Code)
	}
}
