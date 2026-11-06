package inbox

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
)

// TestStreamSurvivesSignalsDuringALongDrain is a fast reader under a sender burst: wake-up
// signals keep arriving while one drain sends page after page. The world's broker holds
// 16 signals; the test publishes 8 per page, so only a drain that discards the queued
// signals after each page keeps its subscription.
func TestStreamSurvivesSignalsDuringALongDrain(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	const pages, perPage = 6, 8
	backlog := make([]Entry, 0, pages*MaxLimit)
	body := strings.Repeat("x", 1024)
	for i := range pages * MaxLimit {
		backlog = append(backlog, w.entry("worker", "alpha", fmt.Sprintf("%d %s", i, body)))
	}
	err := db.InTx(t.Context(), w.db, func(tx *sql.Tx) error {
		_, err := w.svc.insert(t.Context(), tx, backlog)
		return err
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	stream := startStream(t.Context(), t, w, "memberA")
	for i := range len(backlog) + 2 { // + the seeded a1 and a2
		stream.nextEntry()
		if i%MaxLimit == MaxLimit/5 {
			for range perPage {
				w.pub.Publish(topic(w.agents["worker"]), nil)
			}
		}
	}
	w.send(t, "worker", "alpha", "after the burst")
	if got := stream.nextEntry().Body; got != "after the burst" {
		t.Fatalf("entry after the burst = %q", got)
	}
}
