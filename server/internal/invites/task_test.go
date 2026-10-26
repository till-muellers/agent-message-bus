package invites

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/domain"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/apperr/apperrtest"
	"github.com/till-muellers/agent-message-bus/server/internal/platform/db/dbtest"
)

// storedInviter returns the invited_by of the only stored invite.
func storedInviter(t *testing.T, e env) string {
	t.Helper()
	var by string
	if err := e.db.QueryRowContext(context.Background(), `SELECT invited_by FROM invites`).Scan(&by); err != nil {
		t.Fatalf("reading invited_by: %v", err)
	}
	return by
}

// memberEntries reads the inbox of the member that accepted a, as "from:body" strings.
func memberEntries(t *testing.T, e env, a Admission) []string {
	t.Helper()
	p, err := e.auth.Authenticate(context.Background(), a.Token)
	if err != nil {
		t.Fatalf("authenticating %s: %v", a.Name, err)
	}
	es, err := e.ib.Read(context.Background(), p, 0, 500)
	if err != nil {
		t.Fatalf("reading inbox of %s: %v", a.Name, err)
	}
	out := []string{}
	for _, en := range es {
		if en.Type != inbox.DirectMessage || en.Critical || en.Space != "demo" {
			t.Fatalf("entry %+v is not a non-critical direct message in demo", en)
		}
		out = append(out, en.From+":"+en.Body)
	}
	return out
}

func mustAccept(t *testing.T, e env, code string) Admission {
	t.Helper()
	a, err := e.svc.Accept(context.Background(), code)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	return a
}

func TestInviteRecordsInviter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct{ name string }{{"lead"}, {"ops"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			p := e.lead
			if tc.name == "ops" {
				dbtest.SeedAgent(t, e.db, 9, "ops", "admin")
				p = auth.Principal{Kind: auth.Admin, AgentID: 9, AgentName: "ops"}
			}
			task := "fix the bug"
			if _, err := e.svc.Create(ctx, p, "demo", "worker", &task); err != nil {
				t.Fatalf("create: %v", err)
			}
			if got := storedInviter(t, e); got != tc.name {
				t.Fatalf("invited_by = %q, want %q", got, tc.name)
			}
		})
	}
}

func TestAcceptDeliversTask(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, task string }{
		{"short task", "fix the bug"},
		{"task at the limit", strings.Repeat("x", domain.MaxBodyBytes)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			live, cancel := e.pub.Subscribe("inbox/2")
			defer cancel()
			a := mustAccept(t, e, mustCreate(t, e, "worker", &tc.task).Code)
			if got := memberEntries(t, e, a); len(got) != 1 || got[0] != "lead:"+tc.task {
				t.Fatalf("worker inbox has %d entries, want the task from lead", len(got))
			}
			select {
			case <-live:
			case <-time.After(2 * time.Second):
				t.Fatal("no wake-up published for the task entry")
			}
		})
	}
}

func TestAcceptWithoutTaskDeliversNothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := mustAccept(t, e, mustCreate(t, e, "worker", nil).Code)
	if got := memberEntries(t, e, a); len(got) != 0 {
		t.Fatalf("worker inbox = %v, want empty", got)
	}
}

func TestFailedAdmissionKeepsCodeNoEntry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	task := "fix the bug"
	inv := mustCreate(t, e, "x", &task)
	dbtest.SeedAgent(t, e.db, 7, "x", "leader")
	_, err := e.svc.Accept(context.Background(), inv.Code)
	apperrtest.WantKind(t, err, apperr.Conflict, "agent x is a leader agent")
	if n := countInvites(t, e); n != 1 {
		t.Fatalf("invites = %d, want the code kept", n)
	}
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM inbox`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("inbox rows = %d, %v; want 0", n, err)
	}
}

func TestTaskFromDepartedInviter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	dbtest.SeedAgent(t, e.db, 2, "lead2", "leader")
	lead2 := auth.Principal{Kind: auth.Leader, AgentID: 2, AgentName: "lead2"}
	if err := e.spaces.Join(ctx, lead2, "demo"); err != nil {
		t.Fatalf("join: %v", err)
	}
	task := "fix the bug"
	inv, err := e.svc.Create(ctx, lead2, "demo", "worker", &task)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := e.spaces.Leave(ctx, lead2, "demo"); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if got := memberEntries(t, e, mustAccept(t, e, inv.Code)); len(got) != 1 || got[0] != "lead2:fix the bug" {
		t.Fatalf("worker inbox = %v, want the task from lead2", got)
	}
}

func TestReinviteSendsAnotherTask(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	mustAccept(t, e, mustCreate(t, e, "worker", nil).Code)
	task := "second task"
	a := mustAccept(t, e, mustCreate(t, e, "worker", &task).Code)
	if got := memberEntries(t, e, a); len(got) != 1 || got[0] != "lead:second task" {
		t.Fatalf("worker inbox = %v, want the second task", got)
	}
}

func TestAcceptBodyHasNoTask(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	task := "fix the bug"
	status, body := postAccept(t, e, "Bearer "+mustCreate(t, e, "worker", &task).Code)
	if status != http.StatusOK || body.Task != nil || body.Name != "worker" || body.Token == "" {
		t.Fatalf("accept = %d %+v; want no task field", status, body)
	}
}
