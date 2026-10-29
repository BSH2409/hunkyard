package session

import (
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestLocalRangeMetadataCannotMislabelExactComparison(t *testing.T) {
	root := initTestRepo(t)
	initial := gitT(t, root, "rev-parse", "HEAD")
	gitT(t, root, "checkout", "-b", "feature")
	head := commitAt(t, root, "feature.go", "package feature\n", "feature")
	gitT(t, root, "checkout", "main")
	base := commitAt(t, root, "base.go", "package base\n", "advanced base")
	focus := Focus{Kind: FocusRange, RangeMode: "merge_base", RequestedBaseSHA: base, BaseSHA: base, HeadSHA: head}
	if err := canonicalizeFocusSHAs(&focus, &vcs.GitVCS{}, root, false); err != nil {
		t.Fatal(err)
	}
	if focus.BaseSHA != initial || focus.RequestedBaseSHA != base {
		t.Fatal("merge-base metadata described an exact comparison")
	}
	focus.RangeMode = "unknown"
	if err := canonicalizeFocusSHAs(&focus, &vcs.GitVCS{}, root, false); err == nil {
		t.Fatal("unknown comparison mode accepted")
	}
}
