package session

import (
	"context"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// slowListingVCS takes delay to list working-tree changes, like a large tree.
type slowListingVCS struct {
	vcs.VCS
	delay time.Duration
}

func (v *slowListingVCS) Name() string         { return "slow" }
func (v *slowListingVCS) HasStagingArea() bool { return false }
func (v *slowListingVCS) ChangedFilesOnDefaultInDir(string) ([]vcs.FileChange, error) {
	time.Sleep(v.delay)
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
	s := slowSession(t, 1500*time.Millisecond)
	go s.GetSessionInfo()
	time.Sleep(100 * time.Millisecond) // the probe is now reading the tree
	start := time.Now()
	if _, ok := s.AddComment("a.go", 1, 1, "", "body", "", ""); !ok {
		t.Fatal("comment rejected")
	}
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("adding a comment waited %v behind the tree read", took)
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
