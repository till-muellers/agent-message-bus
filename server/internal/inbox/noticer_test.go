package inbox_test

import (
	"context"
	"errors"
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
	waiting := inbox.Notice{Waiting: 2, Spaces: []string{"alpha"}}
	cases := []struct {
		name    string
		noticer fakeNoticer
		due     bool
	}{
		{"due", fakeNoticer{notice: waiting, due: true}, true},
		{"nothing new", fakeNoticer{}, false},
		{"a failed query is swallowed", fakeNoticer{notice: waiting, due: true, err: errors.New("db down")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n, due := inbox.NoticeAfter(t.Context(), tc.noticer, auth.Principal{AgentID: 7})
			if due != tc.due || due && n.Waiting != 2 || !due && n.Waiting != 0 {
				t.Fatalf("NoticeAfter = %+v, %v; want due %v", n, due, tc.due)
			}
		})
	}
}
