package inbox

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/broker"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

const waitTimeout = 5 * time.Second

// testNow is deliberately not UTC so tests prove CreatedAt is normalised.
func testNow() time.Time {
	return time.Date(2026, 10, 25, 9, 30, 0, 123456789, time.FixedZone("CEST", 2*60*60))
}

// world is a seeded inbox: leader "lead" and member agent "worker" in spaces
// "alpha" and "beta", with entries a1, b1, l1, a2, l2 (named by body).
type world struct {
	svc        *Service
	pub        *broker.Memory
	db         *sql.DB
	agents     map[string]int64
	spaces     map[string]int64
	principals map[string]auth.Principal
	entries    map[string]Entry
}

func newWorld(t *testing.T) *world {
	t.Helper()
	db := dbtest.Open(t)
	pub := broker.NewMemory(16)
	w := &world{svc: NewService(db, pub, testNow), pub: pub, db: db, entries: map[string]Entry{}}
	const lead, worker, ops, admin, alpha, beta = 1, 2, 3, 4, 1, 2
	dbtest.SeedAgent(t, db, lead, "lead", "leader")
	dbtest.SeedAgent(t, db, worker, "worker", "member")
	dbtest.SeedAgent(t, db, ops, "ops", "admin")
	dbtest.SeedAgent(t, db, admin, "admin", "admin")
	dbtest.SeedSpace(t, db, alpha, "alpha", lead)
	dbtest.SeedSpace(t, db, beta, "beta", lead)
	w.agents = map[string]int64{"lead": lead, "worker": worker, "ops": ops, "admin": admin}
	w.spaces = map[string]int64{"alpha": alpha, "beta": beta}
	w.principals = map[string]auth.Principal{
		"admin":   {Kind: auth.Admin, AgentID: admin, AgentName: "admin", Bootstrap: true},
		"ops":     {Kind: auth.Admin, AgentID: ops, AgentName: "ops"},
		"leader":  {Kind: auth.Leader, AgentID: lead, AgentName: "lead"},
		"memberA": {Kind: auth.Member, AgentID: worker, AgentName: "worker", SpaceID: w.spaces["alpha"]},
		"memberB": {Kind: auth.Member, AgentID: worker, AgentName: "worker", SpaceID: w.spaces["beta"]},
	}
	for _, s := range [][3]string{
		{"worker", "alpha", "a1"}, {"worker", "beta", "b1"}, {"lead", "alpha", "l1"},
		{"worker", "alpha", "a2"}, {"lead", "beta", "l2"},
	} {
		w.insert(t, s[0], s[1], s[2])
	}
	return w
}

// entry builds an unsaved direct message for agent in space.
func (w *world) entry(agent, space, body string) Entry {
	return Entry{
		AgentID: w.agents[agent],
		SpaceID: w.spaces[space],
		Space:   space,
		Type:    DirectMessage,
		From:    "someone",
		Body:    body,
	}
}

// insert stores one entry and records it by body.
func (w *world) insert(t *testing.T, agent, space, body string) Entry {
	t.Helper()
	got, err := w.svc.insert(t.Context(), w.db, []Entry{w.entry(agent, space, body)})
	if err != nil {
		t.Fatalf("Insert %s: %v", body, err)
	}
	w.entries[body] = got[0]
	return got[0]
}

// send stores one entry and publishes it, as producers do after commit.
func (w *world) send(t *testing.T, agent, space, body string) {
	t.Helper()
	w.svc.publish([]Entry{w.insert(t, agent, space, body)})
}

// read returns the bodies visible to the named principal.
func (w *world) read(t *testing.T, who string) []string {
	t.Helper()
	es, err := w.svc.Read(t.Context(), w.principals[who], 0, 0)
	if err != nil {
		t.Fatalf("Read as %s: %v", who, err)
	}
	return bodies(es)
}

func bodies(es []Entry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Body)
	}
	return out
}

func TestInsert(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	base := Entry{AgentID: w.agents["worker"], SpaceID: w.spaces["alpha"], Space: "alpha", From: "lead"}
	post, closed := base, base
	post.Type, post.Critical, post.Body = ThreadPost, true, "hello"
	post.Blackboard, post.ThreadID, post.ThreadTitle = "plan", 7, "design"
	closed.Type, closed.Topic = TopicClosed, "news"
	in := []Entry{post, closed}
	wantClass := []Class{ClassInterrupt, ClassDeferred}
	tx, err := w.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	got, err := w.svc.insert(t.Context(), tx, in)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	lastSeeded := w.entries["l2"].ID
	for i := range in {
		want := in[i]
		want.ID, want.CreatedAt, want.Class = got[i].ID, "2026-10-25T07:30:00.123456789Z", wantClass[i]
		if got[i].ID <= lastSeeded || (i > 0 && got[i].ID <= got[i-1].ID) {
			t.Fatalf("entry %d id = %d, want increasing ids after %d", i, got[i].ID, lastSeeded)
		}
		if !reflect.DeepEqual(got[i], want) {
			t.Fatalf("Insert()[%d] = %+v, want %+v", i, got[i], want)
		}
	}
	read, err := w.svc.Read(t.Context(), w.principals["memberA"], w.entries["a2"].ID, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(read, got) {
		t.Fatalf("Read() = %+v, want %+v", read, got)
	}
}

func TestRead(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	tests := []struct {
		name    string
		who     string
		after   string // body of the cursor entry; "" means 0
		limit   int
		want    []string
		wantErr apperr.Kind
	}{
		{name: "member sees only its space", who: "memberA", want: []string{"a1", "a2"}},
		{name: "member of the other space", who: "memberB", want: []string{"b1"}},
		{name: "leader sees its entries in every space", who: "leader", want: []string{"l1", "l2"}},
		{name: "after skips older entries", who: "memberA", after: "a1", want: []string{"a2"}},
		{name: "limit caps the page", who: "leader", limit: 1, want: []string{"l1"}},
		{name: "max limit", who: "leader", limit: 500, want: []string{"l1", "l2"}},
		{name: "limit above max", who: "leader", limit: 501, wantErr: apperr.Invalid},
		{name: "negative limit", who: "leader", limit: -1, wantErr: apperr.Invalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := w.svc.Read(t.Context(), w.principals[tc.who], w.entries[tc.after].ID, tc.limit)
			if tc.wantErr != "" {
				apperrtest.WantKind(t, err, tc.wantErr, "")
				return
			}
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if !slices.Equal(bodies(got), tc.want) {
				t.Fatalf("Read() = %v, want %v", bodies(got), tc.want)
			}
		})
	}
}

func TestReadDefaultLimit(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	es := make([]Entry, 0, 101)
	for i := range 101 {
		es = append(es, w.entry("lead", "alpha", fmt.Sprintf("bulk%d", i)))
	}
	if _, err := w.svc.insert(t.Context(), w.db, es); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := w.svc.Read(t.Context(), w.principals["leader"], 0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("Read(limit 0) returned %d entries, want 100", len(got))
	}
}

func TestAck(t *testing.T) {
	t.Parallel()
	type ack struct {
		who   string
		upTo  string // body of the entry whose id is acked up to
		count int64
	}
	tests := []struct {
		name     string
		acks     []ack
		wantLeft map[string][]string
	}{
		{
			name:     "member acks only its space",
			acks:     []ack{{"memberA", "l2", 2}},
			wantLeft: map[string][]string{"memberA": nil, "memberB": {"b1"}, "leader": {"l1", "l2"}},
		},
		{
			name:     "partial ack keeps later entries",
			acks:     []ack{{"memberA", "a1", 1}},
			wantLeft: map[string][]string{"memberA": {"a2"}, "memberB": {"b1"}, "leader": {"l1", "l2"}},
		},
		{
			name:     "ack is idempotent",
			acks:     []ack{{"memberB", "l2", 1}, {"memberB", "l2", 0}},
			wantLeft: map[string][]string{"memberA": {"a1", "a2"}, "memberB": nil, "leader": {"l1", "l2"}},
		},
		{
			name:     "leader acks only its own entries",
			acks:     []ack{{"leader", "l2", 2}},
			wantLeft: map[string][]string{"memberA": {"a1", "a2"}, "memberB": {"b1"}, "leader": nil},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			for _, a := range tc.acks {
				got, err := w.svc.Ack(t.Context(), w.principals[a.who], w.entries[a.upTo].ID)
				if err != nil {
					t.Fatalf("Ack as %s: %v", a.who, err)
				}
				if got != a.count {
					t.Fatalf("Ack as %s up to %s = %d, want %d", a.who, a.upTo, got, a.count)
				}
			}
			for who, want := range tc.wantLeft {
				if got := w.read(t, who); !slices.Equal(got, want) {
					t.Fatalf("%s inbox after ack = %v, want %v", who, got, want)
				}
			}
		})
	}
}

func TestAdminInbox(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.insert(t, "ops", "alpha", "a")
	b := w.insert(t, "ops", "beta", "b")
	if got := w.read(t, "ops"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("admin inbox = %v, want [a b]", got)
	}
	n, err := w.svc.Ack(t.Context(), w.principals["ops"], b.ID)
	if err != nil || n != 2 {
		t.Fatalf("Ack = %d, %v; want 2", n, err)
	}
}

func TestPublishSignalsEachAgentOnce(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	worker, cancelW := w.pub.Subscribe(topic(w.agents["worker"]))
	t.Cleanup(cancelW)
	lead, cancelL := w.pub.Subscribe(topic(w.agents["lead"]))
	t.Cleanup(cancelL)
	w.svc.publish([]Entry{w.entries["a1"], w.entries["b1"], w.entries["l1"], w.entries["a2"]})
	for name, live := range map[string]<-chan []byte{"worker": worker, "lead": lead} {
		if got := signals(t, live); got != 1 {
			t.Fatalf("%s got %d signals, want 1", name, got)
		}
	}
}

// signals waits for one empty wake-up on live, then counts any extra queued ones.
func signals(t *testing.T, live <-chan []byte) int {
	t.Helper()
	select {
	case payload := <-live:
		if payload != nil {
			t.Fatalf("payload = %q, want nil", payload)
		}
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for wake-up signal")
	}
	n := 1
	for {
		select {
		case <-live:
			n++
		case <-time.After(50 * time.Millisecond):
			return n
		}
	}
}

func TestDeleteSpaceEntries(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	if err := w.svc.DeleteSpaceEntries(t.Context(), w.db, w.agents["worker"], w.spaces["alpha"]); err != nil {
		t.Fatalf("DeleteSpaceEntries: %v", err)
	}
	want := map[string][]string{"memberA": nil, "memberB": {"b1"}, "leader": {"l1", "l2"}}
	for who, bodies := range want {
		if got := w.read(t, who); !slices.Equal(got, bodies) {
			t.Fatalf("%s inbox = %v, want %v", who, got, bodies)
		}
	}
}
