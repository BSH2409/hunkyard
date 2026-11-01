package session

import "testing"

// A burst of comment refreshes must not crowd out the event that follows it.
func TestSSEDoesNotDropUnderBurst(t *testing.T) {
	s := &Session{subscribers: map[chan SSEEvent]struct{}{}}
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)
	for range 50 {
		s.Notify(SSEEvent{Type: "comments-changed"})
	}
	s.Notify(SSEEvent{Type: "paused"})
	last := ""
	for len(ch) > 0 {
		last = (<-ch).Type
	}
	if last != "paused" {
		t.Errorf("last delivered event = %q, want paused", last)
	}
}
