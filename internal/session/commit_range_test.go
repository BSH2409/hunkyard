package session

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// TestGetSessionInfoScoped_CommitRange builds a feature branch with three
// commits over a base and asserts that scoping by a commit RANGE
// (C1sha..C3sha) returns the union of files changed across those commits,
// matching ChangedFilesBetweenSHAs, while a single-commit param still returns
// only that commit's files.
func TestGetSessionInfoScoped_CommitRange(t *testing.T) {
	dir := initTestRepo(t)
	base := gitT(t, dir, "rev-parse", "HEAD")

	c1 := commitAt(t, dir, "a.txt", "a1\n", "C1: add a")
	c2 := commitAt(t, dir, "b.txt", "b1\n", "C2: add b")
	c3 := commitAt(t, dir, "c.txt", "c1\n", "C3: add c")

	s := &Session{
		Mode:     "git",
		RepoRoot: dir,
		BaseRef:  base,
		Branch:   "main",
		VCS:      &vcs.GitVCS{},
	}

	// Range C1..C3 should return the union of files touched by C2 and C3
	// (a.txt was introduced by C1 itself, so it's part of the C1 tree, not the
	// diff from C1). Assert against ChangedFilesBetweenSHAs as the source of truth.
	wantChanges, err := vcs.ChangedFilesBetweenSHAs(c1, c3, dir)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := pathsOf(wantChanges)

	rangeInfo := s.GetSessionInfoScoped(context.Background(), "", c1+".."+c3)
	gotPaths := filePaths(rangeInfo.Files)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Errorf("range C1..C3 files = %v, want %v", gotPaths, wantPaths)
	}

	// A single commit (C2) should return only C2's files.
	singleInfo := s.GetSessionInfoScoped(context.Background(), "", c2)
	singleWant, err := vcs.ChangedFilesForCommit(c2, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filePaths(singleInfo.Files), pathsOf(singleWant); !reflect.DeepEqual(got, want) {
		t.Errorf("single commit C2 files = %v, want %v", got, want)
	}
	if want := []string{"b.txt"}; !reflect.DeepEqual(filePaths(singleInfo.Files), want) {
		t.Errorf("single commit C2 files = %v, want %v", filePaths(singleInfo.Files), want)
	}
}

func pathsOf(changes []vcs.FileChange) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = c.Path
	}
	sort.Strings(out)
	return out
}

func filePaths(files []SessionFileInfo) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	sort.Strings(out)
	return out
}
