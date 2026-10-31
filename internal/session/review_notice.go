package session

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
)

// OwnedReview is a live review launched by one host session, with its
// published feedback that has not been acknowledged yet.
type OwnedReview struct {
	ReviewID          string
	Pending           int
	LatestBatchID     string
	LatestDisposition string
}

var reviewIsLive = func(reviewID string) bool {
	_, alive := daemon.FindAliveSession(reviewID)
	return alive
}

// OwnedOpenReviews lists the live reviews a host session owns. Owner records
// whose review folder is gone are skipped with one stat.
func OwnedOpenReviews(stateRoot, hostKind, hostSessionID string) []OwnedReview {
	if hostKind == "" || hostSessionID == "" {
		return nil
	}
	identity := feedback.IdentityForSeed(hostKind + ":" + hostSessionID)
	entries, err := os.ReadDir(filepath.Join(stateRoot, "owners"))
	if err != nil {
		return nil
	}
	var reviews []OwnedReview
	for _, entry := range entries {
		reviewID, isRecord := strings.CutSuffix(entry.Name(), ".json")
		if !isRecord {
			continue
		}
		reviewDir := filepath.Join(stateRoot, "reviews", reviewID)
		if _, err := os.Stat(reviewDir); err != nil {
			continue
		}
		if _, err := feedback.LoadOwner(stateRoot, reviewID, identity); err != nil || !reviewIsLive(reviewID) {
			continue
		}
		review := OwnedReview{ReviewID: reviewID}
		if store, err := feedback.Open(reviewDir); err == nil {
			if pending, err := store.Pending(); err == nil && len(pending) > 0 {
				latest := pending[len(pending)-1]
				review.Pending, review.LatestBatchID, review.LatestDisposition = len(pending), latest.ID, latest.Disposition
			}
		}
		reviews = append(reviews, review)
	}
	return reviews
}

// WaitForPublication blocks until the review has a pending batch that
// accept approves, or until timeout or until active reports the waiter was
// superseded.
func WaitForPublication(stateRoot, reviewID string, timeout time.Duration, accept func(batchID string) bool, active func() bool) (batchID, disposition string, ok bool) {
	return waitForPublication(stateRoot, reviewID, timeout, 2*time.Second, accept, active)
}

func waitForPublication(stateRoot, reviewID string, timeout, interval time.Duration, accept func(string) bool, active func() bool) (string, string, bool) {
	dir := filepath.Join(stateRoot, "reviews", reviewID)
	deadline := time.Now().Add(timeout)
	for {
		if !active() {
			return "", "", false
		}
		if _, err := os.Stat(dir); err != nil {
			return "", "", false
		}
		if store, err := feedback.Open(dir); err == nil {
			if pending, err := store.Pending(); err == nil {
				for _, batch := range pending {
					if accept(batch.ID) {
						return batch.ID, batch.Disposition, true
					}
				}
			}
		}
		if !time.Now().Add(interval).Before(deadline) {
			return "", "", false
		}
		time.Sleep(interval)
	}
}
