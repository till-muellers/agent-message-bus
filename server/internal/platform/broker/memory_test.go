package broker

import (
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan []byte) ([]byte, bool) {
	t.Helper()
	select {
	case p, ok := <-ch:
		return p, ok
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
		return nil, false
	}
}

func TestFanOut(t *testing.T) {
	t.Parallel()
	m := NewMemory(4)
	a, cancelA := m.Subscribe("s/1")
	defer cancelA()
	b, cancelB := m.Subscribe("s/1")
	defer cancelB()
	other, cancelO := m.Subscribe("s/2")
	defer cancelO()
	m.Publish("s/1", []byte("hi"))
	for _, ch := range []<-chan []byte{a, b} {
		if p, ok := recv(t, ch); !ok || string(p) != "hi" {
			t.Fatalf("got %q ok=%v", p, ok)
		}
	}
	select {
	case p := <-other:
		t.Fatalf("other topic got %q", p)
	default:
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	t.Parallel()
	m := NewMemory(4)
	ch, cancel := m.Subscribe("t")
	cancel()
	cancel() // idempotent
	m.Publish("t", []byte("x"))
	if _, ok := recv(t, ch); ok {
		t.Fatal("expected closed channel after cancel")
	}
}

func TestSlowSubscriberClosed(t *testing.T) {
	t.Parallel()
	m := NewMemory(1)
	ch, cancel := m.Subscribe("t")
	defer cancel()
	m.Publish("t", []byte("1"))
	m.Publish("t", []byte("2")) // buffer full: dropped
	if p, ok := recv(t, ch); !ok || string(p) != "1" {
		t.Fatalf("got %q ok=%v", p, ok)
	}
	if _, ok := recv(t, ch); ok {
		t.Fatal("expected slow subscriber channel closed")
	}
	m.Publish("t", []byte("3")) // must not panic or block
}
