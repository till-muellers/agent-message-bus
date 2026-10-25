package inbox

import "sync"

// streamSlots enforces one live inbox stream per agent: an agent's slot is
// held from acquire until release.
type streamSlots struct {
	mu   sync.Mutex
	held map[int64]struct{}
}

func newStreamSlots() *streamSlots {
	return &streamSlots{held: map[int64]struct{}{}}
}

// acquire takes agentID's slot; false means another stream already holds it.
func (s *streamSlots) acquire(agentID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.held[agentID]; ok {
		return false
	}
	s.held[agentID] = struct{}{}
	return true
}

// release frees agentID's slot for the next stream.
func (s *streamSlots) release(agentID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.held, agentID)
}
