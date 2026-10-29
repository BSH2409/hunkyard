package localonly

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestGitGuardDisablesConfiguredProcesses(t *testing.T) {
	if root := os.Getenv("HUNKYARD_GIT_GUARD_TEST"); root != "" {
		Install()
		for _, args := range [][]string{{"status", "--porcelain"}, {"diff", "--no-ext-diff", "--no-textconv"}} {
			command := exec.Command("git", append([]string{"-C", root}, args...)...)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("guarded git: %s: %v", output, err)
			}
		}
		return
	}
	root := testutil.InitTestRepo(t)
	testutil.WriteFile(t, filepath.Join(root, "sample.txt"), "before\n")
	testutil.Git(t, root, "add", ".")
	testutil.Git(t, root, "commit", "-m", "sample")
	helper := filepath.Join(t.TempDir(), "helper")
	marker := filepath.Join(t.TempDir(), "called")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho called >> \"$HUNKYARD_GIT_GUARD_MARKER\"\ncat\n"), 0700); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(root, ".gitattributes"), "*.txt filter=network diff=network\n")
	testutil.Git(t, root, "config", "filter.network.clean", helper)
	testutil.Git(t, root, "config", "filter.network.required", "true")
	testutil.Git(t, root, "config", "core.fsmonitor", helper)
	testutil.WriteFile(t, filepath.Join(root, "sample.txt"), "after\n")
	command := exec.Command(os.Args[0], "-test.run=^TestGitGuardDisablesConfiguredProcesses$", "-test.count=1")
	command.Dir = root
	command.Env = append(os.Environ(), "HUNKYARD_GIT_GUARD_TEST="+root, "HUNKYARD_GIT_GUARD_MARKER="+marker)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("local Git guard: %s: %v", output, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("read-only Git spawned a configured process")
	}
}
