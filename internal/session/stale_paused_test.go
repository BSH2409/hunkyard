package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A review paused before anyone commented holds only paused.json; once old
// it is cleaned up like any other untouched review.
func TestCleanupSelectsPausedReviewWithoutComments(t *testing.T) {
	dir := t.TempDir()
	for name, age := range map[string]time.Duration{"oldpaused": 30 * 24 * time.Hour, "newpaused": time.Hour} {
		folder := filepath.Join(dir, name)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "paused.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(folder, when, when); err != nil {
			t.Fatal(err)
		}
	}
	stale := findStaleReviews(dir, 7)
	if len(stale) != 1 || filepath.Base(stale[0].path) != "oldpaused" {
		t.Fatalf("stale = %+v, want only the old paused review", stale)
	}
}
