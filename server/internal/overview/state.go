package overview

import (
	"context"

	"github.com/till-muellers/agent-message-bus/server/internal/auth"
	"github.com/till-muellers/agent-message-bus/server/internal/invites"
	"github.com/till-muellers/agent-message-bus/server/internal/spaces"
)

// SpaceRow is one space of the GUI list; Invites, the open-invite count, only where the
// caller may see invites (spaces.Membership.MaySeeInvites).
type SpaceRow struct {
	spaces.SpaceInfo
	Invites *int `json:"invites,omitempty"`
}

// ListState is the GUI state without a space: the caller, the spaces it sees and how many
// entries wait in its mailbox scope.
type ListState struct {
	You     spaces.WhoamiResult
	Spaces  []SpaceRow
	Waiting int
}

// SpaceState is the GUI state of one space: the caller, the space's overview, its open
// invites only where the caller may see them, and the waiting entries.
type SpaceState struct {
	You      spaces.WhoamiResult
	Overview Overview
	Invites  *[]invites.Pending
	Waiting  int
}

// ListState composes the GUI space list for p, counting the open invites of every space
// where p may see them in one read.
func (s *Service) ListState(ctx context.Context, p auth.Principal) (ListState, error) {
	you, waiting, err := s.caller(ctx, p)
	if err != nil {
		return ListState{}, err
	}
	listed, err := s.sp.Listing(ctx, p)
	if err != nil {
		return ListState{}, err
	}
	rows, err := s.rows(ctx, listed)
	if err != nil {
		return ListState{}, err
	}
	return ListState{You: you, Spaces: rows, Waiting: waiting}, nil
}

// rows maps listed to GUI rows with the open-invite counts where the caller may see them.
func (s *Service) rows(ctx context.Context, listed []spaces.Listed) ([]SpaceRow, error) {
	var ids []int64
	for _, l := range listed {
		if l.Caller.MaySeeInvites() {
			ids = append(ids, l.Caller.SpaceID)
		}
	}
	counts, err := s.inv.CountOpen(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]SpaceRow, 0, len(listed))
	for _, l := range listed {
		row := SpaceRow{SpaceInfo: l.SpaceInfo}
		if l.Caller.MaySeeInvites() {
			n := counts[l.Caller.SpaceID]
			row.Invites = &n
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// SpaceState composes the GUI view of space for p, resolving the membership once.
func (s *Service) SpaceState(ctx context.Context, p auth.Principal, space string) (SpaceState, error) {
	you, waiting, err := s.caller(ctx, p)
	if err != nil {
		return SpaceState{}, err
	}
	m, err := s.r.Resolve(ctx, s.q, p, space)
	if err != nil {
		return SpaceState{}, err
	}
	o, err := s.of(ctx, m)
	if err != nil {
		return SpaceState{}, err
	}
	st := SpaceState{You: you, Overview: o, Waiting: waiting}
	if m.MaySeeInvites() {
		pending, err := s.inv.ListOpen(ctx, m.SpaceID)
		if err != nil {
			return SpaceState{}, err
		}
		st.Invites = &pending
	}
	return st, nil
}

// caller returns who p is and how many entries wait in its mailbox scope.
func (s *Service) caller(ctx context.Context, p auth.Principal) (spaces.WhoamiResult, int, error) {
	you, err := s.sp.Whoami(ctx, p)
	if err != nil {
		return spaces.WhoamiResult{}, 0, err
	}
	n, err := s.ib.Count(ctx, p)
	if err != nil {
		return spaces.WhoamiResult{}, 0, err
	}
	return you, n, nil
}
