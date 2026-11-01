package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// slowListingVCS takes delay to list working-tree changes, like a large tree.
// When entered is set it is closed as the listing starts, and the listing
// then waits for release instead of the delay.
type slowListingVCS struct {
	vcs.VCS
	delay   time.Duration
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (v *slowListingVCS) Name() string         { return "slow" }
func (v *slowListingVCS) HasStagingArea() bool { return false }
func (v *slowListingVCS) ChangedFilesOnDefaultInDir(string) ([]vcs.FileChange, error) {
	if v.entered != nil {
		v.once.Do(func() { close(v.entered) })
		<-v.release
	} else {
		time.Sleep(v.delay)
	}
	return []vcs.FileChange{{Path: "a.go", Status: "modified"}}, nil
}

func slowSession(t *testing.T, delay time.Duration) *Session {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir+"/a.go", "package a\n")
	s := &Session{
		VCS: &slowListingVCS{VCS: &vcs.GitVCS{}, delay: delay}, Mode: "git", RepoRoot: dir, Branch: "feat", ReviewRound: 1,
		Files:       []*FileEntry{{Path: "a.go", AbsPath: dir + "/a.go", Status: "modified", FileType: "code", Comments: []Comment{}}},
		subscribers: make(map[chan SSEEvent]struct{}), roundComplete: make(chan struct{}, 1),
	}
	t.Cleanup(s.QuiesceForTest)
	return s
}

func TestCommentPostNotBlockedBySlowScopeProbe(t *testing.T) {
	s := slowSession(t, 0)
	lister := s.VCS.(*slowListingVCS)
	lister.entered, lister.release = make(chan struct{}), make(chan struct{})
	probeDone := make(chan struct{})
	go func() { s.GetSessionInfo(); close(probeDone) }()
	t.Cleanup(func() { close(lister.release); <-probeDone })
	select {
	case <-lister.entered: // the probe is now inside the tree read
	case <-time.After(5 * time.Second):
		t.Fatal("the session probe never listed the tree")
	}
	// The tree read is held open until cleanup, so a comment that waits
	// behind it would never finish: bound the wait instead of hanging.
	added := make(chan bool, 1)
	go func() { _, ok := s.AddComment("a.go", 1, 1, "", "body", "", ""); added <- ok }()
	select {
	case ok := <-added:
		if !ok {
			t.Fatal("comment rejected")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("adding a comment waited behind the tree read")
	}
}

func TestCancelledScopedRequestReturnsPromptly(t *testing.T) {
	s := slowSession(t, 1500*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	start := time.Now()
	go func() {
		s.GetSessionInfoScoped(ctx, "all", virtualWorkingTreeCommitSHA)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
		if took := time.Since(start); took > 500*time.Millisecond {
			t.Fatalf("cancelled request returned after %v", took)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request kept waiting for the tree read")
	}
}
