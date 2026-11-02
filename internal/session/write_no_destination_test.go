package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A session with no RepoRoot, OutputDir or ReviewDir has no review
// destination. Before the hunkyardJSONPath guard it fell back to the relative
// path ".hunkyard", so the 200ms debounced write landed in whatever directory
// the process happened to be in — which other tests change with os.Chdir.
// That is how TestNewSessionFromGitLazyThreshold intermittently saw an extra
// .hunkyard/review.json in its fixture repo.
func TestScheduleWriteWithoutDestinationDoesNotTouchCWD(t *testing.T) {
	s := &Session{
		ReviewRound: 1,
		Files: []*FileEntry{{
			Path:     "test.md",
			Comments: []Comment{{ID: "c1", StartLine: 1, EndLine: 1, Body: "Fix this"}},
		}},
	}
	// Positive control: the same debounced write with a destination lands
	// within the window this test waits, so its absence below means something.
	withDest := &Session{RepoRoot: t.TempDir(), ReviewRound: 1, Files: s.Files}
	t.Cleanup(withDest.QuiesceForTest)
	s.mu.Lock()
	s.scheduleWrite()
	s.mu.Unlock()
	withDest.mu.Lock()
	withDest.scheduleWrite()
	withDest.mu.Unlock()

	dir := t.TempDir()
	t.Chdir(dir)

	// Wait for the control write rather than a fixed time: slow runners take
	// longer than the debounce, and both writes were scheduled together.
	deadline := time.Now().Add(10 * time.Second)
	var statErr error
	for {
		if _, statErr = os.Stat(ReviewPathsFor(withDest.hunkyardJSONPath()).Review); statErr == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if statErr != nil {
		t.Fatalf("control: the debounced write did not land within 10s: %v", statErr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hunkyard")); !os.IsNotExist(err) {
		t.Fatalf("debounced write leaked .hunkyard into the working directory (stat err: %v)", err)
	}
}

// SetFocus persists the active diff scope on every focus change, including on
// sessions that have no review destination. Without the guard in
// persistActiveDiffScope that write created a .hunkyard folder in the process
// working directory, which is the same leak the test above covers.
func TestPersistActiveDiffScopeWithoutDestinationDoesNotTouchCWD(t *testing.T) {
	s := &Session{ReviewRound: 1}

	dir := t.TempDir()
	t.Chdir(dir)

	if err := s.persistActiveDiffScope("layer"); err != nil {
		t.Fatalf("persistActiveDiffScope: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".hunkyard")); !os.IsNotExist(err) {
		t.Fatalf("persistActiveDiffScope leaked .hunkyard into the working directory (stat err: %v)", err)
	}
}
