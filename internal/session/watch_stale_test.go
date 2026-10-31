package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// waitForEvent returns true if an event of type kind arrives within d.
func waitForEvent(events chan SSEEvent, kind string, d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case ev := <-events:
			if ev.Type == kind {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func watchedReview(t *testing.T, slowWalk time.Duration) (*Session, string, chan SSEEvent) {
	t.Helper()
	dir := newBudgetRepo(t, 25, 2)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sess.pollInterval = 50 * time.Millisecond
	sess.slowWalkInterval = slowWalk
	sess.InitTestChannels()
	events := sess.Subscribe()
	stop := make(chan struct{})
	go sess.Watch(stop)
	t.Cleanup(func() { close(stop); sess.Unsubscribe(events) })
	return sess, dir, events
}

func TestWatcherMarksReviewStaleWhenHeadMoves(t *testing.T) {
	_, dir, events := watchedReview(t, 0)
	time.Sleep(200 * time.Millisecond) // let the watcher take its baseline
	gitT(t, dir, "commit", "--allow-empty", "-qm", "reviewer commit")
	if !waitForEvent(events, "review-stale", 3*time.Second) {
		t.Fatal("no review-stale event after HEAD moved")
	}
}

func TestWatcherSlowFallbackSeesNewUntrackedFile(t *testing.T) {
	sess, dir, events := watchedReview(t, 100*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	writeFile(t, filepath.Join(dir, "newpkg", "late.go"), "package p\n")
	if !waitForEvent(events, "review-stale", 3*time.Second) {
		t.Fatal("no review-stale event for a new untracked file")
	}
	sess.snapMu.Lock()
	stale := sess.snapStale
	sess.snapMu.Unlock()
	if !stale {
		t.Fatal("snapshot not marked stale")
	}
}

func TestWatcherDoesNotMarkUnchangedReviewStale(t *testing.T) {
	sess, _, events := watchedReview(t, 50*time.Millisecond)
	if waitForEvent(events, "review-stale", 600*time.Millisecond) {
		t.Fatal("review-stale sent with no change: the first tree walk must only take a baseline")
	}
	if sess.GetSessionInfo().Stale {
		t.Fatal("an untouched review reports stale")
	}
}

func TestBaseChangeDoesNotMarkReviewStale(t *testing.T) {
	sess, _, events := watchedReview(t, 0)
	time.Sleep(200 * time.Millisecond)
	if err := sess.ChangeBaseBranch("main"); err != nil {
		t.Fatal(err)
	}
	sess.mu.Lock()
	sess.BaseRef = "different-base" // the watcher reads BaseRef each tick
	sess.mu.Unlock()
	if waitForEvent(events, "review-stale", 500*time.Millisecond) {
		t.Fatal("changing the base marked the working tree stale")
	}
}

func TestIndexRewriteAloneDoesNotMarkReviewStale(t *testing.T) {
	sess, dir, events := watchedReview(t, time.Hour)
	time.Sleep(200 * time.Millisecond) // let the watcher take its baseline
	// What a shell prompt's `git status` does to a touched, unchanged file:
	// the index is rewritten with fresh stat data, the tree is the same.
	path := filepath.Join(dir, "src", "file1.go")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "update-index", "--really-refresh")
	if waitForEvent(events, "review-stale", 800*time.Millisecond) {
		t.Fatal("an index rewrite with no tree change marked the review stale")
	}
	if sess.GetSessionInfo().Stale {
		t.Fatal("review reports stale after an index-only rewrite")
	}
}
