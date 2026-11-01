package session

import (
	"context"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestRangeCacheHitOnRepick(t *testing.T) {
	if testing.Short() {
		t.Skip("git-call budget")
	}
	dir := newBudgetRepo(t, 25, 0)
	shas := strings.Fields(gitT(t, dir, "rev-list", "-2", "HEAD"))
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	calls := countGitCalls(t)
	first := sess.GetSessionInfoScoped(ctx, "", shas[0])
	if len(first.Files) == 0 {
		t.Fatal("precondition: the commit should change files")
	}
	sess.GetSessionInfoScoped(ctx, "", shas[1])
	sess.GetFileDiffSnapshotScoped(first.Files[0].Path, "", shas[0], false)
	if log := gitCallLog(t); !strings.Contains(log, " diff") {
		t.Fatalf("precondition: the first pick should run git diff:\n%s", log)
	}
	calls(true)
	again := sess.GetSessionInfoScoped(ctx, "", shas[0])
	sess.GetFileDiffSnapshotScoped(first.Files[0].Path, "", shas[0], false)
	log := gitCallLog(t)
	calls(true)
	if strings.Contains(log, " diff") {
		t.Fatalf("re-picking a commit ran diffs again:\n%s", log)
	}
	if len(again.Files) != len(first.Files) || again.Files[0].Additions != first.Files[0].Additions {
		t.Fatalf("cached commit view differs: %+v vs %+v", again.Files, first.Files)
	}
}

func TestRangeCacheNeverStoresWorkingTree(t *testing.T) {
	sha := strings.Repeat("a", 40)
	other := strings.Repeat("b", 40)
	for commit, want := range map[string]bool{
		sha:                         true,
		sha + ".." + other:          true,
		virtualWorkingTreeCommitSHA: false,
		"HEAD":                      false,
		"main..HEAD":                false,
		"":                          false,
		sha[:7]:                     false,
	} {
		if _, ok := immutableRange(commit); ok != want {
			t.Errorf("immutableRange(%q) = %v, want %v", commit, ok, want)
		}
	}
	key, _ := immutableRange(sha)
	ws := key
	ws.IgnoreWS = true
	if key.String("hunks", "a.go") == ws.String("hunks", "a.go") {
		t.Error("ignore-whitespace hunks share a cache key with plain hunks")
	}
}
