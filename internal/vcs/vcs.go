package vcs

import "context"

// VCS abstracts version control operations. GitVCS is the only implementation;
// each method corresponds to an existing package-level function in git.go, and
// the interface lets tests substitute a fake backend.
type VCS interface {
	// Name returns the VCS identifier ("git").
	Name() string

	// RepoRoot returns the absolute path to the repository root.
	RepoRoot() (string, error)

	// CurrentBranch returns the name of the current branch.
	CurrentBranch() string

	// DefaultBranch returns the name of the default branch (e.g. "main", "master").
	DefaultBranch() string

	// SetDefaultBranchOverride overrides the default branch detection.
	SetDefaultBranchOverride(branch string)

	// GetDefaultBranchOverride returns the current default branch override, if any.
	GetDefaultBranchOverride() string

	// MergeBase returns the merge-base commit between HEAD and the given ref.
	MergeBase(ref string) (string, error)

	// MergeBaseOf returns the merge-base (best common ancestor) of two
	// arbitrary revisions, computed in dir. Unlike MergeBase, neither side is
	// pinned to the working-tree head — used to derive a PR's diff base as
	// merge-base(base, head) so the layer diff matches GitHub's three-dot PR
	// diff regardless of whether the branch is rebased.
	MergeBaseOf(a, b, dir string) (string, error)

	// DefaultBaseRef returns the best ref to use as the diff base for the
	// auto-detected default branch. For git, prefers `origin/<branch>` when
	// the remote-tracking ref exists locally, since the local default branch
	// is often stale.
	DefaultBaseRef() string

	// ChangedFilesOnDefaultInDir returns changed files when on the default branch.
	ChangedFilesOnDefaultInDir(dir string) ([]FileChange, error)

	// ChangedFilesFromBaseInDir returns files changed between baseRef and the working tree.
	ChangedFilesFromBaseInDir(baseRef, dir string) ([]FileChange, error)

	// ChangedFilesScoped returns changed files for a specific scope (branch, staged, unstaged).
	ChangedFilesScoped(scope, baseRef string) ([]FileChange, error)

	// ChangedFilesForCommit returns the files changed in a single commit.
	ChangedFilesForCommit(sha, dir string) ([]FileChange, error)

	// FileDiffUnified returns parsed diff hunks for a file against a base ref.
	// When ignoreWhitespace is true, whitespace-only changes collapse to context.
	FileDiffUnified(path, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error)

	// FileDiffUnifiedCtx is like FileDiffUnified but accepts a context for timeout control.
	// When ignoreWhitespace is true, whitespace-only changes collapse to context.
	FileDiffUnifiedCtx(ctx context.Context, path, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error)

	// FileDiffScoped returns diff hunks for a file using a scope-appropriate diff command.
	// When ignoreWhitespace is true, whitespace-only changes collapse to context.
	FileDiffScoped(path, scope, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error)

	// FileDiffForCommit returns diff hunks for a file in a single commit.
	// When ignoreWhitespace is true, whitespace-only changes collapse to context.
	FileDiffForCommit(path, sha, dir string, ignoreWhitespace bool) ([]DiffHunk, error)

	// FileDiffUnifiedNewFile returns diff hunks showing an entire file as added.
	FileDiffUnifiedNewFile(path string) ([]DiffHunk, error)

	// CommitLog returns the commits between baseRef and headRef, newest first.
	// If headRef is empty, the VCS's working-tree head (e.g. git HEAD) is used.
	CommitLog(baseRef, headRef, dir string) ([]CommitInfo, error)

	// WorkingTreeFingerprint returns a string representing the current working tree state.
	WorkingTreeFingerprint() string

	// UntrackedFiles returns untracked files, running from the specified directory.
	UntrackedFiles(dir string) ([]FileChange, error)

	// AllTrackedFiles returns all tracked files plus untracked non-ignored files.
	AllTrackedFiles(dir string) ([]string, error)

	// RemoteBranches returns the names of all remote branches.
	RemoteBranches(dir string) ([]string, error)

	// DiffNumstat returns per-file addition/deletion counts.
	DiffNumstat(baseRef, dir string) (map[string]NumstatEntry, error)

	// DiffNumstatBetweenSHAs returns per-file addition/deletion counts for the
	// fixed range baseSHA..headSHA (not vs the working tree).
	DiffNumstatBetweenSHAs(baseSHA, headSHA, dir string) (map[string]NumstatEntry, error)

	// UserName returns the VCS-configured user name.
	UserName() string

	// FileContentAtRef returns the content of a file at a given ref/revision.
	FileContentAtRef(path, ref, dir string) (string, error)

	// ChangedFilesBetweenSHAs returns the files changed in the range baseSHA..headSHA.
	// Renames are reported with status "renamed" and the new path. Untracked
	// working-tree files are NOT included — this is a pure git-history range.
	ChangedFilesBetweenSHAs(baseSHA, headSHA, dir string) ([]FileChange, error)

	// FileDiffBetweenSHAs returns parsed diff hunks for path in the range
	// baseSHA..headSHA. Returns nil hunks when there is no diff for that path.
	// When ignoreWhitespace is true, whitespace-only changes collapse to context.
	FileDiffBetweenSHAs(path, oldPath, baseSHA, headSHA, dir string, ignoreWhitespace bool) ([]DiffHunk, error)

	// ReadFileAtSHA returns the bytes of path at the given SHA. Returns
	// (nil, nil) when the file does not exist at that SHA. Errors are
	// reserved for "command failed" (e.g. invalid ref).
	ReadFileAtSHA(sha, path, dir string) ([]byte, error)

	// HasObject reports whether the given SHA is reachable as a commit
	// object in the local store. Used by callers before resolving a range
	// so they can prompt-fetch missing objects rather than fail mid-render.
	HasObject(sha, dir string) bool

	// FileStatusInRepo returns the status of a single file relative to baseRef.
	FileStatusInRepo(path, baseRef, dir string) string

	// HasStagingArea returns true if the VCS has a staging area (e.g. git index).
	HasStagingArea() bool

	// SkipDirNames returns directory names that should be skipped during walks
	// (e.g. ".git").
	SkipDirNames() []string
}

// InitialChangeSnapshotter is an optional fast path for VCS backends that can
// derive the initial working-tree file list and scope availability from the
// same repository snapshot. The bool is false when the backend cannot take a
// snapshot and the caller should use the regular VCS methods.
type InitialChangeSnapshotter interface {
	InitialChangesAndScopes(baseRef, dir string) (changes []FileChange, scopes []string, ok bool, err error)
}

// DetectVCS returns the git backend when the current directory is inside a
// git repository. Git is the only backend: vcsOverride may be "" or "git";
// any other value (including the removed "sl"/"sapling" and "jj"/"jujutsu")
// returns nil, as does a directory outside a git repository.
func DetectVCS(vcsOverride string) VCS {
	if (vcsOverride == "" || vcsOverride == "git") && IsGitRepo() {
		return &GitVCS{}
	}
	return nil
}
