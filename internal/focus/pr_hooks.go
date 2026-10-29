package focus

import "github.com/BSH2409/hunkyard/internal/vcs"

// ChangeResolveInfo carries the pull-request metadata needed to build a Focus
// without importing internal/github.
type ChangeResolveInfo struct {
	URL               string
	Number            int
	Title             string
	BaseRefOid        string
	HeadRefOid        string
	BaseRefName       string
	HeadRefName       string
	HeadRepoURL       string
	BaseRepoProject   string
	HeadRepoProject   string
	HeadRepoHost      string
	IsCrossRepository bool
}

var (
	// FetchPRHook resolves a --pr <num|url> spec. The raw CLI value is passed
	// through so URL-derived owner/repo survives.
	FetchPRHook     func(spec string) (ChangeResolveInfo, error)
	IsStackedPRHook func(info ChangeResolveInfo, v vcs.VCS) bool
)

// SetPRResolveHooks wires PR resolution from cmd/hunkyard to break focus↔github cycles.
func SetPRResolveHooks(
	fetch func(spec string) (ChangeResolveInfo, error),
	stacked func(info ChangeResolveInfo, v vcs.VCS) bool,
) {
	FetchPRHook = fetch
	IsStackedPRHook = stacked
}
