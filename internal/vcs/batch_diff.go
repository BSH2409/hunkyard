package vcs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DiffSpec names one comparison for a batched diff. Head empty compares
// against the working tree; Cached compares the index (git diff --cached).
// WholeAdded shows working-tree added files whole, read from disk, as the
// review does for "all" and branch views; untracked files always are.
type DiffSpec struct {
	Base       string
	Head       string
	Cached     bool
	WholeAdded bool
}

// BatchDiffer computes diffs and counts for many files in one VCS call.
// Backends without it (sapling, jj) keep the per-file path.
type BatchDiffer interface {
	DiffAll(ctx context.Context, dir string, spec DiffSpec, changes []FileChange) (map[string][]DiffHunk, error)
	NumstatAll(ctx context.Context, dir string, spec DiffSpec, changes []FileChange) (map[string]NumstatEntry, error)
}

// errBatchSplit means the patch and its --raw list disagreed; callers fall
// back to per-file diffs.
var errBatchSplit = errors.New("batched diff did not split cleanly")

func (spec DiffSpec) args() []string {
	var args []string
	if spec.Cached {
		args = append(args, "--cached")
	}
	if spec.Base != "" {
		args = append(args, spec.Base)
	}
	if spec.Head != "" {
		args = append(args, spec.Head)
	}
	return args
}

// diskChange reports whether a change's diff comes from the file on disk:
// working-tree comparisons show added and untracked files whole.
func (spec DiffSpec) diskChange(fc FileChange) bool {
	return spec.Head == "" && !spec.Cached && (fc.Status == "untracked" || fc.Status == "added" && spec.WholeAdded)
}

// batchPathspecLimit is the largest batch diffed by its own paths. A review's
// eager files (at most lazyFileThreshold) always fit, so git never builds the
// patch of a whole large change set to keep 25 files of it; bigger batches
// (the lazy tail's counts) use the selection and stay one short command.
const batchPathspecLimit = 100

func (g *GitVCS) runBatch(ctx context.Context, dir string, spec DiffSpec, changes []FileChange, format ...string) ([]byte, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	var paths []string
	for _, fc := range changes {
		paths = append(paths, diffPathArgs(fc.OldPath, fc.Path)...)
	}
	if spec.Head == "" {
		if err := CheckWorkingTreeFilters(dir, paths...); err != nil {
			return nil, err
		}
	}
	args := append(diffBaseArgs(false), "-M")
	args = append(args, format...)
	args = append(append(args, spec.args()...), "--")
	if len(changes) <= batchPathspecLimit {
		args = append(args, paths...)
	} else {
		args = append(args, g.paths...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = stripExternalDiffEnv()
	cmd.Dir = dir
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	return out, nil
}

// DiffAll returns hunks for every change from one git diff.
func (g *GitVCS) DiffAll(ctx context.Context, dir string, spec DiffSpec, changes []FileChange) (map[string][]DiffHunk, error) {
	out, err := g.runBatch(ctx, dir, spec, changes, "--raw", "-z", "-p")
	if err != nil {
		return nil, err
	}
	sections, err := splitBatchedPatch(out)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]DiffHunk, len(changes))
	for _, fc := range changes {
		if spec.diskChange(fc) {
			data, readErr := os.ReadFile(filepath.Join(dir, fc.Path))
			if readErr != nil {
				return nil, readErr
			}
			result[fc.Path] = FileDiffUnifiedNewFile(string(data))
			continue
		}
		hunks, ok := sections[fc.Path]
		if !ok {
			// The batch saw this change differently (for example a rename
			// that diff.renames=false lists as a deletion and an addition).
			return nil, errBatchSplit
		}
		result[fc.Path] = hunks
	}
	return result, nil
}

// splitBatchedPatch pairs the NUL-separated --raw entries with the patch
// sections that follow them. Paths come from the raw list, never from the
// C-quoted "diff --git" headers; sections are split at lines that start with
// "diff --git ", which content lines (prefixed " ", "+", "-", "\") never do.
func splitBatchedPatch(out []byte) (map[string][]DiffHunk, error) {
	fields := bytes.Split(out, []byte{0})
	var paths []string
	i := 0
	for i < len(fields) && len(fields[i]) > 0 && fields[i][0] == ':' {
		meta := strings.Fields(string(fields[i]))
		if len(meta) == 0 || i+1 >= len(fields) {
			return nil, errBatchSplit
		}
		status := meta[len(meta)-1]
		path := string(fields[i+1])
		i += 2
		if status[0] == 'R' || status[0] == 'C' {
			if i >= len(fields) {
				return nil, errBatchSplit
			}
			path = string(fields[i])
			i++
		}
		paths = append(paths, path)
	}
	patch := string(bytes.Join(fields[i:], []byte{0}))
	patch = strings.TrimLeft(patch, "\x00")
	var sections []string
	for len(patch) > 0 {
		next := strings.Index(patch[1:], "\ndiff --git ")
		if next < 0 {
			sections = append(sections, patch)
			break
		}
		sections = append(sections, patch[:next+2])
		patch = patch[next+2:]
	}
	if len(sections) != len(paths) {
		return nil, errBatchSplit
	}
	result := make(map[string][]DiffHunk, len(paths))
	for k, path := range paths {
		result[path] = ParseUnifiedDiff(sections[k])
	}
	return result, nil
}

// NumstatAll returns added/deleted line counts for every change from one
// git diff --numstat; working-tree added and untracked files are counted on
// disk, as their diffs are shown.
func (g *GitVCS) NumstatAll(ctx context.Context, dir string, spec DiffSpec, changes []FileChange) (map[string]NumstatEntry, error) {
	out, err := g.runBatch(ctx, dir, spec, changes, "--numstat", "-z")
	if err != nil {
		return nil, err
	}
	stats := parseNumstatZ(out)
	for _, fc := range changes {
		if spec.diskChange(fc) {
			data, readErr := os.ReadFile(filepath.Join(dir, fc.Path))
			if readErr != nil {
				continue
			}
			adds := strings.Count(string(data), "\n")
			if len(data) > 0 && data[len(data)-1] != '\n' {
				adds++
			}
			stats[fc.Path] = NumstatEntry{Additions: adds}
		}
	}
	return stats, nil
}

// parseNumstatZ reads "adds\tdels\tpath\0" records; a rename is
// "adds\tdels\t\0old\0new\0" and is keyed by its new path.
func parseNumstatZ(out []byte) map[string]NumstatEntry {
	stats := make(map[string]NumstatEntry)
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(string(fields[i]), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(fields) {
			path = string(fields[i+2])
			i += 2
		}
		entry := NumstatEntry{Binary: parts[0] == "-" && parts[1] == "-"}
		entry.Additions, _ = strconv.Atoi(parts[0])
		entry.Deletions, _ = strconv.Atoi(parts[1])
		stats[path] = entry
	}
	return stats
}
