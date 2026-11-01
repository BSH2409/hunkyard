package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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
