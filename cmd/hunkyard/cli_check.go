package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// slowGitStatus is how long one `git status` may take before `hunkyard check`
// suggests git's untracked cache: Hunkyard runs status on every review load
// and while it watches the working tree.
const slowGitStatus = 500 * time.Millisecond

// gitStatusTimer times one `git status` in dir; ok is false outside a
// repository. A variable so tests can stand in for a slow tree.
var gitStatusTimer = func(dir string) (time.Duration, bool) {
	start := time.Now()
	if err := exec.Command("git", "-C", dir, "status", "--porcelain", "-uall").Run(); err != nil {
		return 0, false
	}
	return time.Since(start), true
}

// untrackedCacheHint suggests core.untrackedCache when git status is slow in
// dir and the cache is not already on; otherwise it returns "".
func untrackedCacheHint(dir string) string {
	elapsed, ok := gitStatusTimer(dir)
	if !ok || elapsed <= slowGitStatus {
		return ""
	}
	out, _ := exec.Command("git", "-C", dir, "config", "--bool", "core.untrackedCache").Output()
	if strings.TrimSpace(string(out)) == "true" {
		return ""
	}
	return fmt.Sprintf("Tip: git status took %d ms here. Speed up reviews with:\n     git config core.untrackedCache true\n", elapsed.Milliseconds())
}
