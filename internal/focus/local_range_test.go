package focus

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestLocalRangeSemantics(t *testing.T) {
	root := testutil.InitTestRepo(t)
	initial := testutil.Git(t, root, "rev-parse", "HEAD")
	testutil.Git(t, root, "checkout", "-b", "feature")
	testutil.WriteFile(t, filepath.Join(root, "feature.go"), "package feature\n")
	testutil.Git(t, root, "add", ".")
	testutil.Git(t, root, "commit", "-m", "feature")
	head := testutil.Git(t, root, "rev-parse", "HEAD")
	testutil.Git(t, root, "checkout", "main")
	testutil.WriteFile(t, filepath.Join(root, "base.go"), "package base\n")
	testutil.Git(t, root, "add", ".")
	testutil.Git(t, root, "commit", "-m", "base advanced")
	base := testutil.Git(t, root, "rev-parse", "HEAD")
	v := &vcs.GitVCS{}
	exact, err := ResolveFocus("", "main..feature", "", false, v, root)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := ResolveFocus("", "main...feature", "", false, v, root)
	if err != nil {
		t.Fatal(err)
	}
	if exact.BaseSHA != base || pr.BaseSHA != initial || exact.HeadSHA != head || pr.HeadSHA != head {
		t.Fatal("comparison did not pin effective full commit IDs")
	}
	if !strings.Contains(pr.Label, "...") {
		t.Fatal("merge-base comparison label lost its mode")
	}
	exactFiles, err := v.ChangedFilesBetweenSHAs(exact.BaseSHA, exact.HeadSHA, root)
	if err != nil {
		t.Fatal(err)
	}
	prFiles, err := v.ChangedFilesBetweenSHAs(pr.BaseSHA, pr.HeadSHA, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(exactFiles) != 2 || len(prFiles) != 1 || prFiles[0].Path != "feature.go" {
		t.Fatalf("diverged ranges: exact=%v merge-base=%v", exactFiles, prFiles)
	}
}
