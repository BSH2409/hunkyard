package comment

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

func TestCLIReplyWaitsForTheDaemonsReviewLock(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "review-id")
	cj := session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{
		"plan.md": {Comments: []session.Comment{{ID: "c_1", StartLine: 1, EndLine: 1, Body: "x"}}},
	}}
	if err := review.SaveHunkyardJSON(identity, cj); err != nil {
		t.Fatal(err)
	}
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_ = session.WithReviewLock(identity, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	go func() { done <- addReplyToHunkyardJSONAtPath("c_1", "Fixed", "agent", false, identity, "") }()
	select {
	case err := <-done:
		t.Fatalf("the CLI wrote while the daemon held the review lock (err %v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the CLI reply never finished after the lock was released")
	}
	got, err := review.LoadHunkyardJSON(identity)
	if err != nil || len(got.Files["plan.md"].Comments[0].Replies) != 1 {
		t.Fatalf("reply not saved: %+v %v", got.Files["plan.md"], err)
	}
}
