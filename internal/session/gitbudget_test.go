package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// countGitCalls puts a git wrapper first on PATH that logs one line (its pid)
// per invocation and execs the real git. The returned func reads the count;
// pass reset=true to zero it.
func countGitCalls(t *testing.T) func(reset bool) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git-call budget uses a POSIX shell wrapper")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$$ $*\" >> '" + logPath + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func(reset bool) int {
		data, _ := os.ReadFile(logPath)
		if reset {
			_ = os.Remove(logPath)
		}
		return strings.Count(string(data), "\n")
	}
}

// newBudgetRepo builds a repository shaped like a large review: committed
// files on main, a feature branch with two commits, untracked files under
// newpkg/ and an ignored build/ directory. Files are generated with one awk
// per class rather than a process per file. The working directory moves into
// the repo for the test.
func newBudgetRepo(t *testing.T, files, untracked int) string {
	t.Helper()
	dir := initTestRepo(t)
	setHome(t, t.TempDir())
	generate := func(dirName, prefix string, count int) {
		if err := os.MkdirAll(filepath.Join(dir, dirName), 0o755); err != nil {
			t.Fatal(err)
		}
		script := `BEGIN { for (i = 0; i < n; i++) { f = d "/" p i ".go"; printf "package p\n\nfunc F%d() int { return %d }\n", i, i > f; close(f) } }`
		cmd := exec.Command("awk", "-v", "n="+itoa(count), "-v", "d="+filepath.ToSlash(filepath.Join(dir, dirName)), "-v", "p="+prefix, script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("awk: %v %s", err, out)
		}
	}
	generate("src", "file", files)
	writeFile(t, filepath.Join(dir, ".gitignore"), "build/\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base files")
	gitT(t, dir, "checkout", "-qb", "feature")
	for c := 0; c < 2; c++ {
		for i := c * 10; i < c*10+10; i++ {
			path := filepath.Join(dir, "src", "file"+itoa(i)+".go")
			writeFile(t, path, "package p\n\nfunc F"+itoa(i)+"() int { return -"+itoa(c+1)+" }\n")
		}
		gitT(t, dir, "commit", "-qam", "feature change "+itoa(c+1))
	}
	writeFile(t, filepath.Join(dir, "src", "file50.go"), "package p\n\nfunc F50() int { return 500 }\n")
	generate("newpkg", "new", untracked)
	generate("build", "out", 2000)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	return dir
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestGitBudgetHarnessCountsCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	calls := countGitCalls(t)
	dir := t.TempDir()
	for range 2 {
		if out, err := exec.Command("git", "-C", dir, "--version").CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	if got := calls(false); got != 2 {
		t.Fatalf("counted %d git calls, want 2", got)
	}
}

// TestGitBudgetBaseline logs today's git-call counts for the flows v1.0.3
// speeds up; later tests assert budgets against the same repo shape.
func TestGitBudgetBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	newBudgetRepo(t, 300, 100)
	calls := countGitCalls(t)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("new session: %d git calls", calls(true))
	sess.GetSessionInfoScoped(context.Background(), "all", virtualWorkingTreeCommitSHA)
	t.Logf("scoped info, commit=WT: %d git calls", calls(true))
	sess.GetCommits()
	t.Logf("commits: %d git calls", calls(true))
	if err := sess.ChangeBaseBranch("main"); err != nil {
		t.Fatal(err)
	}
	t.Logf("change base: %d git calls", calls(true))
}

func TestChangeBaseBranchUsesBatchedDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	newBudgetRepo(t, 300, 100)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := countGitCalls(t)
	if err := sess.ChangeBaseBranch("main"); err != nil {
		t.Fatal(err)
	}
	if log := gitCallLog(t); calls(true) > 8 {
		t.Errorf("base change ran more than 8 git commands:\n%s", log)
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	eager, lazy := 0, 0
	for _, f := range sess.Files {
		if f.Lazy {
			lazy++
			continue
		}
		eager++
		if f.Status == "modified" && len(f.DiffHunks) == 0 {
			t.Errorf("eager %s has no hunks", f.Path)
		}
	}
	if eager != lazyFileThreshold || lazy == 0 {
		t.Errorf("eager=%d lazy=%d, want %d eager and the rest lazy", eager, lazy, lazyFileThreshold)
	}
}

func TestScopedWorkingChangesCountsFromOneNumstat(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	dir := newBudgetRepo(t, 300, 100)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := countGitCalls(t)
	info := sess.GetSessionInfoScoped(context.Background(), "all", virtualWorkingTreeCommitSHA)
	// The scope probe still costs 5 calls here; the snapshot (Task 4) removes it.
	if log := gitCallLog(t); calls(true) > 10 {
		t.Errorf("scoped Working changes ran more than 10 git commands:\n%s", log)
	}
	counts := map[string][2]int{}
	for _, f := range info.Files {
		counts[f.Path] = [2]int{f.Additions, f.Deletions}
	}
	if got := counts["src/file50.go"]; got != [2]int{1, 1} {
		t.Errorf("src/file50.go counts = %v, want +1 -1", got)
	}
	if got := counts["newpkg/new0.go"]; got != [2]int{3, 0} {
		t.Errorf("untracked newpkg/new0.go counts = %v, want +3 -0 (dir %s)", got, dir)
	}
}

// gitCallLog returns the logged git invocations (pid and arguments), for
// diagnosing a budget failure.
func gitCallLog(t *testing.T) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if data, err := os.ReadFile(filepath.Join(dir, "calls.log")); err == nil {
			return string(data)
		}
	}
	return ""
}

func TestNewSessionUsesBatchedDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	newBudgetRepo(t, 300, 100)
	calls := countGitCalls(t)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A fixed cost: per-file diffs (25 eager files, 2 commands each) are gone.
	if log := gitCallLog(t); calls(true) > 18 {
		t.Errorf("opening a review ran more than 18 git commands:\n%s", log)
	}
	eager := 0
	for _, f := range sess.Files {
		if f.Lazy {
			continue
		}
		eager++
		if (f.Status == "modified" || f.Status == "untracked") && len(f.DiffHunks) == 0 {
			t.Errorf("eager %s (%s) has no hunks", f.Path, f.Status)
		}
	}
	if eager != lazyFileThreshold {
		t.Errorf("eager=%d, want %d", eager, lazyFileThreshold)
	}
}

func TestNewSessionBatchedHunksMatchPerFileDiff(t *testing.T) {
	dir := initTestRepo(t)
	t.Chdir(dir)
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nvar A = 1\n")
	gitT(t, dir, "add", "a.go")
	gitT(t, dir, "commit", "-qm", "a")
	// A staged edit plus an unstaged one, and an untracked file: the batched
	// eager hunks must equal what the per-file diff shows.
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nvar A = 2\n")
	gitT(t, dir, "add", "a.go")
	writeFile(t, filepath.Join(dir, "a.go"), "package a\n\nvar A = 2\nvar B = 3\n")
	writeFile(t, filepath.Join(dir, "new.go"), "package a\n")
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range sess.Files {
		want := vcs.FileDiffUnifiedNewFile(f.Content)
		if f.Status != "untracked" && f.Status != "added" {
			want, err = diffHunksForFile(f.Path, f.OldPath, f.Status, sess.BaseRef, sess.RepoRoot, false, sess.VCS)
			if err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(f.DiffHunks, want) {
			t.Errorf("%s (%s) hunks differ from the per-file diff:\n got %+v\nwant %+v", f.Path, f.Status, f.DiffHunks, want)
		}
	}
	if len(sess.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(sess.Files))
	}
}
