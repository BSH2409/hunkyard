package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BSH2409/hunkyard/internal/preferences"
	"github.com/BSH2409/hunkyard/internal/session"
)

func init() {
	preferences.SetReviewIntegration(ownedReviewNotices, waitForReviewPublication)
}

func hunkyardStateRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hunkyard")
}

func ownedReviewNotices(hostKind, hostSessionID string) []preferences.ReviewNotice {
	root := hunkyardStateRoot()
	if root == "" {
		return nil
	}
	var notices []preferences.ReviewNotice
	for _, review := range session.OwnedOpenReviews(root, hostKind, hostSessionID) {
		notices = append(notices, preferences.ReviewNotice{ReviewID: review.ReviewID, Pending: review.Pending, BatchID: review.LatestBatchID, Disposition: review.LatestDisposition})
	}
	return notices
}

func waitForReviewPublication(reviewID string, timeout time.Duration, accept func(string) bool, active func() bool) (string, string, bool) {
	root := hunkyardStateRoot()
	if root == "" {
		return "", "", false
	}
	return session.WaitForPublication(root, reviewID, timeout, accept, active)
}
