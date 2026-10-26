package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
	"github.com/till-muellers/agent-message-bus/server/migrations"
)

// emptyWorld is newWorld with every inbox emptied, so each scope starts empty.
func emptyWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t)
	dbtest.Exec(t, w.db, `DELETE FROM inbox`)
	return w
}

// deliver stores es in one Deliver tx; fail makes the tx fail after producing them.
func (w *world) deliver(t *testing.T, fail bool, es ...Entry) {
	t.Helper()
	boom := errors.New("boom")
	err := w.svc.Deliver(t.Context(), func(*sql.Tx) ([]Entry, error) {
		if fail {
			return es, boom
		}
		return es, nil
	})
	if fail != errors.Is(err, boom) || (!fail && err != nil) {
		t.Fatalf("Deliver err = %v, fail = %v", err, fail)
	}
	if fail {
		return
	}
	stored, rerr := selectEntries(t.Context(), w.db, es[0].AgentID, 0, 0, maxLimit)
	if rerr != nil {
		t.Fatalf("reading delivered entries: %v", rerr)
	}
	for _, e := range stored {
		w.entries[e.Body] = e
	}
}

// sendEach delivers one direct message per body to agent in space, each in its own tx.
func (w *world) sendEach(t *testing.T, agent, space string, bodies ...string) {
	t.Helper()
	for _, b := range bodies {
		w.deliver(t, false, w.entry(agent, space, b))
	}
}

// take returns the notice of the named principal, failing the test on error.
func (w *world) take(t *testing.T, who string) *Notice {
	t.Helper()
	n, ok, err := w.svc.TakeNotice(t.Context(), w.principals[who])
	if err != nil {
		t.Fatalf("TakeNotice as %s: %v", who, err)
	}
	if !ok {
		return nil
	}
	return &n
}

func (w *world) ack(t *testing.T, who, body string) {
	t.Helper()
	if _, err := w.svc.Ack(t.Context(), w.principals[who], w.entries[body].ID); err != nil {
		t.Fatalf("Ack as %s up to %s: %v", who, body, err)
	}
}

func critical(e Entry) Entry {
	e.Critical = true
	return e
}

func TestEdge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		steps func(t *testing.T, w *world)
		who   string
		want  *Notice // nil: no notice is due
	}{
		{"first into empty", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
		}, "memberA", &Notice{Waiting: 1, Spaces: []string{"alpha"}}},
		{"second not", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
			w.take(t, "memberA")
			w.sendEach(t, "worker", "alpha", "a2", "a3")
		}, "memberA", nil},
		{"one tx = one edge", func(t *testing.T, w *world) {
			w.deliver(t, false, w.entry("worker", "alpha", "a1"), w.entry("worker", "alpha", "a2"), w.entry("worker", "alpha", "a3"))
			if n := w.edges(t); n != 1 {
				t.Fatalf("edges = %d, want 1", n)
			}
		}, "memberA", &Notice{Waiting: 3, Spaces: []string{"alpha"}}},
		{"failed tx none", func(t *testing.T, w *world) {
			w.deliver(t, true, w.entry("worker", "alpha", "a1"))
		}, "memberA", nil},
		{"full ack re-arms", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1", "a2")
			w.take(t, "memberA")
			w.ack(t, "memberA", "a2")
			w.sendEach(t, "worker", "alpha", "a3")
		}, "memberA", &Notice{Waiting: 1, Spaces: []string{"alpha"}}},
		{"partial ack does not re-arm", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1", "a2")
			w.take(t, "memberA")
			w.ack(t, "memberA", "a1")
			w.sendEach(t, "worker", "alpha", "a3")
		}, "memberA", nil},
		{"read does not re-arm", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
			w.take(t, "memberA")
			w.read(t, "memberA")
			w.sendEach(t, "worker", "alpha", "a2")
		}, "memberA", nil},
		{"leaving empties scope", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
			w.take(t, "memberA")
			if err := w.svc.DeleteSpaceEntries(t.Context(), w.db, w.agents["worker"], w.spaces["alpha"]); err != nil {
				t.Fatalf("DeleteSpaceEntries: %v", err)
			}
			w.sendEach(t, "worker", "alpha", "a2")
		}, "memberA", &Notice{Waiting: 1, Spaces: []string{"alpha"}}},
		{"member scopes independent", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "beta", "b1")
			w.take(t, "memberB")
			w.sendEach(t, "worker", "alpha", "a1")
		}, "memberA", &Notice{Waiting: 1, Spaces: []string{"alpha"}}},
		{"other space does not notify the member's token", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "beta", "b1")
		}, "memberA", nil},
		{"leader spans spaces", func(t *testing.T, w *world) {
			w.sendEach(t, "lead", "alpha", "l1")
			w.take(t, "leader")
			w.sendEach(t, "lead", "beta", "l2")
		}, "leader", nil},
		{"leader notice lists every space", func(t *testing.T, w *world) {
			w.deliver(t, false, w.entry("lead", "alpha", "l1"), w.entry("lead", "beta", "l2"))
		}, "leader", &Notice{Waiting: 2, Spaces: []string{"alpha", "beta"}}},
		{"admin spans spaces", func(t *testing.T, w *world) {
			w.sendEach(t, "ops", "alpha", "o1")
			w.take(t, "ops")
			w.sendEach(t, "ops", "beta", "o2")
		}, "ops", nil},
		{"per recipient", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
		}, "leader", nil},
		{"critical re-notifies", func(t *testing.T, w *world) {
			w.sendEach(t, "worker", "alpha", "a1")
			w.take(t, "memberA")
			w.deliver(t, false, critical(w.entry("worker", "alpha", "a2")))
		}, "memberA", &Notice{Waiting: 2, Critical: 1, Spaces: []string{"alpha"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := emptyWorld(t)
			tc.steps(t, w)
			got := w.take(t, tc.who)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("notice = %+v, want %+v", got, tc.want)
			}
			if again := w.take(t, tc.who); again != nil {
				t.Fatalf("second notice = %+v, want none (a notice is given once)", again)
			}
		})
	}
}

// edges counts the stored edge entries.
func (w *world) edges(t *testing.T) int {
	t.Helper()
	var n int
	if err := w.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM inbox WHERE edge = 1`).Scan(&n); err != nil {
		t.Fatalf("counting edges: %v", err)
	}
	return n
}

func TestNoticeSurvivesRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "restart.db")
	worker := auth.Principal{Kind: auth.Member, AgentID: 2, AgentName: "worker", SpaceID: 1}
	svc, conn := openService(t, path)
	dbtest.SeedAgent(t, conn, 1, "lead", "leader")
	dbtest.SeedAgent(t, conn, 2, "worker", "member")
	dbtest.SeedSpace(t, conn, 1, "alpha", 1)
	err := svc.Deliver(t.Context(), func(*sql.Tx) ([]Entry, error) {
		return []Entry{{AgentID: 2, SpaceID: 1, Space: "alpha", Type: DirectMessage, From: "lead", Body: "a1"}}, nil
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}
	svc, _ = openService(t, path)
	n, ok, err := svc.TakeNotice(t.Context(), worker)
	if err != nil || !ok {
		t.Fatalf("TakeNotice after restart = %+v, %v, %v; want one notice", n, ok, err)
	}
	if want := (Notice{Waiting: 1, Spaces: []string{"alpha"}}); !reflect.DeepEqual(n, want) {
		t.Fatalf("notice = %+v, want %+v", n, want)
	}
}

// openService opens and migrates the database at path and builds an inbox on it.
func openService(t *testing.T, path string) (*Service, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(ctx, conn, migrations.FS); err != nil {
		t.Fatalf("migrating %s: %v", path, err)
	}
	return NewService(t.Context(), conn, broker.NewMemory(16), testNow), conn
}

func TestNoticeHasNoBodies(t *testing.T) {
	t.Parallel()
	w := emptyWorld(t)
	const body = "the secret plan"
	w.deliver(t, false, w.entry("worker", "alpha", body), critical(w.entry("worker", "alpha", body)))
	n := w.take(t, "memberA")
	if n == nil {
		t.Fatal("no notice, want one")
	}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), body) {
		t.Fatalf("notice %s carries a message body", raw)
	}
	typ := reflect.TypeFor[Notice]()
	fields := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		fields = append(fields, typ.Field(i).Name)
	}
	if want := []string{"Waiting", "Critical", "Spaces"}; !slices.Equal(fields, want) {
		t.Fatalf("Notice fields = %v, want only counts and spaces %v", fields, want)
	}
}
