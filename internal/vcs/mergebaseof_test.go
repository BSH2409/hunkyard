package vcs

import (
	"testing"
)

// buildDriftedGitRepo creates a repo where the base branch has advanced past the
// point the PR branch forked from:
//
//	main:    C0 ── C1(forkpoint) ── M1(drift, base tip)
//	                  └── F1(PR head)
//
// It returns (repoDir, forkpoint=C1, baseTip=M1, prHead=F1).
func buildDriftedGitRepo(t *testing.T) (dir, forkpoint, baseTip, prHead string) {
	t.Helper()
	dir = InitTestRepo(t) // C0 on main
	forkpoint = CommitAtForTest(t, dir, "base.txt", "base\n", "forkpoint")
	GitRun(t, dir, "checkout", "-b", "feature")
	prHead = CommitAtForTest(t, dir, "feature.txt", "feat\n", "pr work")
	GitRun(t, dir, "checkout", "main")
	baseTip = CommitAtForTest(t, dir, "drift.txt", "drift\n", "base branch drift")
	return dir, forkpoint, baseTip, prHead
}

// TestMergeBaseOf_Git pins the two-arg merge-base: neither side is HEAD, and it
// returns the fork point, not the (drifted) base tip. This is the git path of
// the `hunkyard --pr` fix.
func TestMergeBaseOf_Git(t *testing.T) {
	ClearGitEnvForTest(t) // robust under git hooks that export GIT_DIR
	dir, forkpoint, baseTip, prHead := buildDriftedGitRepo(t)

	mb, err := MergeBaseOf(baseTip, prHead, dir)
	if err != nil {
		t.Fatalf("MergeBaseOf: %v", err)
	}
	if mb != forkpoint {
		t.Errorf("MergeBaseOf(baseTip, prHead) = %s, want forkpoint %s (got base tip %s?)",
			mb, forkpoint, baseTip)
	}

	// Symmetric: argument order must not matter.
	mbSwapped, err := MergeBaseOf(prHead, baseTip, dir)
	if err != nil {
		t.Fatalf("MergeBaseOf (swapped): %v", err)
	}
	if mbSwapped != forkpoint {
		t.Errorf("MergeBaseOf(prHead, baseTip) = %s, want forkpoint %s", mbSwapped, forkpoint)
	}
}
