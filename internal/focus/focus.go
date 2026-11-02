package focus

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// InheritedScope is the focus metadata stamped on comments authored via
// `hunkyard comment`. All fields empty for working-tree mode. Forge,
// ChangeNumber, and BaseSHA flow through to the comment's FocusKey so
// view-scoped visibility matches the daemon's view.
//
// Defined in session; aliased here for focus helpers.

// prURLRe matches GitHub PR URLs like https://github.com/owner/repo/pull/123.
// The trailing group accepts /, ?, # so suffixes like /files or ?diff=split work.
var prURLRe = regexp.MustCompile(`^https?://[^/]+/[^/]+/[^/]+/pull/(\d+)(?:[/?#].*)?$`)

// parsePRSpec resolves --pr <num|url> to a numeric PR number. Returns an error
// for non-numeric, non-positive, or unparsable inputs.
func parsePRSpec(spec string) (int, error) {
	if m := prURLRe.FindStringSubmatch(spec); m != nil {
		return strconv.Atoi(m[1])
	}
	n, err := strconv.Atoi(spec)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid --pr value %q (expected number or https://.../pull/N URL)", spec)
	}
	return n, nil
}

// parseRangeSpec splits "base..head" with strict validation. Three dots ("...")
// are explicitly rejected — symmetric-difference is not what users expect.
func parseRangeSpec(spec string) (base, head string, err error) {
	if strings.Contains(spec, "...") {
		return "", "", fmt.Errorf("--range expects two-dot syntax (base..head), got %q", spec)
	}
	parts := strings.SplitN(spec, "..", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Count(spec, "..") != 1 || strings.HasPrefix(parts[0], "-") || strings.HasPrefix(parts[1], "-") {
		return "", "", fmt.Errorf("invalid --range value %q (expected base..head)", spec)
	}
	return parts[0], parts[1], nil
}

// normalizeScopeSpec is the shared parser for the --scope CLI flag. It maps
// the raw string to a DiffScope and reports whether the value referred to
// the working-tree pseudo-scope (only valid for `hunkyard comment`, not for
// starting a session). Callers gate on the bool depending on which surface
// they implement.
//
// "" defaults to layer. "full-stack" / "full_stack" both map to full_stack.
// "working-tree" / "working_tree" map to (DiffScopeLayer, true) — the
// DiffScope value is irrelevant because callers that accept working-tree
// inspect the bool first.
func normalizeScopeSpec(s string) (DiffScope, bool, error) {
	switch s {
	case "", "layer":
		return DiffScopeLayer, false, nil
	case "full-stack", "full_stack":
		return DiffScopeFullStack, false, nil
	case "working-tree", "working_tree":
		return DiffScopeLayer, true, nil
	default:
		return "", false, fmt.Errorf("invalid --scope value %q", s)
	}
}

// parseScopeSpec maps the --scope flag to a DiffScope for the session-start
// surface (`hunkyard --pr <n> --scope=...`). Rejects "working-tree" because it's
// not a valid scope for a focus — sessions either run in working-tree mode
// (no --pr/--range) or in range mode with layer/full-stack.
func parseScopeSpec(s string) (DiffScope, error) {
	scope, isWorkingTree, err := normalizeScopeSpec(s)
	if err != nil {
		return "", fmt.Errorf("%w (expected layer or full-stack)", err)
	}
	if isWorkingTree {
		return "", fmt.Errorf("invalid --scope value %q (expected layer or full-stack)", s)
	}
	return scope, nil
}

// shortSHA returns a 7-char SHA prefix for display. Defensive against short input.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ResolveFocus turns CLI inputs into a *Focus, or nil for working-tree default.
// --pr and --range are mutually exclusive.
//
// remoteFiles=true skips local-fetch / object presence checks because file
// content reads will go through the GitHub API instead of local git.
func ResolveFocus(prSpec, rangeSpec, scopeSpec string, remoteFiles bool, v vcs.VCS, repoRoot string) (*Focus, error) {
	if prSpec != "" && rangeSpec != "" {
		return nil, fmt.Errorf("--pr and --range are mutually exclusive")
	}
	scope, err := parseScopeSpec(scopeSpec)
	if err != nil {
		return nil, err
	}
	if scopeSpec != "" && rangeSpec != "" {
		fmt.Fprintln(os.Stderr, "Note: --scope is ignored with --range; pass an explicit base..head instead")
	}
	if prSpec != "" {
		return resolveFocusFromPR(prSpec, scope, remoteFiles, v, repoRoot)
	}
	if rangeSpec != "" {
		return resolveFocusFromRange(rangeSpec, remoteFiles, v, repoRoot)
	}
	return nil, nil
}

func resolveFocusFromPR(prSpec string, scope DiffScope, remoteFiles bool, v vcs.VCS, repoRoot string) (*Focus, error) {
	prNum, err := parsePRSpec(prSpec)
	if err != nil {
		return nil, err
	}
	if FetchPRHook == nil {
		return nil, fmt.Errorf("resolving PR #%d: PR fetch not wired", prNum)
	}
	info, err := FetchPRHook(prSpec)
	if err != nil {
		return nil, fmt.Errorf("resolving PR #%d: %w", prNum, err)
	}
	forkURL := ""
	if info.IsCrossRepository {
		forkURL = info.HeadRepoURL
	}
	// In --remote mode, file content reads go through `gh api`, so we don't
	// need (and don't want) the local git-fetch side effects.
	if !remoteFiles {
		if err := vcs.EnsureSHAFetched(v, info.BaseRefOid, repoRoot, ""); err != nil {
			return nil, err
		}
		if err := vcs.EnsureSHAFetched(v, info.HeadRefOid, repoRoot, forkURL); err != nil {
			return nil, err
		}
	}

	defaultBranch := ""
	if v != nil {
		defaultBranch = v.DefaultBranch()
	}
	defaultSHA, _ := vcs.ResolveDefaultBranchSHA(v, repoRoot, defaultBranch)
	if scope == DiffScopeFullStack && defaultSHA == "" {
		return nil, fmt.Errorf("--scope=full-stack requires a resolvable default branch tip; got none for %q (detached HEAD or no remote?)", defaultBranch)
	}

	// GitHub renders PRs from the merge-base to the head.
	// Using info.BaseRefOid directly would diff base..head (two-dot), folding
	// in any commits that landed on the base branch since this branch diverged
	// — surfacing files that aren't part of the PR. Pin the layer diff base to
	// the merge-base so the diff matches GitHub regardless of rebase state.
	baseSHA := info.BaseRefOid
	if v != nil {
		// Best-effort: in --remote mode the base/head objects may not be local
		// (EnsureSHAFetched was skipped above), so merge-base can fail. We then
		// fall back to BaseRefOid — a two-dot diff (the case #659 fixed locally).
		if mb, err := v.MergeBaseOf(info.BaseRefOid, info.HeadRefOid, repoRoot); err == nil && mb != "" {
			baseSHA = mb
		}
	}

	return &Focus{
		Kind:              FocusRange,
		Forge:             "github",
		ChangeNumber:      info.Number,
		PRURL:             info.URL,
		Label:             fmt.Sprintf("PR #%d: %s", info.Number, info.Title),
		BaseSHA:           baseSHA,
		HeadSHA:           info.HeadRefOid,
		DefaultSHA:        defaultSHA,
		ForkURL:           forkURL,
		RemoteProject:     info.HeadRepoProject,
		RemoteBaseProject: info.BaseRepoProject,
		RemoteHost:        info.HeadRepoHost,
		BaseRefName:       info.BaseRefName,
		HeadRefName:       info.HeadRefName,
		DiffScope:         scope,
		IsStacked:         IsStackedPRHook != nil && IsStackedPRHook(info, v),
	}, nil
}

//nolint:gocyclo // Exact and merge-base comparisons share the revision validation flow.
func resolveFocusFromRange(rangeSpec string, remoteFiles bool, v vcs.VCS, repoRoot string) (*Focus, error) {
	separator := ".."
	mode := "exact"
	parseSpec := rangeSpec
	if strings.Contains(rangeSpec, "...") {
		if strings.Count(rangeSpec, "...") != 1 {
			return nil, fmt.Errorf("invalid --range comparison")
		}
		separator = "..."
		mode = "merge_base"
		parseSpec = strings.Replace(rangeSpec, "...", "..", 1)
	}
	base, head, err := parseRangeSpec(parseSpec)
	if err != nil {
		return nil, err
	}
	// In --remote mode, content reads come from the GitHub API; we can't
	// prove the SHAs exist locally because we don't intend to use them.
	if !remoteFiles && v != nil && repoRoot != "" {
		if !v.HasObject(base, repoRoot) {
			return nil, fmt.Errorf("base SHA %s not present locally", base)
		}
		if !v.HasObject(head, repoRoot) {
			return nil, fmt.Errorf("head SHA %s not present locally", head)
		}
		// Resolve branch names / abbreviated SHAs to full OIDs so Focus and
		// comment focus_keys stay stable when the UI later re-enters with OIDs.
		baseOID, err := vcs.ResolveCommitOID(v, base, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving base %q: %w", base, err)
		}
		headOID, err := vcs.ResolveCommitOID(v, head, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving head %q: %w", head, err)
		}
		base, head = baseOID, headOID
	}
	requestedBase := base
	if mode == "merge_base" {
		if remoteFiles || v == nil || repoRoot == "" {
			return nil, fmt.Errorf("three-dot comparison requires a local repository")
		}
		base, err = v.MergeBaseOf(base, head, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve local merge base: %w", err)
		}
		if base == "" {
			return nil, fmt.Errorf("selected commits have no local merge base")
		}
	}
	return &Focus{
		Kind:             FocusRange,
		BaseSHA:          base,
		HeadSHA:          head,
		Label:            fmt.Sprintf("%s%s%s", shortSHA(requestedBase), separator, shortSHA(head)),
		RangeMode:        mode,
		RequestedBaseSHA: requestedBase,
		RangeSpec:        rangeSpec,
		DiffScope:        DiffScopeLayer,
		IsStacked:        false,
	}, nil
}
