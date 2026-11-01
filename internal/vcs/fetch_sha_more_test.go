package vcs

import (
	"strings"
	"testing"
)

func TestTryGitFetch_FetchesMissingCommit(t *testing.T) {
	// Bare remote with two commits; clone only has the first.
	bare := t.TempDir()
	GitRun(t, bare, "init", "--bare")
	origin := InitTestRepo(t)
	firstSHA := GitRun(t, origin, "rev-parse", "HEAD")
	GitRun(t, origin, "remote", "add", "bare", bare)
	GitRun(t, origin, "push", "bare", "main")

	cloneDir := t.TempDir()
	GitRun(t, cloneDir, "clone", bare, ".")
	GitRun(t, cloneDir, "checkout", firstSHA)

	secondSHA := CommitAtForTest(t, origin, "second.txt", "2", "second")
	GitRun(t, origin, "push", "bare", "main")

	if HasObject(secondSHA, cloneDir) {
		t.Fatal("setup: clone should not have second commit yet")
	}
	if err := tryGitFetchForTest(cloneDir, "origin", secondSHA); err != nil {
		t.Fatalf("tryGitFetch: %v", err)
	}
	if !HasObject(secondSHA, cloneDir) {
		t.Fatal("second commit should be present after fetch")
	}
}

func TestEnsureSHAFetched_Git_FetchesFromOrigin(t *testing.T) {
	bare := t.TempDir()
	GitRun(t, bare, "init", "--bare")
	origin := InitTestRepo(t)
	firstSHA := GitRun(t, origin, "rev-parse", "HEAD")
	GitRun(t, origin, "remote", "add", "bare", bare)
	GitRun(t, origin, "push", "bare", "main")

	cloneDir := t.TempDir()
	GitRun(t, cloneDir, "clone", bare, ".")
	GitRun(t, cloneDir, "checkout", firstSHA)

	secondSHA := CommitAtForTest(t, origin, "fetch.txt", "x", "fetch me")
	GitRun(t, origin, "push", "bare", "main")

	g := &GitVCS{}
	if err := ensureSHAFetched(g, secondSHA, cloneDir, ""); err != nil {
		t.Fatalf("EnsureSHAFetched: %v", err)
	}
	if !g.HasObject(secondSHA, cloneDir) {
		t.Fatal("commit should be reachable after EnsureSHAFetched")
	}
}

func TestEnsureSHAFetched_Git_ForkURLStillMissing(t *testing.T) {
	dir := InitTestRepo(t)
	g := &GitVCS{}
	err := ensureSHAFetched(g, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", dir, "https://github.com/example/fork.git")
	if err == nil {
		t.Fatal("expected error when commit still missing after fetch attempts")
	}
	if !strings.Contains(err.Error(), "fork") {
		t.Errorf("error should mention fork URL, got: %v", err)
	}
}
