package server_test

import (
	"net/http"
	"testing"
	"time"
)

// participantPresence is one bus_list_participants row's presence fields.
type participantPresence struct {
	Name         string
	Connected    bool
	OfflineSince string
}

// offlineSinceOf returns agent's offlineSince in alpha ("" while connected).
func offlineSinceOf(t *testing.T, w mailWorld, agent string) string {
	t.Helper()
	var rows []participantPresence
	callTool(t, w.srv, w.lead, "bus_list_participants", `{"space":"alpha"}`, &rows)
	for _, r := range rows {
		if r.Name == agent {
			if r.Connected == (r.OfflineSince != "") {
				t.Fatalf("%s: connected %v with offlineSince %q", agent, r.Connected, r.OfflineSince)
			}
			return r.OfflineSince
		}
	}
	t.Fatalf("%s is not a participant", agent)
	return ""
}

func TestOfflineSinceAfterStreamEnds(t *testing.T) {
	t.Parallel()
	w := newMailWorld(t)
	started := parseTime(t, offlineSinceOf(t, w, "worker"))
	status, _, closeStream := openStream(t, w.srv, w.worker)
	if status != http.StatusOK {
		t.Fatalf("stream = %d", status)
	}
	untilConnected(t, w, "worker")
	closeStream()
	deadline := time.Now().Add(5 * time.Second)
	for offlineSinceOf(t, w, "worker") == "" {
		if time.Now().After(deadline) {
			t.Fatal("worker still connected after its stream ended")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ended := parseTime(t, offlineSinceOf(t, w, "worker")); !ended.After(started) {
		t.Fatalf("offlineSince %v is not after the start time %v", ended, started)
	}
	if lead := offlineSinceOf(t, w, "lead"); !parseTime(t, lead).Equal(started) {
		t.Fatalf("lead's offlineSince = %s, want the start time %v", lead, started)
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("offlineSince %q: %v", s, err)
	}
	return v
}
