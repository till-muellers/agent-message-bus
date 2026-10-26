package overview_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
)

const (
	demoParticipants = `[{"name":"lead","role":"leader"},{"name":"peer","role":"member"},{"name":"worker","role":"member"}]`
	allOnly          = `[{"name":"all","createdBy":"lead","subscribed":true}]`
)

// overviewCase is one bus_space_overview call; want holds the expected JSON of each
// top-level result key it checks (lastPostAt values are compared as "*").
type overviewCase struct {
	name   string
	setup  func(t *testing.T, w *world)
	caller func(w *world) auth.Principal
	space  string
	want   map[string]string
}

func TestOverview(t *testing.T) {
	t.Parallel()
	for _, tc := range overviewCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			if tc.setup != nil {
				tc.setup(t, w)
			}
			res, err := w.overview(t.Context(), t, tc.caller(w), fmt.Sprintf(`{"space":%q}`, tc.space))
			w.must(t, err)
			got := normalize(t, res)
			if got["space"] != tc.space {
				t.Fatalf("space = %v, want %s", got["space"], tc.space)
			}
			for key, want := range tc.want {
				assertJSON(t, key, got[key], want)
			}
		})
	}
}

func overviewCases() []overviewCase {
	lead := func(w *world) auth.Principal { return w.lead }
	worker := func(w *world) auth.Principal { return w.wkr }
	peer := func(w *world) auth.Principal { return w.peer }
	ops := func(w *world) auth.Principal { return w.ops }
	return []overviewCase{
		{name: "member view", setup: memberViewSetup, caller: worker, space: "demo", want: map[string]string{
			"you":          `{"name":"worker","role":"member"}`,
			"participants": demoParticipants,
			"topics":       `[{"name":"all","createdBy":"lead","subscribed":true},{"name":"build","createdBy":"lead","subscribed":true}]`,
			"blackboards":  `[{"name":"plan","owner":"lead","threads":1}]`,
			"recentThreads": `[{"id":1,"blackboard":"plan","title":"design","createdBy":"lead","posts":1,` +
				`"lastPostAt":"*","subscribed":false}]`,
		}},
		{name: "leader view", setup: memberViewSetup, caller: lead, space: "demo", want: map[string]string{
			"you":          `{"name":"lead","role":"leader"}`,
			"participants": demoParticipants,
			"topics":       `[{"name":"all","createdBy":"lead","subscribed":true},{"name":"build","createdBy":"lead","subscribed":true}]`,
			"recentThreads": `[{"id":1,"blackboard":"plan","title":"design","createdBy":"lead","posts":1,` +
				`"lastPostAt":"*","subscribed":true}]`,
		}},
		{name: "joined admin has role admin, listed first", setup: func(t *testing.T, w *world) {
			w.must(t, w.sp.Join(t.Context(), w.ops, "demo"))
		}, caller: ops, space: "demo", want: map[string]string{
			"you":          `{"name":"ops","role":"admin"}`,
			"participants": `[{"name":"ops","role":"admin"},` + demoParticipants[1:],
			"topics":       allOnly,
		}},
		{name: "unjoined admin has no role and no subscriptions", setup: memberViewSetup, caller: ops, space: "demo",
			want: map[string]string{
				"you":          `{"name":"ops","role":null}`,
				"participants": demoParticipants,
				"topics":       `[{"name":"all","createdBy":"lead","subscribed":false},{"name":"build","createdBy":"lead","subscribed":false}]`,
			}},
		{name: "empty space shows empty lists", setup: func(t *testing.T, w *world) {
			w.must(t, w.sp.Create(t.Context(), w.lead, "empty"))
		}, caller: lead, space: "empty", want: map[string]string{
			"you": `{"name":"lead","role":"leader"}`, "participants": `[{"name":"lead","role":"leader"}]`,
			"topics": allOnly, "blackboards": `[]`, "recentThreads": `[]`,
		}},
		{name: "the all topic shows as subscribed for every participant", caller: peer, space: "demo",
			want: map[string]string{"you": `{"name":"peer","role":"member"}`, "topics": allOnly}},
		{name: "closed topics are not shown", setup: func(t *testing.T, w *world) {
			w.must(t, w.msg.CreateTopic(t.Context(), w.lead, "demo", "old"))
			w.must(t, w.msg.CloseTopic(t.Context(), w.lead, "demo", "old"))
		}, caller: lead, space: "demo", want: map[string]string{"topics": allOnly}},
		{name: "at most 10 recent threads across boards, newest post first", setup: recentSetup, caller: lead,
			space: "demo", want: map[string]string{"recentThreads": recentWant()}},
		{name: "thread subscription is the caller's (creator)", setup: subscribedSetup, caller: worker, space: "demo",
			want: map[string]string{"recentThreads": `[{"id":1,"blackboard":"plan","title":"t1","createdBy":"worker",` +
				`"posts":1,"lastPostAt":"*","subscribed":true}]`}},
		{name: "thread subscription is the caller's (other member)", setup: subscribedSetup, caller: peer, space: "demo",
			want: map[string]string{"recentThreads": `[{"id":1,"blackboard":"plan","title":"t1","createdBy":"worker",` +
				`"posts":1,"lastPostAt":"*","subscribed":false}]`}},
	}
}

// memberViewSetup: topic build (worker subscribed), board plan with thread design by lead.
func memberViewSetup(t *testing.T, w *world) {
	w.must(t, w.msg.CreateTopic(t.Context(), w.lead, "demo", "build"))
	w.must(t, w.msg.Subscribe(t.Context(), w.wkr, "demo", "build"))
	w.must(t, w.bb.Create(t.Context(), w.lead, "demo", "plan"))
	w.thread(t, w.lead, "plan", "design")
}

// recentSetup: boards plan and notes, 12 threads by lead alternately on them, then a
// post to the oldest thread.
func recentSetup(t *testing.T, w *world) {
	w.must(t, w.bb.Create(t.Context(), w.lead, "demo", "plan"))
	w.must(t, w.bb.Create(t.Context(), w.wkr, "demo", "notes"))
	for i := 1; i <= 12; i++ {
		w.thread(t, w.lead, recentBoard(i), fmt.Sprintf("t%02d", i))
	}
	_, err := w.bb.AddPost(t.Context(), w.lead, "demo", 1, "bump")
	w.must(t, err)
}

func recentBoard(i int) string {
	if i%2 == 1 {
		return "plan"
	}
	return "notes"
}

// recentWant is thread 1 (bumped), then threads 12 down to 4.
func recentWant() string {
	ids := []int{1, 12, 11, 10, 9, 8, 7, 6, 5, 4}
	rows := make([]string, 0, len(ids))
	for _, id := range ids {
		posts := 1
		if id == 1 {
			posts = 2
		}
		rows = append(rows, fmt.Sprintf(`{"id":%d,"blackboard":%q,"title":"t%02d","createdBy":"lead","posts":%d,`+
			`"lastPostAt":"*","subscribed":true}`, id, recentBoard(id), id, posts))
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func subscribedSetup(t *testing.T, w *world) {
	w.must(t, w.bb.Create(t.Context(), w.lead, "demo", "plan"))
	w.thread(t, w.wkr, "plan", "t1")
}

// normalize encodes res as JSON, checks it has exactly the contract's keys and replaces
// every recentThreads lastPostAt (an RFC 3339 timestamp) with "*".
func normalize(t *testing.T, res any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	keys := []string{"space", "you", "participants", "topics", "blackboards", "recentThreads"}
	if len(got) != len(keys) {
		t.Fatalf("result keys = %s, want %v", raw, keys)
	}
	threads, _ := got["recentThreads"].([]any)
	for _, th := range threads {
		m := th.(map[string]any)
		if _, err := time.Parse(time.RFC3339Nano, fmt.Sprint(m["lastPostAt"])); err != nil {
			t.Fatalf("lastPostAt %v: %v", m["lastPostAt"], err)
		}
		m["lastPostAt"] = "*"
	}
	return got
}

func assertJSON(t *testing.T, key string, got any, want string) {
	t.Helper()
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad want for %s: %v", key, err)
	}
	if !reflect.DeepEqual(got, w) {
		g, _ := json.Marshal(got)
		t.Fatalf("%s = %s\nwant %s", key, g, want)
	}
}

func TestOverviewGate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		caller func(w *world) auth.Principal
		args   string
		kind   apperr.Kind
		msg    string
	}{
		{"unknown space", func(w *world) auth.Principal { return w.lead }, `{"space":"nope"}`,
			apperr.NotFound, "space nope not found"},
		{"member token of another space", func(w *world) auth.Principal { return w.wkr }, `{"space":"beta"}`,
			apperr.Forbidden, "token is not valid for space beta"},
		{"leader that does not participate", func(w *world) auth.Principal { return w.lead2 }, `{"space":"demo"}`,
			apperr.Forbidden, "not a participant of space demo"},
		{"space is required", func(w *world) auth.Principal { return w.wkr }, `{}`,
			apperr.Invalid, "space is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			_, err := w.overview(t.Context(), t, tc.caller(w), tc.args)
			apperrtest.WantKind(t, err, tc.kind, tc.msg)
		})
	}
}

func TestOverviewHasNoSideEffects(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	memberViewSetup(t, w)
	w.must(t, w.msg.SendDirect(t.Context(), w.lead, "demo", "worker", "hello", false))
	before := rowCounts(t, w)
	for _, p := range []auth.Principal{w.wkr, w.lead, w.ops} {
		_, err := w.overview(t.Context(), t, p, `{"space":"demo"}`)
		w.must(t, err)
	}
	if after := rowCounts(t, w); !reflect.DeepEqual(after, before) {
		t.Fatalf("row counts changed:\nbefore %v\nafter  %v", before, after)
	}
}

// rowCounts returns the row count of every table in the database.
func rowCounts(t *testing.T, w *world) map[string]int {
	t.Helper()
	rows, err := w.conn.QueryContext(t.Context(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	w.must(t, err)
	var names []string
	for rows.Next() {
		var n string
		w.must(t, rows.Scan(&n))
		names = append(names, n)
	}
	w.must(t, rows.Err())
	w.must(t, rows.Close())
	counts := make(map[string]int, len(names))
	for _, n := range names {
		var c int
		w.must(t, w.conn.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+n).Scan(&c))
		counts[n] = c
	}
	if counts["inbox"] == 0 || counts["topic_subscriptions"] == 0 || counts["thread_subscriptions"] == 0 {
		t.Fatalf("fixture lacks rows to watch: %v", counts)
	}
	return counts
}
