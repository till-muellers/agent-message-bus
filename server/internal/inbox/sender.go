package inbox

// Sender is where entries come from: a space and the agent acting in it.
type Sender struct {
	SpaceID int64
	Space   string
	Name    string
}

// Notify copies proto once per recipient, addressed to it in the sender's space.
func (s Sender) Notify(recipients []int64, proto Entry) []Entry {
	es := make([]Entry, 0, len(recipients))
	for _, id := range recipients {
		e := proto
		e.AgentID, e.SpaceID, e.Space, e.From = id, s.SpaceID, s.Space, s.Name
		es = append(es, e)
	}
	return es
}
