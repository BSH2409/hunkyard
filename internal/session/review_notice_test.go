package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func ownedReviewFixture(t *testing.T, stateRoot, reviewID, seed string, batches int) {
	t.Helper()
	if _, err := feedback.OpenOwner(stateRoot, reviewID, feedback.IdentityForSeed(seed)); err != nil {
		t.Fatal(err)
	}
	store, err := feedback.Open(filepath.Join(stateRoot, "reviews", reviewID))
	if err != nil {
		t.Fatal(err)
	}
	for i := range batches {
		batch := feedback.Batch{Version: 1, ReviewID: reviewID, SnapshotID: "snapshot_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Round: 1, Disposition: "request_changes", Comments: []feedback.Comment{{ID: "c" + string(rune('a'+i)), Body: "fix it"}}}
		if _, err := store.Publish("request-"+string(rune('a'+i)), batch); err != nil {
			t.Fatal(err)
		}
	}
}

func withLiveReviews(t *testing.T, live map[string]bool) {
	t.Helper()
	previous := reviewIsLive
	reviewIsLive = func(id string) bool { return live[id] }
	t.Cleanup(func() { reviewIsLive = previous })
}

func TestOwnedOpenReviewsFindsOnlyThisSession(t *testing.T) {
	root := t.TempDir()
	ownedReviewFixture(t, root, "aaaaaaaaaaaa", "claude:session-1", 1)
	ownedReviewFixture(t, root, "bbbbbbbbbbbb", "claude:session-2", 1)
	withLiveReviews(t, map[string]bool{"aaaaaaaaaaaa": true, "bbbbbbbbbbbb": true})
	got := OwnedOpenReviews(root, "claude", "session-1")
	if len(got) != 1 || got[0].ReviewID != "aaaaaaaaaaaa" || got[0].Pending != 1 || got[0].LatestDisposition != "request_changes" {
		t.Fatalf("owned reviews: %+v", got)
	}
}

func TestOwnedOpenReviewsSkipsDeadSessions(t *testing.T) {
	root := t.TempDir()
	ownedReviewFixture(t, root, "aaaaaaaaaaaa", "codex:thread-1", 1)
	withLiveReviews(t, map[string]bool{})
	if got := OwnedOpenReviews(root, "codex", "thread-1"); len(got) != 0 {
		t.Fatalf("dead review reported: %+v", got)
	}
}

func TestOwnedOpenReviewsCountsUnacknowledged(t *testing.T) {
	root := t.TempDir()
	ownedReviewFixture(t, root, "aaaaaaaaaaaa", "claude:session-1", 2)
	withLiveReviews(t, map[string]bool{"aaaaaaaaaaaa": true})
	store, _ := feedback.Open(filepath.Join(root, "reviews", "aaaaaaaaaaaa"))
	pending, _ := store.Pending()
	if got := OwnedOpenReviews(root, "claude", "session-1"); len(got) != 1 || got[0].Pending != 2 || got[0].LatestBatchID != pending[1].ID {
		t.Fatalf("unacknowledged count: %+v", got)
	}
}

func TestOwnedOpenReviewsIgnoresOrphanOwnerRecords(t *testing.T) {
	root := t.TempDir()
	if _, err := feedback.OpenOwner(root, "cccccccccccc", feedback.IdentityForSeed("claude:session-1")); err != nil {
		t.Fatal(err)
	}
	withLiveReviews(t, map[string]bool{"cccccccccccc": true})
	if got := OwnedOpenReviews(root, "claude", "session-1"); len(got) != 0 {
		t.Fatalf("owner record without a review folder reported: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "reviews", "cccccccccccc")); !os.IsNotExist(err) {
		t.Fatal("lookup created a review folder for an orphan owner record")
	}
}

func TestWaitForPublicationAcceptsOnlyUnclaimedBatches(t *testing.T) {
	root := t.TempDir()
	ownedReviewFixture(t, root, "aaaaaaaaaaaa", "claude:session-1", 1)
	store, _ := feedback.Open(filepath.Join(root, "reviews", "aaaaaaaaaaaa"))
	pending, _ := store.Pending()
	existing := pending[0].ID
	id, _, ok := waitForPublication(root, "aaaaaaaaaaaa", time.Second, 20*time.Millisecond, func(batch string) bool { return batch == existing }, func() bool { return true })
	if !ok || id != existing {
		t.Fatalf("pending unclaimed batch not delivered: %q %v", id, ok)
	}
	if _, _, ok := waitForPublication(root, "aaaaaaaaaaaa", 200*time.Millisecond, 20*time.Millisecond, func(string) bool { return false }, func() bool { return true }); ok {
		t.Fatal("claimed batch delivered again")
	}
	if _, _, ok := waitForPublication(root, "aaaaaaaaaaaa", time.Second, 20*time.Millisecond, func(string) bool { return true }, func() bool { return false }); ok {
		t.Fatal("superseded waiter delivered")
	}
}
