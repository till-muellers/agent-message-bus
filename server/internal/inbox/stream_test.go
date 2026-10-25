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

// nextLine waits for one line; deadline bounds the caller's whole wait, so a
// steady heartbeat cannot keep a test waiting forever.
func (r *sseReader) nextLine(deadline <-chan time.Time) (string, bool) {
	r.t.Helper()
	select {
	case line, ok := <-r.lines:
		return line, ok
	case <-deadline:
		r.t.Fatal("timed out waiting for stream data")
		return "", false
	}
}

func (r *sseReader) nextFrame(deadline <-chan time.Time) frame {
	r.t.Helper()
	var f frame
	for {
		line, ok := r.nextLine(deadline)
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

// nextEntry returns the next inbox event within waitTimeout, skipping heartbeats.
func (r *sseReader) nextEntry() Entry {
	r.t.Helper()
	deadline := time.After(waitTimeout)
	for {
		f := r.nextFrame(deadline)
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

// entries returns the bodies of the next n inbox events.
func (r *sseReader) entries(n int) []string {
	r.t.Helper()
	got := make([]string, 0, n)
	for range n {
		got = append(got, r.nextEntry().Body)
	}
	return got
}

// expectPing asserts the next frame is a heartbeat. Heartbeats start only after
// the backlog, so a ping proves the stream is waiting for live entries.
func (r *sseReader) expectPing() {
	r.t.Helper()
	if f := r.nextFrame(time.After(waitTimeout)); f != (frame{comment: "ping"}) {
		r.t.Fatalf("frame = %+v, want ping comment", f)
	}
}

// expectClosed waits for the server to end the stream cleanly.
func (r *sseReader) expectClosed() {
	r.t.Helper()
	deadline := time.After(waitTimeout)
	for {
		if _, ok := r.nextLine(deadline); !ok {
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
		name    string
		who     string
		backlog []string
		live    func(t *testing.T, w *world) // runs once the stream is live
		want    []string                     // entries streamed after the backlog
	}{
		{
			name:    "backlog then live entry",
			who:     "memberA",
			backlog: []string{"a1", "a2"},
			live:    func(t *testing.T, w *world) { w.send(t, "worker", "alpha", "a3") },
			want:    []string{"a3"},
		},
		{
			name:    "live copy of a backlog entry is not repeated",
			who:     "memberA",
			backlog: []string{"a1", "a2"},
			live: func(t *testing.T, w *world) {
				w.svc.Publish([]Entry{w.entries["a2"]})
				w.send(t, "worker", "alpha", "a3")
			},
			want: []string{"a3"},
		},
		{
			name:    "member skips live entries of other spaces",
			who:     "memberA",
			backlog: []string{"a1", "a2"},
			live: func(t *testing.T, w *world) {
				w.send(t, "worker", "beta", "b2")
				w.send(t, "worker", "alpha", "a3")
			},
			want: []string{"a3"},
		},
		{
			name:    "leader streams every space",
			who:     "leader",
			backlog: []string{"l1", "l2"},
			live:    func(t *testing.T, w *world) { w.send(t, "lead", "beta", "l3") },
			want:    []string{"l3"},
		},
		{
			name:    "entry published out of order is not lost",
			who:     "memberA",
			backlog: []string{"a1", "a2"},
			live: func(t *testing.T, w *world) {
				a3 := w.insert(t, "worker", "alpha", "a3")
				a4 := w.insert(t, "worker", "alpha", "a4")
				w.svc.Publish([]Entry{a4})
				w.svc.Publish([]Entry{a3})
				w.send(t, "worker", "alpha", "a5")
			},
			want: []string{"a3", "a4", "a5"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			w.svc.heartbeat = time.Millisecond
			stream := startStream(t.Context(), t, w, tc.who)
			if got := stream.entries(len(tc.backlog)); !slices.Equal(got, tc.backlog) {
				t.Fatalf("backlog streamed %v, want %v", got, tc.backlog)
			}
			stream.expectPing()
			tc.live(t, w)
			if got := stream.entries(len(tc.want)); !slices.Equal(got, tc.want) {
				t.Fatalf("live streamed %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamClosesOnRootCancel(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Millisecond
	root, cancel := context.WithCancel(t.Context())
	stream := startStream(root, t, w, "memberB")
	if got := stream.entries(1); !slices.Equal(got, []string{"b1"}) {
		t.Fatalf("backlog streamed %v, want [b1]", got)
	}
	stream.expectPing()
	cancel()
	stream.expectClosed()
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
