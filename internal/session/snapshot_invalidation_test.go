package session

import (
	"context"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// A watcher invalidation that lands while a reading is under way leaves the
// reading stale, so the next request reads the tree again.
func TestSnapshotInvalidationDuringReadingIsKept(t *testing.T) {
	s := slowSession(t, 0)
	listing := &slowListingVCS{VCS: &vcs.GitVCS{}, entered: make(chan struct{}), release: make(chan struct{})}
	s.VCS = listing
	done := make(chan error, 1)
	go func() {
		_, err := s.snapshotFor(context.Background(), listing, s.RepoRoot, "", false, true)
		done <- err
	}()
	<-listing.entered
	s.invalidateWorkingTreeSnapshot()
	close(listing.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	s.snapMu.Lock()
	stale := s.snapStale
	s.snapMu.Unlock()
	if !stale {
		t.Error("an invalidation during the reading was dropped")
	}
}

// The waiting screen counts the agent's edits as fast as it did before the
// cheap HEAD/index probe existed.
func TestWaitingWalkInterval(t *testing.T) {
	_, waiting := (&Session{}).fingerprintIntervals()
	if waiting != time.Second {
		t.Errorf("waiting-screen walk interval = %v, want 1s", waiting)
	}
}
