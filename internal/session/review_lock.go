package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// WithReviewLock runs fn while holding the review's exclusive file lock, so
// the daemon's read-merge-write of review.json and the CLI's
// load-modify-save (`hunkyard comment`) never interleave. identity is the
// review folder (what hunkyardJSONPath returns). A review whose folder does
// not exist yet has nothing to race over, and creating the folder here would
// make an empty review look stored, so fn then runs without the lock.
func WithReviewLock(identity string, fn func() error) error {
	folder := filepath.Dir(ReviewPathsFor(identity).Review)
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return fn()
	}
	f, err := os.OpenFile(filepath.Join(folder, "review.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("opening review lock: %w", err)
	}
	defer f.Close()
	if err := flockExclusive(f); err != nil {
		return fmt.Errorf("locking review: %w", err)
	}
	defer func() { _ = funlock(f) }()
	return fn()
}
