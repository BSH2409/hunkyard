package vcs

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// countGit puts a counting git wrapper first on PATH; the returned func
// reports and resets the number of invocations.
func countGit(t *testing.T) func() int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git-call counting uses a POSIX shell wrapper")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho x >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		data, _ := os.ReadFile(log)
		_ = os.Remove(log)
		return strings.Count(string(data), "\n")
	}
}

func changedPaths(changes []FileChange) []string {
	var paths []string
	for _, c := range changes {
		paths = append(paths, c.Path+":"+c.Status)
	}
	slices.Sort(paths)
	return paths
}

func TestChangedFilesListsUntrackedOnce(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "src", "a.go"), "package a\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base")
	writeFile(t, filepath.Join(dir, "src", "a.go"), "package a\n\nvar X = 1\n")
	writeFile(t, filepath.Join(dir, "src", "new.go"), "package a\n")
	writeFile(t, filepath.Join(dir, "newdir", "one.go"), "package b\n")
	writeFile(t, filepath.Join(dir, "other", "skip.go"), "package c\n")

	selected := (&GitVCS{}).ForPaths([]string{"src", "newdir"})
	calls := countGit(t)
	all, err := changedFilesOnDefaultInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := calls(); n > 3 {
		t.Errorf("unselected change listing ran %d git commands, want at most 3", n)
	}
	want := []string{"newdir/one.go:untracked", "other/skip.go:untracked", "src/a.go:modified", "src/new.go:untracked"}
	if got := changedPaths(all); !slices.Equal(got, want) {
		t.Errorf("unselected changes = %v, want %v", got, want)
	}
	scoped, err := selected.selectedChanges(dir, "HEAD", "all")
	if err != nil {
		t.Fatal(err)
	}
	if n := calls(); n > 3 {
		t.Errorf("selected change listing ran %d git commands, want at most 3", n)
	}
	want = []string{"newdir/one.go:untracked", "src/a.go:modified", "src/new.go:untracked"}
	if got := changedPaths(scoped); !slices.Equal(got, want) {
		t.Errorf("selected changes = %v, want %v", got, want)
	}
}

func TestFilterAttributeOnUntrackedFileStillRejected(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, ".gitattributes"), "*.lfs filter=lfs\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "attributes")
	writeFile(t, filepath.Join(dir, "big.lfs"), "binary-ish\n")
	var filterErr *UnsupportedFilterError
	if _, err := changedFilesOnDefaultInDir(dir); !errors.As(err, &filterErr) {
		t.Fatalf("unselected listing: want UnsupportedFilterError, got %v", err)
	}
	if _, err := (&GitVCS{}).ForPaths([]string{"big.lfs"}).selectedChanges(dir, "HEAD", "all"); !errors.As(err, &filterErr) {
		t.Fatalf("selected listing: want UnsupportedFilterError, got %v", err)
	}
}

func TestAllTrackedFilesHonoursSelection(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "src", "a.go"), "package a\n")
	writeFile(t, filepath.Join(dir, "docs", "x.md"), "# x\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base")
	writeFile(t, filepath.Join(dir, "src", "untracked.go"), "package a\n")
	files, err := (&GitVCS{}).ForPaths([]string{"src"}).AllTrackedFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	if want := []string{"src/a.go", "src/untracked.go"}; !slices.Equal(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

func TestBranchListSkipsOriginHEAD(t *testing.T) {
	dir := initTestRepo(t)
	main := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	gitT(t, dir, "update-ref", "refs/remotes/origin/main", main)
	gitT(t, dir, "update-ref", "refs/remotes/origin/feature", main)
	gitT(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	remote, err := RemoteBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(remote)
	if want := []string{"feature", "main"}; !slices.Equal(remote, want) {
		t.Errorf("remote branches = %v, want %v", remote, want)
	}
	entries, err := RemoteBranchTips(&GitVCS{}, dir, "none")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(entries, func(e BranchEntry) bool { return strings.HasSuffix(e.Name, "main") }) {
		t.Fatalf("precondition: remote tips should list origin/main, got %+v", entries)
	}
	for _, e := range entries {
		if e.Name == "origin" || strings.HasSuffix(e.Name, "/HEAD") {
			t.Errorf("remote tips list %q", e.Name)
		}
	}
	local, err := LocalBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(local, "main") {
		t.Errorf("local branches = %v, want main", local)
	}
}

func TestWorkingTreeFingerprintSeesEditsToDirtyFilesAndNestedUntracked(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base")
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nvar X = 1\n")
	writeFile(t, filepath.Join(dir, "newpkg", "one.go"), "package b\n")
	for _, vc := range []*GitVCS{{}, (&GitVCS{}).ForPaths([]string{"a.go", "newpkg"})} {
		t.Chdir(dir)
		first := vc.WorkingTreeFingerprint()
		writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nvar X = 22\n")
		second := vc.WorkingTreeFingerprint()
		if second == first {
			t.Errorf("paths=%v: editing an already-modified file did not change the fingerprint", vc.paths)
		}
		writeFile(t, filepath.Join(dir, "newpkg", "two.go"), "package b\n")
		if third := vc.WorkingTreeFingerprint(); third == second {
			t.Errorf("paths=%v: a new file inside an untracked folder did not change the fingerprint", vc.paths)
		}
		_ = os.Remove(filepath.Join(dir, "newpkg", "two.go"))
	}
}
