package session

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func snapshotPaths(changes []vcs.FileChange) []string {
	var paths []string
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	slices.Sort(paths)
	return paths
}

func TestSnapshotSingleflight(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	dir := newBudgetRepo(t, 50, 20)
	s := &Session{}
	vc := &vcs.GitVCS{}
	calls := countGitCalls(t)
	single, err := s.snapshotFor(context.Background(), vc, dir, "main", true, true)
	if err != nil {
		t.Fatal(err)
	}
	once := calls(true)
	s.invalidateWorkingTreeSnapshot()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.snapshotFor(context.Background(), vc, dir, "main", false, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	// 20 key probes plus at most one shared reading.
	if got := calls(true); got > 20+once {
		t.Fatalf("20 concurrent readers ran %d git commands; one reading costs %d", got, once)
	}
	if len(single.changes) == 0 {
		t.Fatal("snapshot found no changes")
	}
}

func TestSnapshotInvalidatesOnHeadAndIndex(t *testing.T) {
	dir := newBudgetRepo(t, 25, 0)
	s := &Session{}
	previous := keyProbeReuse
	keyProbeReuse = 0 // probe on every lookup: this test moves HEAD and the index back to back
	t.Cleanup(func() { keyProbeReuse = previous })
	vc := &vcs.GitVCS{}
	before, err := s.snapshotChanges(context.Background(), vc, dir, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("precondition: the branch should start with changes against main")
	}
	// Staging a new file moves the index.
	writeFile(t, filepath.Join(dir, "staged.go"), "package p\n")
	gitT(t, dir, "add", "staged.go")
	staged, err := s.snapshotChanges(context.Background(), vc, dir, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(snapshotPaths(staged), "staged.go") {
		t.Fatalf("staged file missing after index moved: %v", snapshotPaths(staged))
	}
	// Committing everything moves HEAD and clears the changes.
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "all in")
	after, err := s.snapshotChanges(context.Background(), vc, dir, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("changes after commit: %v", snapshotPaths(after))
	}
}

func TestFreshSnapshotSeesReviewerEdit(t *testing.T) {
	dir := newBudgetRepo(t, 25, 0)
	s := &Session{}
	vc := &vcs.GitVCS{}
	previous := freshSnapshotWindow
	freshSnapshotWindow = 0
	t.Cleanup(func() { freshSnapshotWindow = previous })
	if _, err := s.snapshotChanges(context.Background(), vc, dir, "main", false); err != nil {
		t.Fatal(err)
	}
	// An untracked file moves neither HEAD nor the index.
	writeFile(t, filepath.Join(dir, "later.go"), "package p\n")
	cached, err := s.snapshotChanges(context.Background(), vc, dir, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	// HEAD and the index did not move, so the cached reading is reused: it
	// cannot know about the new file yet. Seeing it means the cache was skipped.
	if slices.Contains(snapshotPaths(cached), "later.go") {
		t.Fatal("a non-fresh reading bypassed the snapshot cache")
	}
	fresh, _ := s.snapshotChanges(context.Background(), vc, dir, "main", true)
	if !slices.Contains(snapshotPaths(fresh), "later.go") {
		t.Fatalf("a reviewer-initiated reading missed the new file: %v", snapshotPaths(fresh))
	}
	// The watcher's invalidation also forces a new reading.
	writeFile(t, filepath.Join(dir, "later2.go"), "package p\n")
	s.invalidateWorkingTreeSnapshot()
	if got, _ := s.snapshotChanges(context.Background(), vc, dir, "main", false); !slices.Contains(snapshotPaths(got), "later2.go") {
		t.Fatalf("invalidated reading missed the new file: %v", snapshotPaths(got))
	}
}

func TestVirtualCommitDiffsUntrackedFile(t *testing.T) {
	dir := newBudgetRepo(t, 25, 3)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hunks, ok := scopedHunksForCommit(vcs.FileChange{Path: "newpkg/new0.go", Status: "untracked"}, virtualWorkingTreeCommitSHA, dir, sess.VCS, false)
	if !ok || len(hunks) != 1 || len(hunks[0].Lines) != 3 {
		t.Fatalf("untracked file in Working changes: ok=%v hunks=%+v", ok, hunks)
	}
}

func TestGitBudgetScopedAndCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	newBudgetRepo(t, 300, 100)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := countGitCalls(t)
	sess.GetSessionInfoScoped(context.Background(), "all", virtualWorkingTreeCommitSHA)
	if log := gitCallLog(t); calls(true) > 11 {
		t.Errorf("cold Working changes ran more than 11 git commands:\n%s", log)
	}
	sess.GetCommits()
	if log := gitCallLog(t); calls(true) > 2 {
		t.Errorf("warm commit list ran more than 2 git commands:\n%s", log)
	}
	sess.GetFileDiffSnapshotScoped("newpkg/new1.go", "all", virtualWorkingTreeCommitSHA, false)
	if log := gitCallLog(t); calls(true) > 2 {
		t.Errorf("warm Working changes file diff ran more than 2 git commands:\n%s", log)
	}
	// Within one reviewer action, a second fresh request reuses the reading.
	start := time.Now()
	sess.GetSessionInfoScoped(context.Background(), "all", virtualWorkingTreeCommitSHA)
	if log := gitCallLog(t); time.Since(start) < freshSnapshotWindow && calls(true) > 3 {
		t.Errorf("repeat Working changes ran more than 3 git commands:\n%s", log)
	}
}
