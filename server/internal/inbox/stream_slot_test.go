package inbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"
)

// errorBody is the JSON error envelope httpx.WriteError writes.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// wantConflict asserts resp is the 409 a second stream of agent gets.
func wantConflict(t *testing.T, resp *http.Response, agent string) {
	t.Helper()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading error body: %v", err)
	}
	var body errorBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decoding error body %q: %v", raw, err)
	}
	want := "agent " + agent + " already has an active inbox stream"
	if body.Error.Code != "conflict" || body.Error.Message != want {
		t.Fatalf("error = %+v, want conflict %q", body.Error, want)
	}
}

// eventuallyOpen retries opening who's stream until it is accepted; the
// server frees a disconnected stream's slot only once it notices the drop.
func eventuallyOpen(t *testing.T, w *world, who string) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		ctx, cancel := context.WithCancel(t.Context())
		resp := openStream(ctx, t.Context(), t, w, who)
		status := resp.StatusCode
		cancel()
		if status == http.StatusOK {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream still rejected with %d after %v", status, waitTimeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStreamOneSlotPerAgent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, first, second string
		conflict            string // agent name in the 409; "" means accepted
	}{
		{"same agent in another space is rejected", "memberA", "memberB", "worker"},
		{"same principal twice is rejected", "leader", "leader", "lead"},
		{"member does not block a leader", "memberA", "leader", ""},
		{"leader does not block a member", "leader", "memberA", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			startStream(t.Context(), t, w, tc.first)
			if tc.conflict == "" {
				startStream(t.Context(), t, w, tc.second)
				return
			}
			wantConflict(t, openStream(t.Context(), t.Context(), t, w, tc.second), tc.conflict)
		})
	}
}

func TestStreamRejectedSecondKeepsFirstDelivering(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.svc.heartbeat = time.Millisecond
	first := startStream(t.Context(), t, w, "memberA")
	if got := first.entries(2); !slices.Equal(got, []string{"a1", "a2"}) {
		t.Fatalf("backlog streamed %v, want [a1 a2]", got)
	}
	wantConflict(t, openStream(t.Context(), t.Context(), t, w, "memberA"), "worker")
	first.expectPing()
	w.send(t, "worker", "alpha", "a3")
	if got := first.entries(1); !slices.Equal(got, []string{"a3"}) {
		t.Fatalf("live streamed %v, want [a3]", got)
	}
}

func TestStreamSlotReleasedOnDisconnect(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx, cancel := context.WithCancel(t.Context())
	if resp := openStream(ctx, t.Context(), t, w, "memberA"); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	cancel()
	eventuallyOpen(t, w, "memberA")
}

func TestStreamSlotReleasedOnRootCancel(t *testing.T) {
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
	startStream(t.Context(), t, w, "memberB")
}
