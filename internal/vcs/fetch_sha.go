package vcs

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/BSH2409/hunkyard/internal/localonly"
)

// EnsureSHAFetched ensures sha is reachable in the local object store,
// attempting auto-fetch from origin (and forkURL when set) when missing.
func EnsureSHAFetched(vcsInst VCS, sha, repoRoot, forkURL string) error {
	if vcsInst == nil {
		return nil
	}
	if vcsInst.HasObject(sha, repoRoot) {
		return nil
	}
	if vcsInst.Name() != "git" {
		return fmt.Errorf("commit %s not present locally (auto-fetch not supported for vcs=%q)", sha, vcsInst.Name())
	}

	if err := tryGitFetch(repoRoot, "origin", sha); err == nil &&
		vcsInst.HasObject(sha, repoRoot) {
		return nil
	}

	if forkURL != "" {
		if err := tryGitFetch(repoRoot, forkURL, sha); err == nil &&
			vcsInst.HasObject(sha, repoRoot) {
			return nil
		}
		return fmt.Errorf("commit %s not present locally; tried origin and fork %s — manual fetch required", sha, forkURL)
	}
	return fmt.Errorf("commit %s not present locally; manual fetch required (run `git fetch <remote> %s`)", sha, sha)
}

func tryGitFetch(repoRoot, remote, sha string) error {
	if localonly.Enabled() {
		return fmt.Errorf("local-only build: commit must already exist locally")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "fetch", remote, sha)
	cmd.Dir = repoRoot
	return cmd.Run()
}
