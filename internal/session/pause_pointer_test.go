package session

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// A plan or --output review keeps its paused marker outside
// ~/.hunkyard/reviews/<key>; await, resume and stop must still find it.
func TestPausedReviewOutsideTheReviewsFolderIsFound(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	key := "abcdefabcdef"
	planDir := filepath.Join(t.TempDir(), "plans", "my-plan", ".hunkyard")
	if err := WritePausedMarker(planDir, PausedMarker{PausedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := RecordPausedReview(key, planDir); err != nil {
		t.Fatal(err)
	}
	if dir, paused := PausedReviewDir(key); !paused || dir != planDir {
		t.Fatalf("PausedReviewDir = %q, %v; want %q", dir, paused, planDir)
	}
	if dir, err := resolveReconnectReviewDir(key, daemon.SessionEntry{}); err != nil || dir != planDir {
		t.Fatalf("resume looked in %q (%v), want %q", dir, err, planDir)
	}
	if stopped, err := stopPausedReview(key); err != nil || !stopped {
		t.Fatalf("stop: %v %v", stopped, err)
	}
	if _, paused := PausedReviewDir(key); paused {
		t.Fatal("stop left the review paused")
	}
}
