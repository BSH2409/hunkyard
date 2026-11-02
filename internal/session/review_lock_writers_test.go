package session

import (
	"os"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// holdReviewLockWhileWriting holds the review lock, as `hunkyard comment`
// does, and adds a marker file before releasing it; write starts while the
// lock is held and must keep the marker.
func holdReviewLockWhileWriting(t *testing.T, hunkyardPath string, write func()) {
	t.Helper()
	locked := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- WithReviewLock(hunkyardPath, func() error {
			close(locked)
			cj, err := readHunkyardJSONFromDisk(hunkyardPath)
			if err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			cj.Files["cli-write.md"] = HunkyardJSONFile{Status: "added", Comments: []Comment{}}
			return saveHunkyardJSONToDisk(hunkyardPath, cj)
		})
	}()
	<-locked
	write()
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	cj, err := readHunkyardJSONFromDisk(hunkyardPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cj.Files["cli-write.md"]; !ok {
		t.Fatal("the write ignored the review lock and dropped the CLI's change")
	}
}

func seedLockedReview(t *testing.T) string {
	t.Helper()
	hunkyardPath := t.TempDir() + "/review-key"
	if err := saveHunkyardJSONToDisk(hunkyardPath, HunkyardJSON{Files: map[string]HunkyardJSONFile{
		"old.go": {Status: "modified", Comments: []Comment{{ID: "c1", Body: "x", StartLine: 1, EndLine: 1}}},
	}}); err != nil {
		t.Fatal(err)
	}
	return hunkyardPath
}

func TestRenameRewriteWaitsForTheReviewLock(t *testing.T) {
	hunkyardPath := seedLockedReview(t)
	holdReviewLockWhileWriting(t, hunkyardPath, func() {
		rewriteReviewJSONRenames(hunkyardPath, []pathRename{{from: "old.go", to: "new.go"}})
	})
	cj, _ := readHunkyardJSONFromDisk(hunkyardPath)
	if _, ok := cj.Files["new.go"]; !ok {
		t.Fatalf("rename not applied: %+v", cj.Files)
	}
}

func TestPersistActiveDiffScopeWaitsForTheReviewLock(t *testing.T) {
	hunkyardPath := seedLockedReview(t)
	s := &Session{ReviewFilePath: hunkyardPath}
	holdReviewLockWhileWriting(t, hunkyardPath, func() {
		if err := s.persistActiveDiffScope("layer"); err != nil {
			t.Error(err)
		}
	})
	if cj, _ := readHunkyardJSONFromDisk(hunkyardPath); cj.ActiveDiffScope != "layer" {
		t.Fatalf("active diff scope lost: %q", cj.ActiveDiffScope)
	}
}

func TestWriteSnapshotOwnsItsReplies(t *testing.T) {
	s := &Session{
		Files:          []*FileEntry{{Path: "a.go", Comments: []Comment{{ID: "c1", Body: "x", Replies: []Reply{{ID: "r1", Body: "before"}}}}}},
		reviewComments: []Comment{{ID: "g1", Body: "y", Replies: []Reply{{ID: "r2", Body: "before"}}}},
		subscribers:    map[chan SSEEvent]struct{}{},
	}
	snap := s.snapshotForWrite(t.TempDir() + "/review-key")
	s.mu.Lock()
	s.Files[0].Comments[0].Replies[0].Body = "after"
	s.reviewComments[0].Replies[0].Body = "after"
	s.mu.Unlock()
	if got := snap.files[0].comments[0].Replies[0].Body; got != "before" {
		t.Errorf("file reply in the snapshot changed to %q", got)
	}
	if got := snap.reviewComments[0].Replies[0].Body; got != "before" {
		t.Errorf("review-level reply in the snapshot changed to %q", got)
	}
}

func TestCommitDiffOfAnAddedFileShowsThatCommitsContent(t *testing.T) {
	dir := initTestRepo(t)
	commitAt(t, dir, "added.txt", "first version\n", "add file")
	added := gitT(t, dir, "rev-parse", "HEAD")
	commitAt(t, dir, "added.txt", "today's version\n", "edit file")
	hunks := computeScopedDiffHunks("added.txt", "all", added, "added", "", "today's version\n", "", dir, &vcs.GitVCS{}, false)
	var lines []string
	for _, h := range hunks {
		for _, l := range h.Lines {
			lines = append(lines, l.Content)
		}
	}
	if len(lines) != 1 || lines[0] != "first version" {
		t.Fatalf("commit diff lines = %q, want the content the commit added", lines)
	}
}

func TestSignalRoundCompleteKeepsTheLastDebouncedChange(t *testing.T) {
	s := newTestSession(t)
	s.ReviewFilePath = t.TempDir() + "/review-key"
	if _, ok := s.AddComment(s.Files[0].Path, 1, 1, "", "last-second comment", "", "reviewer"); !ok {
		t.Fatal("comment not added")
	}
	s.SignalRoundComplete()
	cj, err := readHunkyardJSONFromDisk(s.ReviewFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cj.Files[s.Files[0].Path].Comments) != 1 {
		t.Fatalf("the comment added just before round complete was not written: %+v", cj.Files)
	}
}

// An edit another writer made to review.json while a write is pending (for
// example hunkyard pull stamping a github_id) survives round complete.
func TestSignalRoundCompleteLeavesAnExternallyEditedReviewFile(t *testing.T) {
	s := newTestSession(t)
	s.ReviewFilePath = t.TempDir() + "/review-key"
	path := s.Files[0].Path
	if _, ok := s.AddComment(path, 1, 1, "", "first", "", "reviewer"); !ok {
		t.Fatal("comment not added")
	}
	s.WriteFiles()
	cj, err := readHunkyardJSONFromDisk(s.ReviewFilePath)
	if err != nil {
		t.Fatal(err)
	}
	file := cj.Files[path]
	file.Comments[0].GitHubID = 12345
	cj.Files[path] = file
	if err := saveHunkyardJSONToDisk(s.ReviewFilePath, cj); err != nil {
		t.Fatal(err)
	}
	// File timestamps are coarse; make the other writer's change visibly newer.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(ReviewPathsFor(s.ReviewFilePath).Review, later, later); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.AddComment(path, 1, 1, "", "second", "", "reviewer"); !ok {
		t.Fatal("second comment not added")
	}
	s.SignalRoundComplete()
	after, err := readHunkyardJSONFromDisk(s.ReviewFilePath)
	if err != nil {
		t.Fatal(err)
	}
	stamped := false
	for _, c := range after.Files[path].Comments {
		stamped = stamped || c.GitHubID == 12345
	}
	if !stamped {
		t.Fatalf("the externally written github_id was overwritten: %+v", after.Files[path].Comments)
	}
}
