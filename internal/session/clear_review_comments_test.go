package session

import (
	"sync"
	"testing"
)

// Run with -race: clearing review comments must not race with readers.
func TestClearReviewCommentsTakesLock(t *testing.T) {
	s := &Session{}
	s.AddReviewComment("note", "reviewer")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.ClearReviewComments() }()
	go func() { defer wg.Done(); _ = s.GetReviewComments() }()
	wg.Wait()
	if got := s.GetReviewComments(); len(got) != 0 {
		t.Errorf("review comments after clear = %d", len(got))
	}
}
