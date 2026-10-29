package vcs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirtyComparisonsRejectExecutableFilters(t *testing.T) {
	root := InitTestRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("README.md filter=fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Match the binary's disabled filter environment; no executable is invoked.
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "filter.fixture.clean")
	t.Setenv("GIT_CONFIG_VALUE_0", "")
	t.Setenv("GIT_CONFIG_KEY_1", "filter.fixture.required")
	t.Setenv("GIT_CONFIG_VALUE_1", "false")
	_, _, _, err := initialGitChangesAndScopes("HEAD", root)
	if err == nil || !strings.Contains(err.Error(), "filter") {
		t.Fatal("filtered dirty comparison silently changed meaning", err)
	}
	if _, err := FileDiffUnifiedCtx(context.Background(), "README.md", "", "HEAD", root, false); err == nil {
		t.Fatal("filtered path diff accepted")
	}
	if _, err := ChangedFilesBetweenSHAs("HEAD", "HEAD", root); err != nil {
		t.Fatal("commit-only comparisons must remain usable", err)
	}
}
