package inbox

import (
	"reflect"
	"testing"
)

func TestSenderNotify(t *testing.T) {
	t.Parallel()
	from := Sender{SpaceID: 7, Space: "alpha", Name: "lead"}
	proto := Entry{Type: TopicMessage, Topic: "news", Body: "hi", Critical: true}
	got := from.Notify([]int64{2, 3}, proto)
	want := []Entry{
		{AgentID: 2, SpaceID: 7, Space: "alpha", From: "lead", Type: TopicMessage, Topic: "news", Body: "hi", Critical: true},
		{AgentID: 3, SpaceID: 7, Space: "alpha", From: "lead", Type: TopicMessage, Topic: "news", Body: "hi", Critical: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Notify = %+v, want %+v", got, want)
	}
	if got := from.Notify(nil, proto); got == nil || len(got) != 0 {
		t.Fatalf("Notify(nil) = %#v, want empty non-nil", got)
	}
}
