package inbox_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/inbox"
)

// fakeNoticer answers TakeNotice with fixed values.
type fakeNoticer struct {
	notice inbox.Notice
	due    bool
	err    error
}

func (f fakeNoticer) TakeNotice(context.Context, auth.Principal) (inbox.Notice, bool, error) {
	return f.notice, f.due, f.err
}

func TestNoticeAfter(t *testing.T) {
	t.Parallel()
	waiting := inbox.Notice{Waiting: 2, Critical: 1, Spaces: []string{"alpha"}}
	cases := []struct {
		name    string
		noticer fakeNoticer
		want    inbox.Notice
		due     bool
	}{
		{"due", fakeNoticer{notice: waiting, due: true}, waiting, true},
		{"nothing new", fakeNoticer{}, inbox.Notice{}, false},
		{"a failed query is swallowed", fakeNoticer{notice: waiting, due: true, err: errors.New("db down")}, inbox.Notice{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n, due := inbox.NoticeAfter(t.Context(), tc.noticer, auth.Principal{AgentID: 7})
			if due != tc.due || !reflect.DeepEqual(n, tc.want) {
				t.Fatalf("NoticeAfter = %+v, %v; want %+v, %v", n, due, tc.want, tc.due)
			}
		})
	}
}

// countingNoticer records how often TakeNotice ran.
type countingNoticer struct{ calls int }

func (c *countingNoticer) TakeNotice(ctx context.Context, _ auth.Principal) (inbox.Notice, bool, error) {
	c.calls++
	return inbox.Notice{}, false, ctx.Err()
}

func TestNoticeAfterCancelledContextSkipsTheQuery(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	n := &countingNoticer{}
	notice, due := inbox.NoticeAfter(ctx, n, auth.Principal{AgentID: 7})
	if due || !reflect.DeepEqual(notice, inbox.Notice{}) || n.calls != 0 {
		t.Fatalf("NoticeAfter(cancelled) = %+v, %v after %d queries; want {}, false without a query", notice, due, n.calls)
	}
}
