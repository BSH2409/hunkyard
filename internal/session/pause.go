package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
)

// pausedMarkerName is the file a paused review leaves in its review folder:
// the daemon exited to free its port, and `hunkyard --session <id>` brings it
// back. Its presence tells the CLI and the agent's waiter "paused", not
// "crashed".
const pausedMarkerName = "paused.json"

// PausedMarker records when a review paused and where it was listening.
type PausedMarker struct {
	PausedAt time.Time `json:"paused_at"`
	Port     int       `json:"port"`
	Host     string    `json:"host,omitempty"`
	// CWD and CliArgs let a review with no comments resume: such a review
	// has no review.json to record them.
	CWD     string   `json:"cwd,omitempty"`
	CliArgs []string `json:"cli_args,omitempty"`
}

// WritePausedMarker records a pause in reviewDir.
func WritePausedMarker(reviewDir string, marker PausedMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return AtomicWriteFile(filepath.Join(reviewDir, pausedMarkerName), data, 0o600)
}

// ReadPausedMarker reports whether the review in reviewDir is paused.
func ReadPausedMarker(reviewDir string) (PausedMarker, bool) {
	var marker PausedMarker
	data, err := os.ReadFile(filepath.Join(reviewDir, pausedMarkerName))
	if err != nil || json.Unmarshal(data, &marker) != nil {
		return PausedMarker{}, false
	}
	return marker, true
}

// RemovePausedMarker clears a pause, when the review resumes or is stopped.
func RemovePausedMarker(reviewDir string) error {
	err := os.Remove(filepath.Join(reviewDir, pausedMarkerName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RoundBusy reports whether a round transition is being applied; a review
// never pauses in the middle of one.
func (s *Session) RoundBusy() bool { return s.roundBusy.Load() }

// SetRoundBusyForTest marks a round transition in progress.
func (s *Session) SetRoundBusyForTest(busy bool) { s.roundBusy.Store(busy) }

// A paused review's marker lives in its own folder, which for plan and
// --output reviews is not ~/.hunkyard/reviews/<key>, and the session file
// that named that folder is gone once the daemon exits. A pointer in the
// sessions folder records it until the review resumes or is stopped.
func pausedPointerPath(key string) (string, error) {
	dir, err := daemon.SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".paused"), nil
}

// RecordPausedReview remembers where key's paused review lives.
func RecordPausedReview(key, reviewDir string) error {
	path, err := pausedPointerPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return AtomicWriteFile(path, []byte(reviewDir), 0o600)
}

// PausedReviewDir returns the folder of key's paused review, if it is paused.
func PausedReviewDir(key string) (string, bool) {
	if path, err := pausedPointerPath(key); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			if _, paused := ReadPausedMarker(string(data)); paused {
				return string(data), true
			}
		}
	}
	dir, err := daemon.ReviewDir(key)
	if err != nil {
		return "", false
	}
	_, paused := ReadPausedMarker(dir)
	return dir, paused
}

// ForgetPausedReview clears key's pause: the marker in reviewDir and the
// pointer to it.
func ForgetPausedReview(key, reviewDir string) error {
	if path, err := pausedPointerPath(key); err == nil {
		_ = os.Remove(path)
	}
	return RemovePausedMarker(reviewDir)
}

// ForgetAllPausedReviews clears every paused review: those recorded in the
// sessions folder and any marker left in the reviews folder.
func ForgetAllPausedReviews() error {
	var errs []error
	if dir, err := daemon.SessionsDir(); err == nil {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			key, ok := strings.CutSuffix(e.Name(), ".paused")
			if !ok {
				continue
			}
			reviewDir, _ := PausedReviewDir(key)
			if err := ForgetPausedReview(key, reviewDir); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if dir, err := daemon.ReviewsDir(); err == nil {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				if err := RemovePausedMarker(filepath.Join(dir, e.Name())); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}
