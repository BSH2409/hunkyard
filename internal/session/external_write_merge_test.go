package session

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// cliEdit does what `hunkyard comment` does: load review.json, change it and
// save it, under the review lock, then moves its mtime forward so the change
// is distinguishable from the daemon's own write.
func cliEdit(t *testing.T, s *Session, edit func(cj *HunkyardJSON)) {
	t.Helper()
	identity := s.hunkyardJSONPath()
	err := WithReviewLock(identity, func() error {
		data, err := os.ReadFile(ReviewPathsFor(identity).Review)
		if err != nil {
			return err
		}
		var cj HunkyardJSON
		if err := json.Unmarshal(data, &cj); err != nil {
			return err
		}
		edit(&cj)
		if err := SaveHunkyardJSON(identity, cj); err != nil {
			return err
		}
		later := time.Now().Add(2 * time.Second)
		return os.Chtimes(ReviewPathsFor(identity).Review, later, later)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readDisk(t *testing.T, s *Session) HunkyardJSON {
	t.Helper()
	data, err := os.ReadFile(ReviewPathsFor(s.hunkyardJSONPath()).Review)
	if err != nil {
		t.Fatal(err)
	}
	var cj HunkyardJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		t.Fatal(err)
	}
	return cj
}

func writtenReview(t *testing.T) (*Session, Comment) {
	t.Helper()
	s := NewTestSession(t)
	t.Cleanup(s.QuiesceForTest)
	c, ok := s.AddComment("plan.md", 1, 1, "", "first", "", "reviewer")
	if !ok {
		t.Fatal("comment not added")
	}
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	return s, c
}

func TestDaemonWriteKeepsAnAgentReply(t *testing.T) {
	s, c := writtenReview(t)
	cliEdit(t, s, func(cj *HunkyardJSON) {
		f := cj.Files["plan.md"]
		f.Comments[0].Replies = append(f.Comments[0].Replies, Reply{ID: "r_agent", Body: "Fixed", Author: "agent"})
		cj.Files["plan.md"] = f
	})
	// The reviewer acts before the watcher's next tick.
	s.AddComment("plan.md", 3, 3, "", "second", "", "reviewer")
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	for _, dc := range readDisk(t, s).Files["plan.md"].Comments {
		if dc.ID == c.ID && (len(dc.Replies) != 1 || dc.Replies[0].ID != "r_agent") {
			t.Fatalf("agent reply lost on disk: %+v", dc.Replies)
		}
	}
	for _, mc := range s.GetComments("plan.md") {
		if mc.ID == c.ID && len(mc.Replies) != 1 {
			t.Fatalf("agent reply missing from the review: %+v", mc.Replies)
		}
	}
}

func TestDaemonWriteKeepsAnAgentReviewCommentAndResolve(t *testing.T) {
	s, c := writtenReview(t)
	cliEdit(t, s, func(cj *HunkyardJSON) {
		cj.ReviewComments = append(cj.ReviewComments, Comment{ID: "rc_agent", Body: "Summary from the agent", Author: "agent", Scope: "review"})
		f := cj.Files["plan.md"]
		f.Comments[0].Resolved = true
		cj.Files["plan.md"] = f
	})
	s.AddComment("plan.md", 3, 3, "", "second", "", "reviewer")
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	disk := readDisk(t, s)
	if len(disk.ReviewComments) != 1 || disk.ReviewComments[0].ID != "rc_agent" {
		t.Fatalf("agent review comment lost: %+v", disk.ReviewComments)
	}
	for _, dc := range disk.Files["plan.md"].Comments {
		if dc.ID == c.ID && !dc.Resolved {
			t.Fatal("agent resolve lost")
		}
	}
}

func TestDaemonWriteDoesNotRestoreAReplyDeletedInTheBrowser(t *testing.T) {
	s, c := writtenReview(t)
	r, ok := s.AddReply("plan.md", c.ID, "temp", "reviewer")
	if !ok {
		t.Fatal("reply not added")
	}
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	if !s.DeleteReply("plan.md", c.ID, r.ID) {
		t.Fatal("reply not deleted")
	}
	// An unrelated external change makes the daemon compare against disk.
	cliEdit(t, s, func(cj *HunkyardJSON) {
		cj.ReviewComments = append(cj.ReviewComments, Comment{ID: "rc_agent", Body: "note", Author: "agent", Scope: "review"})
	})
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	for _, dc := range readDisk(t, s).Files["plan.md"].Comments {
		if dc.ID == c.ID && len(dc.Replies) != 0 {
			t.Fatalf("deleted reply came back: %+v", dc.Replies)
		}
	}
	if len(readDisk(t, s).ReviewComments) != 1 {
		t.Fatal("precondition: the external review comment should be kept")
	}
}

func TestDaemonWriteWaitsForTheReviewLock(t *testing.T) {
	s, _ := writtenReview(t)
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = WithReviewLock(s.hunkyardJSONPath(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	go func() {
		s.AddComment("plan.md", 3, 3, "", "second", "", "reviewer")
		_ = s.SyncWriteFiles()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("the daemon wrote while the CLI held the review lock")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the daemon write never finished after the lock was released")
	}
}
