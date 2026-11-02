package vcs

import "testing"

func commitFileStatuses(t *testing.T, sha, dir string) map[string]string {
	t.Helper()
	changes, err := ChangedFilesForCommit(sha, dir)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, c := range changes {
		paths[c.Path] = c.Status
	}
	return paths
}

// The commit picker lists root and merge commits; selecting one shows what it
// added (root) or what it brought in over its first parent (merge).
func TestChangedFilesForRootAndMergeCommits(t *testing.T) {
	dir := InitTestRepo(t)
	root := CommitAtForTest(t, dir, "a.txt", "a\n", "root")
	if got := commitFileStatuses(t, root, dir); got["a.txt"] != "added" {
		t.Fatalf("root commit files = %v, want a.txt added", got)
	}
	GitRun(t, dir, "checkout", "-q", "-b", "feature")
	CommitAtForTest(t, dir, "b.txt", "b\n", "feature work")
	GitRun(t, dir, "checkout", "-q", "-")
	CommitAtForTest(t, dir, "c.txt", "c\n", "main work")
	GitRun(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	merge := GitRun(t, dir, "rev-parse", "HEAD")
	if got := commitFileStatuses(t, merge, dir); got["b.txt"] != "added" || len(got) != 1 {
		t.Fatalf("merge commit files = %v, want b.txt added over the first parent", got)
	}
}
