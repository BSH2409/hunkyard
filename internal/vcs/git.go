package vcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/BSH2409/hunkyard/internal/diff"
)

// FileChange represents a single file change detected by git.
type FileChange struct {
	Path    string // relative to repo root (new path for renames)
	OldPath string // previous path when Status is "renamed"
	Status  string // "added", "modified", "deleted", "renamed", "untracked"
}

// DiffHunk represents a single hunk in a unified diff.
type DiffHunk = diff.DiffHunk

// DiffLine represents a single line within a diff hunk.
type DiffLine = diff.DiffLine

// stripExternalDiffEnv returns a copy of the current process environment with
// any external-diff variables removed. Used together with `-c diff.external=`
// to guarantee diff output is the standard unified format regardless of how
// the user has configured external diff tools (e.g. difftastic).
// surfaced cases where `--no-ext-diff` alone wasn't enough.
func stripExternalDiffEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_EXTERNAL_DIFF=") || strings.HasPrefix(e, "GIT_DIFF_OPTS=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// runGit runs `git args...` in dir with hardened env (GIT_TERMINAL_PROMPT=0
// so a misconfigured credential helper can't hang the daemon waiting for tty
// input) and an optional context for cancellation. Returns stdout bytes; on
// error, the returned error includes captured stderr.
//
// This is a partial migration target: most call sites in this file still use
// exec.Command directly. New code, and any site that benefits from
// cancellation, should use runGit. Convert opportunistically.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Append rather than replace os.Environ so existing GIT_* config (auth,
	// SSH agent, etc.) is preserved. GIT_TERMINAL_PROMPT=0 prevents git from
	// blocking on a credential prompt when run from a daemon with no tty.
	cmd.Env = append(stripExternalDiffEnv(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrTrim := strings.TrimSpace(stderr.String())
		if stderrTrim != "" {
			return stdout.Bytes(), fmt.Errorf("git %s: %w: %s", args[0], err, stderrTrim)
		}
		return stdout.Bytes(), fmt.Errorf("git %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

// IsGitRepo returns true if the current directory is inside a git repository.
func IsGitRepo() bool {
	out, err := runGit(context.Background(), "", "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// RepoRoot returns the absolute path to the git repository root.
func RepoRoot() (string, error) {
	out, err := runGit(context.Background(), "", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

var (
	defaultBranchOnce     sync.Once
	defaultBranchResult   string
	defaultBranchOverride string
	defaultBranchMu       sync.RWMutex // protects defaultBranchOverride
)

// DefaultBranch returns the name of the default branch (main or master).
// The result is cached after the first call since it doesn't change during a session.
// If defaultBranchOverride is set, it is returned immediately without caching.
func DefaultBranch() string {
	defaultBranchMu.RLock()
	override := defaultBranchOverride
	defaultBranchMu.RUnlock()
	if override != "" {
		return override
	}
	defaultBranchOnce.Do(func() {
		defaultBranchResult = detectDefaultBranch()
	})
	return defaultBranchResult
}

// setDefaultBranchOverride safely updates the default branch override.
// SetDefaultBranchOverride sets the default branch for subsequent git operations.
func SetDefaultBranchOverride(branch string) {
	defaultBranchMu.Lock()
	defaultBranchOverride = branch
	defaultBranchMu.Unlock()
}

// ResetDefaultBranchOnceForTest clears cached default-branch detection (tests only).
func ResetDefaultBranchOnceForTest() {
	defaultBranchOnce = sync.Once{}
	defaultBranchResult = ""
}

// getDefaultBranchOverride safely reads the default branch override.
func getDefaultBranchOverride() string {
	defaultBranchMu.RLock()
	defer defaultBranchMu.RUnlock()
	return defaultBranchOverride
}

// defaultBaseRef returns the best ref to use as the diff base for the
// auto-detected default branch. Prefers `origin/<defaultBranch>` when that
// remote-tracking ref exists locally, otherwise falls back to the bare local
// branch name. The remote ref is preferred because the local default branch
// is often stale relative to upstream — leading to misleading diffs that
// include commits already merged on the remote.
//
// If the user has set a base-branch override (via config or CLI), this
// function honors it as-is, since the user has explicitly chosen a ref.
func DefaultBaseRef() string {
	branch := DefaultBranch()
	if getDefaultBranchOverride() != "" {
		return branch
	}
	remote := "origin/" + branch
	if exec.Command("git", "rev-parse", "--verify", "refs/remotes/"+remote).Run() == nil {
		return remote
	}
	return branch
}

func detectDefaultBranch() string {
	// Try remote HEAD first
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		// refs/remotes/origin/main -> main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main exists
	if err := exec.Command("git", "rev-parse", "--verify", "main").Run(); err == nil {
		return "main"
	}
	// Fallback: check if master exists
	if err := exec.Command("git", "rev-parse", "--verify", "master").Run(); err == nil {
		return "master"
	}
	return "main"
}

// RemoteBranches returns the names of all remote branches (without the "origin/" prefix).
// The result excludes HEAD. If dir is non-empty, git runs in that directory.
func RemoteBranches(dir string) ([]string, error) {
	// lstrip, not short: short does an ambiguity check per ref and turns
	// origin/HEAD into "origin".
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:lstrip=3)", "refs/remotes/origin/")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("for-each-ref failed: %w", err)
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := line
		if name == "" || name == "HEAD" {
			continue
		}
		branches = append(branches, name)
	}
	return branches, nil
}

// CurrentBranch returns the name of the current branch.
func CurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isOnDefaultBranch returns true if HEAD is on the default branch.
func isOnDefaultBranch() bool {
	return CurrentBranch() == DefaultBranch()
}

// MergeBase returns the merge base commit between HEAD and the given base ref.
// Falls back to origin/<base> when the local ref is missing — common in
// worktrees off a bare repo where only the remote-tracking ref exists.
func MergeBase(base string) (string, error) {
	out, err := exec.Command("git", "merge-base", "HEAD", base).Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	if !strings.HasPrefix(base, "origin/") {
		fallback, fbErr := exec.Command("git", "merge-base", "HEAD", "origin/"+base).Output()
		if fbErr == nil {
			return strings.TrimSpace(string(fallback)), nil
		}
	}
	return "", fmt.Errorf("merge-base failed: %w", err)
}

// MergeBaseOf returns the merge base of two arbitrary commits, run in dir.
// Neither side is pinned to HEAD (unlike MergeBase), so it can compute a PR's
// base as merge-base(base, head).
func MergeBaseOf(a, b, dir string) (string, error) {
	out, err := runGit(context.Background(), dir, "merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("git merge-base: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// fileContentAtRef returns the content of a file at the given git ref.
// Returns empty string on any error (file doesn't exist at ref, not a git repo, etc.).
func FileContentAtRef(path, ref, dir string) string {
	if ref == "" {
		return ""
	}
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// ChangedFiles returns the list of files changed in the current working state.
// On the default branch: staged + unstaged + untracked files.
// On a feature branch: all changes since the merge base with the default branch + untracked.
func ChangedFiles() ([]FileChange, error) {
	if isOnDefaultBranch() {
		return changedFilesOnDefault()
	}
	return changedFilesOnFeature()
}

// ChangedFilesScoped returns changed files for a specific scope.
// Supported scopes: "branch", "staged", "unstaged". Any other value falls back to ChangedFiles.
func ChangedFilesScoped(scope, baseRef string) ([]FileChange, error) {
	switch scope {
	case "branch":
		return changedFilesBranch(baseRef)
	case "staged":
		return changedFilesStaged()
	case "unstaged":
		return changedFilesUnstaged()
	default:
		return ChangedFiles()
	}
}

// changedFilesStaged returns only staged (cached) changes.
func changedFilesStaged() ([]FileChange, error) {
	out, err := runGit(context.Background(), "", "diff", "--cached", "--name-status", "-z")
	if err != nil {
		return nil, fmt.Errorf("git diff --cached failed: %w", err)
	}
	return parseNameStatusZ(out), nil
}

// changedFilesUnstaged returns unstaged modifications plus untracked files.
func changedFilesUnstaged() ([]FileChange, error) {
	untracked, err := workingTreeListing("", nil)
	if err != nil {
		return nil, err
	}
	out, err := runGit(context.Background(), "", "diff", "--name-status", "-z")
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	changes := parseNameStatusZ(out)
	changes = append(changes, untracked...)

	return dedup(changes), nil
}

// changedFilesBranch returns files changed between baseRef and HEAD.
// Returns nil if baseRef is empty.
func changedFilesBranch(baseRef string) ([]FileChange, error) {
	if baseRef == "" {
		return nil, nil
	}
	out, err := runGit(context.Background(), "", "diff", baseRef+"..HEAD", "--name-status", "-z")
	if err != nil {
		return nil, fmt.Errorf("git diff %s..HEAD failed: %w", baseRef, err)
	}
	return parseNameStatusZ(out), nil
}

// FileDiffScoped returns parsed diff hunks for a file using a scope-appropriate git diff command.
// Supported scopes: "branch", "staged", "unstaged". Any other value delegates to fileDiffUnified.
// The dir parameter sets the working directory for git commands (use repo root for correct path resolution).
// When ignoreWhitespace is true, whitespace-only changes collapse to context ("-w").
func FileDiffScoped(path, scope, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error) {
	base := diffBaseArgs(ignoreWhitespace)
	var cmd *exec.Cmd
	switch scope {
	case "branch":
		if baseRef == "" {
			return nil, nil
		}
		cmd = exec.Command("git", append(base, baseRef+"..HEAD", "--", path)...)
	case "staged":
		cmd = exec.Command("git", append(base, "--cached", "--", path)...)
	case "unstaged":
		if err := CheckWorkingTreeFilters(dir, path); err != nil {
			return nil, err
		}
		cmd = exec.Command("git", append(base, "--", path)...)
	default:
		return fileDiffUnified(path, baseRef, dir, ignoreWhitespace)
	}
	cmd.Env = stripExternalDiffEnv()
	if dir != "" {
		cmd.Dir = dir
	}

	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 means diff found changes (normal), check for actual errors
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// git diff exits 1 when there are differences
		} else {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}
	return ParseUnifiedDiff(string(out)), nil
}

// CommitInfo represents a single commit in a log.
type CommitInfo struct {
	SHA      string `json:"sha"`
	ShortSHA string `json:"short_sha"`
	Message  string `json:"message"`
	Author   string `json:"author"`
	Date     string `json:"date"`
	Virtual  bool   `json:"virtual,omitempty"`
}

// CommitLog returns the commits between baseRef and headRef, newest first.
// Returns nil if baseRef is empty. When headRef is empty, git's HEAD ref is used
// (working-tree behavior). Pass an explicit SHA to scope the upper bound — e.g.
// in range mode where HEAD may point past the focus.
// The dir parameter sets the working directory for the git command.
func CommitLog(baseRef, headRef, dir string) ([]CommitInfo, error) {
	if baseRef == "" {
		return nil, nil
	}
	if headRef == "" {
		headRef = "HEAD"
	}
	cmd := exec.Command("git", "log", "--format=%H%n%h%n%s%n%an%n%aI", baseRef+".."+headRef)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return nil, nil
	}
	lines := strings.Split(output, "\n")
	if len(lines)%5 != 0 {
		return nil, fmt.Errorf("unexpected git log output: %d lines (not a multiple of 5)", len(lines))
	}
	var commits []CommitInfo
	for i := 0; i < len(lines); i += 5 {
		commits = append(commits, CommitInfo{
			SHA:      lines[i],
			ShortSHA: lines[i+1],
			Message:  lines[i+2],
			Author:   lines[i+3],
			Date:     lines[i+4],
		})
	}
	return commits, nil
}

// commitishRe matches a safe git ref token: a hex string 4–40 chars long.
// Restricting to hex rejects option-injection (a token starting with "-")
// since base+".."+head is passed to git as a single argument.
var commitishRe = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// isCommitish reports whether s is a safe ref token (hex SHA, 4–40 chars).
func isCommitish(s string) bool {
	return commitishRe.MatchString(s)
}

// ValidCommitParam reports whether a commit selection from a request is a hex
// SHA or a "<sha>..<sha>" range: never a ref name, and never anything git
// could parse as an option.
func ValidCommitParam(commit string) bool {
	if _, _, ok := SplitCommitRange(commit); ok {
		return true
	}
	return isCommitish(commit)
}

// splitCommitRange parses a commit query param of the form "<baseSHA>..<headSHA>".
// Returns ok=false for a single-SHA (no "..") param, which keeps single-commit
// behavior. Both parts must be valid commit-ish tokens; otherwise ok=false.
func SplitCommitRange(commit string) (base, head string, ok bool) {
	i := strings.Index(commit, "..")
	if i < 0 {
		return "", "", false
	}
	base, head = commit[:i], commit[i+2:]
	if !isCommitish(base) || !isCommitish(head) {
		return "", "", false
	}
	return base, head, true
}

// commitFileListArgs lists a commit's files the way FileDiffForCommit diffs
// them: a root commit against the empty tree, a merge against its first
// parent.
func commitFileListArgs(sha string) []string {
	return []string{"--no-commit-id", "-r", "--root", "--diff-merges=first-parent", "--name-status", "-z", sha}
}

// ChangedFilesForCommit returns the files changed in a single commit.
// The dir parameter sets the working directory for the git command.
func ChangedFilesForCommit(sha, dir string) ([]FileChange, error) {
	cmd := exec.Command("git", append([]string{"diff-tree"}, commitFileListArgs(sha)...)...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree failed: %w", err)
	}
	return parseNameStatusZ(out), nil
}

// FileDiffForCommit returns parsed diff hunks for a file in a single commit.
// The dir parameter sets the working directory for the git command.
// For the initial (root) commit, sha^ is undefined so we diff against the empty tree.
// When ignoreWhitespace is true, whitespace-only changes collapse to context ("-w").
func FileDiffForCommit(path, sha, dir string, ignoreWhitespace bool) ([]DiffHunk, error) {
	cmd := exec.Command("git", append(diffBaseArgs(ignoreWhitespace), sha+"^.."+sha, "--", path)...)
	cmd.Env = stripExternalDiffEnv()
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
			// git diff exits 1 when there are differences — not an error
		case errors.As(err, &exitErr) && exitErr.ExitCode() == 128:
			// sha^ failed (root commit) — diff against the empty tree
			emptyTree := "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
			cmd2 := exec.Command("git", append(diffBaseArgs(ignoreWhitespace), emptyTree+".."+sha, "--", path)...)
			cmd2.Env = stripExternalDiffEnv()
			if dir != "" {
				cmd2.Dir = dir
			}
			out, err = cmd2.Output()
			if err != nil {
				var exitErr2 *exec.ExitError
				if errors.As(err, &exitErr2) && exitErr2.ExitCode() == 1 {
					// differences found
				} else {
					return nil, fmt.Errorf("git diff (root commit) failed: %w", err)
				}
			}
		default:
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}
	return ParseUnifiedDiff(string(out)), nil
}

func changedFilesOnDefault() ([]FileChange, error) {
	return changedFilesOnDefaultInDir("")
}

func changedFilesOnDefaultInDir(dir string) ([]FileChange, error) {
	untracked, err := workingTreeListing(dir, nil)
	if err != nil {
		return nil, err
	}
	// Staged + unstaged changes vs HEAD
	cmd := exec.Command("git", "diff", "HEAD", "--name-status", "-z")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		// If there's no HEAD (empty repo), try diff --cached + working tree
		cmd = exec.Command("git", "diff", "--name-status", "-z")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}

	changes := append(parseNameStatusZ(out), untracked...)
	return dedup(changes), nil
}

func changedFilesOnFeature() ([]FileChange, error) {
	mergeBase, err := MergeBase(DefaultBaseRef())
	if err != nil {
		// Fallback to HEAD diff if merge-base fails
		return changedFilesOnDefault()
	}

	return changedFilesFromBase(mergeBase)
}

// changedFilesFromBase returns files changed between a base ref and the working tree, plus untracked files.
func changedFilesFromBase(baseRef string) ([]FileChange, error) {
	return changedFilesFromBaseInDir(baseRef, "")
}

// changedFilesFromBaseInDir is like changedFilesFromBase but runs git from the specified directory.
func changedFilesFromBaseInDir(baseRef, dir string) ([]FileChange, error) {
	untracked, err := workingTreeListing(dir, nil)
	if err != nil {
		return nil, err
	}
	// All changes from base ref to working tree
	cmd := exec.Command("git", "diff", baseRef, "--name-status", "-z")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	changes := append(parseNameStatusZ(out), untracked...)
	return dedup(changes), nil
}

type gitStatusSnapshot struct {
	staged             []FileChange
	unstaged           []FileChange
	untracked          []FileChange
	needsUntrackedScan bool
}

// initialGitChangesAndScopes uses one status snapshot for staged, unstaged,
// and untracked state. A tree-only diff supplies committed branch changes.
// When status cannot produce a snapshot, ok=false tells the caller to fall
// back to collecting each change scope separately.
func initialGitChangesAndScopes(baseRef, dir string) ([]FileChange, []string, bool, error) {
	if err := CheckWorkingTreeFilters(dir); err != nil {
		return nil, nil, true, err
	}
	snapshot, err := gitStatusSnapshotInDir(dir)
	if err != nil {
		return nil, nil, false, nil //nolint:nilerr // unavailable snapshot uses the regular discovery path
	}
	if snapshot.needsUntrackedScan {
		snapshot.untracked, err = untrackedFilesFullScanInDir(dir)
		if err != nil {
			return nil, nil, true, err
		}
	}

	var branch []FileChange
	if baseRef != "" {
		out, diffErr := runGit(context.Background(), dir, "diff", baseRef+"..HEAD", "--name-status", "-z")
		if diffErr != nil {
			return nil, nil, true, fmt.Errorf("git diff %s..HEAD failed: %w", baseRef, diffErr)
		}
		branch = parseNameStatusZ(out)
	}

	comparisonBase := baseRef
	if comparisonBase == "" {
		comparisonBase = "HEAD"
	}
	tracked, err := mergeGitChangeLayers(comparisonBase, dir, branch, snapshot.staged, snapshot.unstaged)
	if err != nil {
		if baseRef == "" {
			return nil, nil, false, nil
		}
		return nil, nil, true, err
	}
	tracked = append(tracked, snapshot.untracked...)

	scopes := []string{"all"}
	if baseRef != "" && len(branch) > 0 {
		scopes = append(scopes, "branch")
	}
	if len(snapshot.staged) > 0 {
		scopes = append(scopes, "staged")
	}
	if len(snapshot.unstaged) > 0 || len(snapshot.untracked) > 0 {
		scopes = append(scopes, "unstaged")
	}
	return dedup(tracked), scopes, true, nil
}

func gitStatusSnapshotInDir(dir string) (gitStatusSnapshot, error) {
	cmd := exec.Command("git", "--no-optional-locks", "status", "--porcelain=v2", "-z", "--untracked-files=normal")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return gitStatusSnapshot{}, err
	}
	return parseGitStatusSnapshot(out), nil
}

func parseGitStatusSnapshot(out []byte) gitStatusSnapshot {
	var snapshot gitStatusSnapshot
	records := bytes.Split(out, []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 2 {
			continue
		}
		switch record[0] {
		case '1':
			fields := bytes.SplitN(record, []byte{' '}, 9)
			if len(fields) == 9 {
				appendGitStatusXY(&snapshot, fields[1], string(fields[8]), "")
			}
		case '2':
			fields := bytes.SplitN(record, []byte{' '}, 10)
			if len(fields) == 10 && i+1 < len(records) {
				i++
				appendGitStatusXY(&snapshot, fields[1], string(fields[9]), string(records[i]))
			}
		case 'u':
			fields := bytes.SplitN(record, []byte{' '}, 11)
			if len(fields) == 11 {
				appendGitStatusXY(&snapshot, fields[1], string(fields[10]), "")
			}
		case '?':
			if record[1] != ' ' {
				continue
			}
			path := string(record[2:])
			if strings.HasSuffix(path, "/") {
				snapshot.needsUntrackedScan = true
				continue
			}
			snapshot.untracked = append(snapshot.untracked, FileChange{Path: path, Status: "untracked"})
		}
	}
	return snapshot
}

func appendGitStatusXY(snapshot *gitStatusSnapshot, xy []byte, path, oldPath string) {
	if len(xy) != 2 {
		return
	}
	if xy[0] != '.' {
		snapshot.staged = append(snapshot.staged, gitStatusChange(xy[0], path, oldPath))
	}
	if xy[1] != '.' {
		snapshot.unstaged = append(snapshot.unstaged, gitStatusChange(xy[1], path, oldPath))
	}
}

func gitStatusChange(code byte, path, oldPath string) FileChange {
	switch code {
	case 'A':
		return FileChange{Path: path, Status: "added"}
	case 'D':
		return FileChange{Path: path, Status: "deleted"}
	case 'R':
		return FileChange{Path: path, OldPath: oldPath, Status: "renamed"}
	default:
		return FileChange{Path: path, Status: "modified"}
	}
}

func mergeGitChangeLayers(baseRef, dir string, layers ...[]FileChange) ([]FileChange, error) {
	seenPaths := make(map[string]bool)
	var merged []FileChange
	for _, layer := range layers {
		for _, change := range layer {
			if seenPaths[change.Path] || (change.OldPath != "" && seenPaths[change.OldPath]) {
				out, err := runGit(context.Background(), dir, "diff", baseRef, "--name-status", "-z")
				if err != nil {
					return nil, fmt.Errorf("git diff failed: %w", err)
				}
				return parseNameStatusZ(out), nil
			}
			seenPaths[change.Path] = true
			if change.OldPath != "" {
				seenPaths[change.OldPath] = true
			}
			merged = append(merged, change)
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Path < merged[j].Path
	})
	return merged, nil
}

// HeadSHAInDir returns the full SHA of HEAD in dir.
func HeadSHAInDir(dir string) (string, error) {
	out, err := RunGitInDir(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// RunGitInDir runs `git <args...>` in dir and returns the stdout. Mirrors the
// existing inline exec.Command pattern used elsewhere in git.go.
func RunGitInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	return string(out), err
}

// ResolveDefaultBranchSHA returns the tip SHA of the repo's default branch,
// preferring the remote (origin) and falling back to local. Returns
// ("", err) on failure — callers treat that as "full-stack unavailable"
// rather than fatal.
func ResolveDefaultBranchSHA(vcs VCS, repoRoot, defaultBranch string) (string, error) {
	if vcs == nil || defaultBranch == "" {
		return "", fmt.Errorf("default branch unknown")
	}
	if out, err := RunGitInDir(repoRoot, "rev-parse", "--verify", "origin/"+defaultBranch); err == nil {
		return strings.TrimSpace(out), nil
	}
	if out, err := RunGitInDir(repoRoot, "rev-parse", "--verify", defaultBranch); err == nil {
		return strings.TrimSpace(out), nil
	}
	return "", fmt.Errorf("could not resolve %s tip", defaultBranch)
}

// ResolveCommitOID resolves ref (branch name, tag, abbreviated SHA, or full
// OID) to a full commit object id. Used to canonicalize Focus.BaseSHA/HeadSHA
// so comment focus_keys stay stable when stack navigation switches between
// symbolic refs and resolved OIDs.
func ResolveCommitOID(v VCS, ref, dir string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil VCS")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}
	if v.Name() != "git" {
		return "", fmt.Errorf("resolve not supported for vcs=%q", v.Name())
	}
	out, err := RunGitInDir(dir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", ref, err)
	}
	return strings.TrimSpace(out), nil
}

// walkAncestors enumerates HEAD-first the recent ancestor SHAs that are
// candidates for stack stops. Capped at maxDepth.
func WalkAncestors(vcs VCS, repoRoot string, maxDepth int) ([]string, error) {
	if vcs == nil {
		return nil, nil
	}
	out, err := RunGitInDir(repoRoot, "rev-list", "--first-parent", "-n", strconv.Itoa(maxDepth), "HEAD")
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(out), nil
}

// localBranchTips returns SHAs that have a useful local label, mapped to that
// label: the refs/heads/ entries.
func LocalBranchTips(vcs VCS, repoRoot string) (map[string]string, error) {
	if vcs == nil {
		return nil, nil
	}
	out, err := RunGitInDir(repoRoot, "for-each-ref", "--format=%(objectname) %(refname:lstrip=2)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	addLabelLines(result, out)
	return result, nil
}

// addLabelLines records each "<sha> <label>" line of output in result,
// replacing any earlier label for the same SHA.
func addLabelLines(result map[string]string, output string) {
	for _, line := range splitNonEmpty(output) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = parts[1]
	}
}

// BranchEntry is one row in the picker's "Remote branches" section.
type BranchEntry struct {
	Name    string `json:"name"`
	HeadSHA string `json:"head_sha"`
}

// RemoteBranchTips returns up to 20 remote branches sorted by recency,
// excluding the default branch and any HEAD pointers. Caller subtracts SHAs
// already covered by other picker sections.
func RemoteBranchTips(vcs VCS, repoRoot, defaultBranch string) ([]BranchEntry, error) {
	if vcs == nil {
		return nil, nil
	}
	out, err := RunGitInDir(repoRoot, "for-each-ref",
		"--sort=-committerdate", "--count=40",
		"--format=%(objectname) %(refname:lstrip=2)",
		"refs/remotes/")
	if err != nil {
		return nil, err
	}
	var entries []BranchEntry
	for _, line := range splitNonEmpty(out) {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[1]
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		if name == "origin/"+defaultBranch || name == defaultBranch {
			continue
		}
		entries = append(entries, BranchEntry{Name: name, HeadSHA: parts[0]})
		if len(entries) >= 20 {
			break
		}
	}
	return entries, nil
}

// ChangedFilesBetweenSHAs returns the files changed in the range baseSHA..headSHA.
// Renames are reported with status "renamed", OldPath set to the previous path,
// and Path set to the new path.
// Untracked working-tree files are NOT included — this is a pure git-history range.
func ChangedFilesBetweenSHAs(baseSHA, headSHA, dir string) ([]FileChange, error) {
	cmd := exec.Command("git", "diff", "--name-status", "-z", "-M", baseSHA+".."+headSHA)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s..%s --name-status: %w", baseSHA, headSHA, err)
	}
	return parseNameStatusZ(out), nil
}

// FileDiffBetweenSHAs returns parsed diff hunks for path in the range
// baseSHA..headSHA. Pass oldPath when status is "renamed" so git pairs the
// rename instead of treating the new path as a full-file add.
// Returns nil hunks when there is no diff.
// When ignoreWhitespace is true, whitespace-only changes collapse to context ("-w").
func FileDiffBetweenSHAs(path, oldPath, baseSHA, headSHA, dir string, ignoreWhitespace bool) ([]DiffHunk, error) {
	pathArgs := diffPathArgs(oldPath, path)
	cmd := exec.Command("git", append(renameDiffArgs(oldPath, ignoreWhitespace), append([]string{baseSHA + ".." + headSHA, "--"}, pathArgs...)...)...)
	cmd.Env = stripExternalDiffEnv()
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		// git diff exits 1 when there are differences — that's normal.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("git diff: %w", err)
		}
	}
	return ParseUnifiedDiff(string(out)), nil
}

// ReadFileAtSHA returns the bytes of path at the given SHA.
// Returns (nil, nil) when the file does not exist at that SHA (deleted/added cases).
// Errors are reserved for "git command failed" (e.g. SHA not present locally).
func ReadFileAtSHA(sha, path, dir string) ([]byte, error) {
	cmd := exec.Command("git", "show", sha+":"+path)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
			// Distinguish "path missing at sha" (not an error) from
			// "sha missing entirely" (error). git show prints "fatal:
			// path 'foo' exists on disk, but not in 'sha'" or similar
			// for missing path; "fatal: bad object sha" for missing sha.
			msg := strings.TrimSpace(stderr.String())
			lower := strings.ToLower(msg)
			if strings.Contains(lower, "path") || strings.Contains(lower, "does not exist") {
				return nil, nil
			}
			// A submodule (gitlink) entry hits a third message shape: "fatal:
			// bad object <sha>:<path>". Gitlinks aren't blobs — git can never
			// show their "content" this way, regardless of whether the
			// submodule is initialized locally — so this isn't the "sha
			// missing entirely" case the branch above guards against (that one
			// reads "fatal: invalid object name '<sha>'", with no path
			// suffix). Treat it like a missing path rather than failing the
			// whole focus rebuild.
			if strings.Contains(lower, "bad object") && strings.Contains(msg, sha+":"+path) {
				return nil, nil
			}
			return nil, fmt.Errorf("git show %s:%s: %s", sha, path, msg)
		}
		return nil, fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return out, nil
}

// HasObject reports whether sha is reachable as a commit object in the local store.
// Cheap (no walk).
func HasObject(sha, dir string) bool {
	cmd := exec.Command("git", "cat-file", "-e", sha+"^{commit}")
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run() == nil
}

// untrackedFilesInDir returns untracked files, running from the specified
// directory. Start with status's cache-aware normal mode: it reports standalone
// files exactly and collapses wholly-untracked directories with a trailing slash.
// Only those directories require the slower full enumeration.
func untrackedFilesInDir(dir string) ([]FileChange, error) {
	cmd := exec.Command("git", "--no-optional-locks", "status", "--porcelain=v2", "-z", "--untracked-files=normal")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err == nil {
		changes, needsFullScan := parseNormalUntrackedStatus(out)
		if !needsFullScan {
			return changes, nil
		}
	}
	return untrackedFilesFullScanInDir(dir)
}

func parseNormalUntrackedStatus(out []byte) ([]FileChange, bool) {
	var changes []FileChange
	for _, record := range bytes.Split(out, []byte{0}) {
		if !bytes.HasPrefix(record, []byte("? ")) {
			continue
		}
		path := string(record[2:])
		if strings.HasSuffix(path, "/") {
			return nil, true
		}
		changes = append(changes, FileChange{Path: path, Status: "untracked"})
	}
	return changes, false
}

func untrackedFilesFullScanInDir(dir string) ([]FileChange, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--others", "--exclude-standard")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ls-files failed: %w", err)
	}
	var changes []FileChange
	for _, path := range bytes.Split(out, []byte{0}) {
		if len(path) > 0 {
			changes = append(changes, FileChange{Path: string(path), Status: "untracked"})
		}
	}
	return changes, nil
}

// AllTrackedFiles returns all tracked files plus untracked non-ignored files.
// Paths are relative to the repo root. dir should be the repo root.
func AllTrackedFiles(dir string, paths ...string) ([]string, error) {
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	if len(paths) > 0 {
		args = append(append([]string{"--literal-pathspecs"}, args...), "--")
		args = append(args, paths...)
	}
	out, err := runGit(context.Background(), dir, args...)
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}
	seen := make(map[string]bool)
	var files []string
	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files, nil
}

// SkipDirs is the shared set of directory names to skip during recursive walks.
// Used by both WalkFiles (git.go) and walkDirectory (session.go) to stay in sync.
var SkipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".git":         true,
	"dist":         true,
	"build":        true,
	"_build":       true,
	"deps":         true,
}

// WalkFiles returns all files under root, skipping hidden directories,
// node_modules, and other common non-project directories.
// Paths are relative to root.
func WalkFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort walk: skip inaccessible entries
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || SkipDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil //nolint:nilerr // skip files with unresolvable relative paths
		}
		// Normalize to forward slashes — file paths flow into review JSON,
		// the picker UI, and ignore-pattern matching, all of which assume
		// POSIX-style separators across platforms.
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

func parseNameStatus(output string) []FileChange {
	var changes []FileChange
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		path := parts[1]
		// For renames (R100\told\tnew), keep both paths
		if strings.HasPrefix(status, "R") && len(parts) >= 3 {
			changes = append(changes, FileChange{
				Path:    parts[2],
				OldPath: parts[1],
				Status:  "renamed",
			})
			continue
		}
		// A copy (C097\tsource\tcopy) is a new file at the second path.
		if strings.HasPrefix(status, "C") && len(parts) >= 3 {
			changes = append(changes, FileChange{Path: parts[2], Status: "added"})
			continue
		}
		switch status {
		case "A":
			changes = append(changes, FileChange{Path: path, Status: "added"})
		case "M":
			changes = append(changes, FileChange{Path: path, Status: "modified"})
		case "D":
			changes = append(changes, FileChange{Path: path, Status: "deleted"})
		default:
			changes = append(changes, FileChange{Path: path, Status: "modified"})
		}
	}
	return changes
}

// fileStatusInRepo returns the git status of a single file relative to baseRef
// by running `git diff --name-status <baseRef> -- <path>` from the repo root.
// This mirrors the same diff approach that ChangedFiles uses (merge-base diff),
// so files committed on a branch are correctly reported as added/modified.
// Falls back to checking whether the file is tracked when baseRef is empty.
func fileStatusInRepo(path, repoRoot, baseRef string) string {
	if baseRef == "" {
		// No base ref — check if the file is tracked at all.
		cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", path)
		cmd.Dir = repoRoot
		if err := cmd.Run(); err != nil {
			return "untracked"
		}
		return "modified"
	}
	cmd := exec.Command("git", "diff", "--name-status", baseRef, "--", path)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "untracked"
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		// File exists but is unchanged relative to baseRef.
		// Check if it's tracked; if not, it's untracked.
		chk := exec.Command("git", "ls-files", "--error-unmatch", "--", path)
		chk.Dir = repoRoot
		if err := chk.Run(); err != nil {
			return "untracked"
		}
		return "modified"
	}
	changes := parseNameStatus(line)
	if len(changes) > 0 {
		return changes[0].Status
	}
	return "modified"
}

// dedup removes duplicate paths, keeping the first occurrence.
func dedup(changes []FileChange) []FileChange {
	seen := map[string]bool{}
	var result []FileChange
	for _, c := range changes {
		if !seen[c.Path] {
			seen[c.Path] = true
			result = append(result, c)
		}
	}
	return result
}

// diffBaseArgs returns the leading git diff arguments. When ignoreWhitespace
// is true, "-w" is inserted right after "--no-ext-diff" (GitHub's ?w=1 parity).
func diffBaseArgs(ignoreWhitespace bool) []string {
	args := []string{"--literal-pathspecs", "-c", "diff.external=", "diff", "--no-color", "--no-ext-diff", "--no-textconv"}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	return args
}

// renameDiffArgs is diffBaseArgs plus -M for a renamed file, so a user's
// diff.renames=false cannot turn the pair into a whole-file add and delete.
func renameDiffArgs(oldPath string, ignoreWhitespace bool) []string {
	args := diffBaseArgs(ignoreWhitespace)
	if oldPath != "" {
		args = append(args, "-M")
	}
	return args
}

// diffPathArgs returns git diff path arguments for a file, pairing old+new
// paths for renames so git reports rename-only diffs instead of full adds.
func diffPathArgs(oldPath, path string) []string {
	if oldPath != "" {
		return []string{oldPath, path}
	}
	return []string{path}
}

// fileDiffUnified returns the parsed diff hunks for a file against a base ref.
// If baseRef is empty, diffs against HEAD. The dir parameter sets the working directory.
// When ignoreWhitespace is true, whitespace-only changes collapse to context ("-w").
// For renames, use diffHunksForFile which passes old+new paths via fileDiffUnifiedCtx.
func fileDiffUnified(path, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error) {
	return FileDiffUnifiedCtx(context.Background(), path, "", baseRef, dir, ignoreWhitespace)
}

// fileDiffUnifiedCtx is like fileDiffUnified but accepts a context for timeout control.
// When ignoreWhitespace is true, whitespace-only changes collapse to context ("-w").
// FileDiffUnifiedCtx runs git diff with context cancellation support.
func FileDiffUnifiedCtx(ctx context.Context, path, oldPath, baseRef, dir string, ignoreWhitespace bool) ([]DiffHunk, error) {
	if err := CheckWorkingTreeFilters(dir, diffPathArgs(oldPath, path)...); err != nil {
		return nil, err
	}
	ref := baseRef
	if ref == "" {
		ref = "HEAD"
	}
	pathArgs := diffPathArgs(oldPath, path)
	args := append(renameDiffArgs(oldPath, ignoreWhitespace), append([]string{ref, "--"}, pathArgs...)...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = stripExternalDiffEnv()
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// git diff exits 1 when there are differences
		} else {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}
	return ParseUnifiedDiff(string(out)), nil
}

// FileDiffUnifiedNewFile returns parsed diff hunks showing the entire file as added.
// Used for untracked files that don't have a git diff.
func FileDiffUnifiedNewFile(content string) []DiffHunk {
	lines := strings.Split(content, "\n")
	// Remove trailing empty line from split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	hunk := DiffHunk{
		OldStart: 0,
		OldCount: 0,
		NewStart: 1,
		NewCount: len(lines),
		Header:   fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)),
	}
	for i, line := range lines {
		hunk.Lines = append(hunk.Lines, DiffLine{
			Type:    "add",
			Content: line,
			OldNum:  0,
			NewNum:  i + 1,
		})
	}
	return []DiffHunk{hunk}
}

// NumstatEntry holds per-file addition/deletion counts from git diff --numstat.
type NumstatEntry struct {
	Additions int
	Deletions int
	Binary    bool // numstat reports "-" for both counts
}

// DiffNumstatDir runs git diff --numstat against the given base ref and returns per-file stats.
// If dir is non-empty, git runs in that directory.
func DiffNumstatDir(baseRef, dir string) (map[string]NumstatEntry, error) {
	if err := CheckWorkingTreeFilters(dir); err != nil {
		return nil, err
	}
	return runDiffNumstat(dir, baseRef)
}

// DiffNumstatBetweenSHAs runs git diff --numstat baseSHA headSHA and returns
// per-file stats for that fixed range (not vs the working tree).
func DiffNumstatBetweenSHAs(baseSHA, headSHA, dir string) (map[string]NumstatEntry, error) {
	if baseSHA == "" || headSHA == "" {
		return nil, fmt.Errorf("diff numstat between SHAs requires both base and head")
	}
	return runDiffNumstat(dir, baseSHA, headSHA)
}

func runDiffNumstat(dir string, refs ...string) (map[string]NumstatEntry, error) {
	// -z keeps rename pairs and non-ASCII paths verbatim, so counts key by
	// the real path instead of "dir/{old => new}" or a quoted escape.
	args := append([]string{"-c", "diff.external=", "diff", "--no-ext-diff", "--numstat", "-z"}, refs...)
	cmd := exec.Command("git", args...)
	cmd.Env = stripExternalDiffEnv()
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// git diff exits 1 when there are differences — normal
		} else {
			return nil, fmt.Errorf("git diff --numstat failed: %w", err)
		}
	}
	return parseNumstatZ(out), nil
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// ParseUnifiedDiff parses a unified diff string into hunks.
// hunkFromHeader parses an "@@ -a,b +c,d @@" line into an empty hunk, or
// returns nil for any other line. A missing count means one line.
func hunkFromHeader(line string) *DiffHunk {
	m := hunkHeaderRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	oldStart, _ := strconv.Atoi(m[1])
	oldCount := 1
	if m[2] != "" {
		oldCount, _ = strconv.Atoi(m[2])
	}
	newStart, _ := strconv.Atoi(m[3])
	newCount := 1
	if m[4] != "" {
		newCount, _ = strconv.Atoi(m[4])
	}
	return &DiffHunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount, Header: line}
}

func ParseUnifiedDiff(diff string) []DiffHunk {
	var hunks []DiffHunk
	// TrimRight removes the trailing newline so strings.Split doesn't produce
	// a spurious empty element that could be confused with a blank context line.
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")

	var current *DiffHunk
	oldLine, newLine := 0, 0

	for _, line := range lines {
		if hunk := hunkFromHeader(line); hunk != nil {
			if current != nil {
				hunks = append(hunks, *current)
			}
			current = hunk
			oldLine, newLine = hunk.OldStart, hunk.NewStart
			continue
		}

		// A second section for the same or another path starts a new file:
		// its header lines belong to no hunk.
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				hunks = append(hunks, *current)
				current = nil
			}
			continue
		}
		if current == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			current.Lines = append(current.Lines, DiffLine{
				Type:    "add",
				Content: strings.TrimPrefix(line, "+"),
				NewNum:  newLine,
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			current.Lines = append(current.Lines, DiffLine{
				Type:    "del",
				Content: strings.TrimPrefix(line, "-"),
				OldNum:  oldLine,
			})
			oldLine++
		case strings.HasPrefix(line, " "):
			current.Lines = append(current.Lines, DiffLine{
				Type:    "context",
				Content: strings.TrimPrefix(line, " "),
				OldNum:  oldLine,
				NewNum:  newLine,
			})
			oldLine++
			newLine++
		case line == "" && oldLine < current.OldStart+current.OldCount:
			// Bare empty line within expected hunk bounds — treat as blank context line.
			// Git outputs these when diff.suppressBlankEmpty is set, stripping the
			// leading space from blank context lines. We check bounds to avoid treating
			// the trailing empty string from strings.Split as a spurious context line.
			current.Lines = append(current.Lines, DiffLine{
				Type:    "context",
				Content: "",
				OldNum:  oldLine,
				NewNum:  newLine,
			})
			oldLine++
			newLine++
		case line == `\ No newline at end of file`:
			if len(current.Lines) > 0 {
				current.Lines[len(current.Lines)-1].NoFinalNewline = true
			}
			continue
		}
	}

	if current != nil {
		hunks = append(hunks, *current)
	}
	return hunks
}

// WorkingTreeFingerprint returns a string representing the current working tree state.
// Compare consecutive calls to detect changes.
func WorkingTreeFingerprint() string {
	root, err := RepoRoot()
	if err != nil {
		return ""
	}
	out, err := runGit(context.Background(), root, "--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return ""
	}
	return fingerprintWithStats(root, out)
}

// fingerprintWithStats adds each listed file's size and modification time
// to `git status --porcelain -z` output. Status alone stays "M file" while an
// already-modified file keeps changing; the stats make those edits visible.
func fingerprintWithStats(root string, status []byte) string {
	var b strings.Builder
	b.Write(status)
	records := bytes.Split(status, []byte{0})
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		path := string(record[3:])
		if record[0] == 'R' || record[0] == 'C' {
			i++ // the rename's source path follows
		}
		if info, statErr := os.Stat(filepath.Join(root, path)); statErr == nil {
			fmt.Fprintf(&b, "\x00%s %d %d", path, info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

// splitNonEmpty splits output by newline and returns the non-empty lines,
// trimming a trailing carriage return from each.
func splitNonEmpty(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	var result []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
