package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

func TestSetReviewStoryKeepsCommentsWrittenMeanwhile(t *testing.T) {
	hunkyardPath := filepath.Join(t.TempDir(), "review-key")
	if err := review.SaveHunkyardJSON(hunkyardPath, review.HunkyardJSON{Files: map[string]review.HunkyardJSONFile{}}); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- session.WithReviewLock(hunkyardPath, func() error {
			close(locked)
			cj, err := review.LoadHunkyardJSON(hunkyardPath)
			if err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			cj.Files["reviewer.md"] = review.HunkyardJSONFile{Status: "added", Comments: []session.Comment{{ID: "c1", Body: "new comment"}}}
			return review.SaveHunkyardJSON(hunkyardPath, cj)
		})
	}()
	<-locked
	if err := setReviewStory(hunkyardPath, &session.Story{}); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		t.Fatal(err)
	}
	if cj.Story == nil {
		t.Error("story not saved")
	}
	if _, ok := cj.Files["reviewer.md"]; !ok {
		t.Error("saving the story dropped a comment written meanwhile")
	}
}
