package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/preferences"
	"github.com/BSH2409/hunkyard/internal/session"
)

func init() {
	preferences.SetReviewIntegration(ownedReviewNotices, waitForReviewPublication)
	preferences.SetReviewActivity(func(hostKind, hostSessionID, state string) {
		if root := hunkyardStateRoot(); root != "" {
			session.TouchOwnedReviewActivity(root, hostKind, hostSessionID, state)
		}
	})
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

func waitForReviewPublication(reviewID, ownerID string, timeout time.Duration, accept func(string) bool, active func() bool) (string, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !active() {
					cancel()
					return
				}
			}
		}
	}()
	excluded := []string{}
	for ctx.Err() == nil && active() {
		entry, err := daemon.ReadSessionFile(reviewID)
		if err != nil {
			return "", "", false
		}
		path := "/api/feedback/await?timeout=45s&exclude=" + url.QueryEscape(strings.Join(excluded, ","))
		response, err := daemon.OwnerRequestForIdentity(ctx, entry, reviewID, http.MethodGet, path, nil, feedback.IdentityForSeed(ownerID))
		if err != nil {
			return "", "", false
		}
		var result struct {
			Batches []feedback.Batch `json:"batches"`
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&result)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			return "", "", false
		}
		for _, batch := range result.Batches {
			if active() && accept(batch.ID) {
				return batch.ID, batch.Disposition, true
			}
			excluded = append(excluded, batch.ID)
		}
	}
	return "", "", false
}
