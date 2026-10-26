package inbox

import "sync"

// Delivery tells a sender whether a recipient was connected when its entry committed.
type Delivery string

// Delivery values of a Receipt.
const (
	Live   Delivery = "live"
	Queued Delivery = "queued"
)

// Receipt reports how one recipient got a delivered entry.
type Receipt struct {
	To       string   `json:"to"`
	Delivery Delivery `json:"delivery"`
}

// presence tracks who is connected in memory: the holder of an agent's one live inbox
// stream slot, and the agents blocked in Wait (counted, as one agent may wait twice).
type presence struct {
	mu      sync.Mutex
	held    map[int64]struct{}
	waiting map[int64]int
}

func newPresence() *presence {
	return &presence{held: map[int64]struct{}{}, waiting: map[int64]int{}}
}

// acquire takes agentID's stream slot; false means another stream already holds it.
func (p *presence) acquire(agentID int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.held[agentID]; ok {
		return false
	}
	p.held[agentID] = struct{}{}
	return true
}

// release frees agentID's stream slot for the next stream.
func (p *presence) release(agentID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.held, agentID)
}

// enterWait counts one more Wait of agentID.
func (p *presence) enterWait(agentID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting[agentID]++
}

// leaveWait counts one Wait of agentID less.
func (p *presence) leaveWait(agentID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.waiting[agentID]--; p.waiting[agentID] <= 0 {
		delete(p.waiting, agentID)
	}
}

// connected reports whether agentID holds its stream slot or is blocked in Wait.
func (p *presence) connected(agentID int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, held := p.held[agentID]
	return held || p.waiting[agentID] > 0
}

// Connected reports whether agentID holds its inbox stream slot or is blocked in Wait.
func (s *Service) Connected(agentID int64) bool { return s.presence.connected(agentID) }

// receipts returns one receipt per named recipient, sorted by name (names is sorted),
// live when the recipient is connected now.
func (s *Service) receipts(names []recipient) []Receipt {
	out := make([]Receipt, 0, len(names))
	for _, r := range names {
		d := Queued
		if s.presence.connected(r.id) {
			d = Live
		}
		out = append(out, Receipt{To: r.name, Delivery: d})
	}
	return out
}
