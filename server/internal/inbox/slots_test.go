package inbox

import "testing"

func TestStreamSlots(t *testing.T) {
	t.Parallel()
	type step struct {
		op      string // "acquire" or "release"
		agentID int64
		want    bool // acquire result
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"first acquire wins", []step{{"acquire", 1, true}}},
		{"second acquire of same agent fails", []step{{"acquire", 1, true}, {"acquire", 1, false}}},
		{"release frees the slot", []step{{"acquire", 1, true}, {"release", 1, false}, {"acquire", 1, true}}},
		{"agents have their own slots", []step{{"acquire", 1, true}, {"acquire", 2, true}}},
		{"releasing one agent keeps another held", []step{
			{"acquire", 1, true}, {"acquire", 2, true}, {"release", 1, false}, {"acquire", 2, false},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newStreamSlots()
			for i, st := range tc.steps {
				if st.op == "release" {
					s.release(st.agentID)
					continue
				}
				if got := s.acquire(st.agentID); got != st.want {
					t.Fatalf("step %d: acquire(%d) = %v, want %v", i, st.agentID, got, st.want)
				}
			}
		})
	}
}
