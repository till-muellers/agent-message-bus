// Package broker provides in-process pub/sub keyed by MQTT-style topic strings.
package broker

import "sync"

// Memory is an in-process broker. Publish never blocks; slow subscribers are dropped.
type Memory struct {
	mu     sync.Mutex
	buffer int
	subs   map[string]map[*subscriber]struct{}
}

type subscriber struct {
	ch chan []byte
}

// NewMemory returns a broker whose subscriber channels hold buffer messages.
func NewMemory(buffer int) *Memory {
	return &Memory{buffer: buffer, subs: make(map[string]map[*subscriber]struct{})}
}

// Publish delivers payload to every subscriber of topic. A subscriber whose
// buffer is full has its channel closed and is removed.
func (m *Memory) Publish(topic string, payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for s := range m.subs[topic] {
		select {
		case s.ch <- payload:
		default:
			m.removeLocked(topic, s)
		}
	}
}

// Subscribe registers a subscriber on topic. The returned cancel func is idempotent.
func (m *Memory) Subscribe(topic string) (<-chan []byte, func()) {
	s := &subscriber{ch: make(chan []byte, m.buffer)}
	m.mu.Lock()
	if m.subs[topic] == nil {
		m.subs[topic] = make(map[*subscriber]struct{})
	}
	m.subs[topic][s] = struct{}{}
	m.mu.Unlock()
	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.removeLocked(topic, s)
	}
	return s.ch, cancel
}

// removeLocked closes and removes s if still registered. Caller holds m.mu.
func (m *Memory) removeLocked(topic string, s *subscriber) {
	set := m.subs[topic]
	if _, ok := set[s]; !ok {
		return
	}
	delete(set, s)
	close(s.ch)
	if len(set) == 0 {
		delete(m.subs, topic)
	}
}
