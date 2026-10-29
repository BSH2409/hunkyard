package session

import (
	"encoding/json"
	"fmt"
	"os"
)

// readHunkyardJSONFromDisk reads review.json for a review identity folder.
// Missing file returns zero HunkyardJSON and nil error.
func readHunkyardJSONFromDisk(hunkyardPath string) (HunkyardJSON, error) {
	var cj HunkyardJSON
	data, err := ReadFileShared(ReviewPathsFor(hunkyardPath).Review)
	if err != nil {
		if os.IsNotExist(err) {
			return cj, nil
		}
		return cj, err
	}
	if err := json.Unmarshal(data, &cj); err != nil {
		return cj, err
	}
	return cj, nil
}

// SaveHunkyardJSON writes review.json for a review identity folder. It is the
// single write path for review.json: review.SaveHunkyardJSON delegates here, so
// every writer (comment, github, live, preview, cmd/hunkyard) gets the
// same folder-normalization guard and atomic write.
func SaveHunkyardJSON(hunkyardPath string, cj HunkyardJSON) error {
	return saveHunkyardJSONToDisk(hunkyardPath, cj)
}

// saveHunkyardJSONToDisk writes review.json for a review identity folder.
func saveHunkyardJSONToDisk(hunkyardPath string, cj HunkyardJSON) error {
	// Defense-in-depth: if a code path stat-tests <identity> and finds a flat
	// file (e.g. an external tool dropped one in, or a v3 downgrade
	// reintroduced the layout), normalize to the folder form before writing.
	// ensureReviewFolder is a no-op when <identity> is already a directory
	// (the steady-state v4 case), so this guard adds only a single os.Stat per
	// save in production. See plan v4 §Folder-format invariants.
	if err := ensureReviewFolder(hunkyardPath); err != nil {
		return fmt.Errorf("ensuring review folder: %w", err)
	}
	data, err := json.MarshalIndent(cj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling review file: %w", err)
	}
	return AtomicWriteFile(ReviewPathsFor(hunkyardPath).Review, append(data, '\n'), 0o644)
}

// SnapshotsFile is the on-disk shape of snapshots.json.
type SnapshotsFile struct {
	RoundSnapshots map[string]map[int]RoundSnapshot `json:"round_snapshots"`
}

// loadSnapshotsFromDisk reads snapshots.json. Missing file is not an error.
func loadSnapshotsFromDisk(snapshotsPath string) (SnapshotsFile, error) {
	sf := SnapshotsFile{RoundSnapshots: map[string]map[int]RoundSnapshot{}}
	data, err := ReadFileShared(snapshotsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return sf, nil
		}
		return sf, fmt.Errorf("reading snapshots file: %w", err)
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return sf, fmt.Errorf("invalid snapshots file: %w", err)
	}
	if sf.RoundSnapshots == nil {
		sf.RoundSnapshots = map[string]map[int]RoundSnapshot{}
	}
	return sf, nil
}

// ClearReviewFolder removes the entire review identity folder.
func ClearReviewFolder(identity string) error {
	if err := os.RemoveAll(identity); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveSnapshotsFile writes snapshots.json atomically.
func SaveSnapshotsFile(snapshotsPath string, sf SnapshotsFile) error {
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshots file: %w", err)
	}
	return AtomicWriteFile(snapshotsPath, append(data, '\n'), 0o644)
}

// CheckGitHubSyncAllowed gates `hunkyard pull` and `hunkyard push` from running
// on live reviews. Live pins have no line anchors and cannot round-trip
// through PR review comments.
func CheckGitHubSyncAllowed(cj HunkyardJSON, op string) error {
	if cj.ReviewType == "live" {
		return fmt.Errorf("%s is not supported for live reviews", op)
	}
	return nil
}
