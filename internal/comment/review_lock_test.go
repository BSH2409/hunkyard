package comment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// writeWhileLocked holds hunkyardPath's review lock, as the daemon does
// while it writes, and adds a marker file entry before releasing it. write
// starts while the lock is held; it must wait and keep the marker.
func writeWhileLocked(t *testing.T, hunkyardPath string, write func() error) {
	t.Helper()
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
			cj.Files["daemon-write.md"] = HunkyardJSONFile{Status: "added", Comments: []session.Comment{}}
			return review.SaveHunkyardJSON(hunkyardPath, cj)
		})
	}()
	<-locked
	if err := write(); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	cj, err := review.LoadHunkyardJSON(hunkyardPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cj.Files["daemon-write.md"]; !ok {
		t.Fatal("the write ignored the review lock and dropped the daemon's change")
	}
}

func TestAddPinCommentWaitsForTheReviewLock(t *testing.T) {
	dir := t.TempDir()
	htmlPath := writePreviewPage(t, dir)
	seedPinReview(t, dir, HunkyardJSON{ReviewType: "preview", Origin: htmlPath, ReviewRound: 1, Files: map[string]HunkyardJSONFile{}})
	hunkyardPath, err := review.ResolveReviewPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeWhileLocked(t, hunkyardPath, func() error {
		_, err := addPinComment(hunkyardPath, "h1", "/", "body", "bot")
		return err
	})
	if pins := loadOutputReview(t, dir).Files["/preview-content"].Comments; len(pins) != 1 {
		t.Fatalf("pin lost: %+v", pins)
	}
}

func TestRedirectedBulkReplyWaitsForTheTargetReviewLock(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	reviews := filepath.Join(home, ".hunkyard", "reviews")
	primary := filepath.Join(reviews, "aaaaaaaaaaaa")
	target := filepath.Join(reviews, "bbbbbbbbbbbb")
	if err := review.SaveHunkyardJSON(primary, HunkyardJSON{Files: map[string]HunkyardJSONFile{}}); err != nil {
		t.Fatal(err)
	}
	if err := review.SaveHunkyardJSON(target, HunkyardJSON{Files: map[string]HunkyardJSONFile{
		"a.go": {Status: "modified", Comments: []session.Comment{{ID: "c_target", Body: "fix this", StartLine: 1, EndLine: 1}}},
	}}); err != nil {
		t.Fatal(err)
	}
	writeWhileLocked(t, target, func() error {
		return bulkAddCommentsToHunkyardJSONAtPath([]BulkCommentEntry{{ReplyTo: "c_target", Body: "done"}}, "bot", primary, inheritedScope{})
	})
	cj, err := review.LoadHunkyardJSON(target)
	if err != nil {
		t.Fatal(err)
	}
	if replies := cj.Files["a.go"].Comments[0].Replies; len(replies) != 1 {
		t.Fatalf("reply lost: %+v", cj.Files["a.go"].Comments[0])
	}
}
