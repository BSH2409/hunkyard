package vcs

import (
	"context"
	"strings"
)

// ForPaths returns a Git backend restricted to literal repo-relative paths.
// All dirty discovery and filter validation use the same selection.
func (g *GitVCS) ForPaths(paths []string) *GitVCS {
	selected := *g
	selected.paths = append([]string(nil), paths...)
	return &selected
}

func (g *GitVCS) runSelectedGit(dir string, dirty bool, args ...string) ([]byte, error) {
	if dir == "" {
		var err error
		dir, err = g.RepoRoot()
		if err != nil {
			return nil, err
		}
	}
	if dirty {
		listArgs := append([]string{"--literal-pathspecs", "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--"}, g.paths...)
		out, err := runGit(context.Background(), dir, listArgs...)
		if err != nil {
			return nil, err
		}
		if len(out) > 0 {
			files := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
			if err := CheckWorkingTreeFilters(dir, files...); err != nil {
				return nil, err
			}
		}
	}
	command := append([]string{"--literal-pathspecs"}, args...)
	command = append(command, "--")
	command = append(command, g.paths...)
	return runGit(context.Background(), dir, command...)
}

func (g *GitVCS) selectedChanges(dir, baseRef, scope string) ([]FileChange, error) {
	args := []string{"diff", "--no-ext-diff", "--name-status", "-z", "-M"}
	dirty, untracked := false, false
	switch scope {
	case "branch":
		if baseRef == "" {
			return nil, nil
		}
		args = append(args, baseRef, "HEAD")
	case "staged":
		args = append(args, "--cached")
	case "unstaged":
		dirty, untracked = true, true
	default:
		if baseRef == "" {
			baseRef = "HEAD"
		}
		args = append(args, baseRef)
		dirty, untracked = true, true
	}
	var untrackedFiles []FileChange
	if untracked {
		if dir == "" {
			var err error
			if dir, err = g.RepoRoot(); err != nil {
				return nil, err
			}
		}
		// One listing both checks filters and supplies the untracked files.
		var err error
		if untrackedFiles, err = workingTreeListing(dir, g.paths); err != nil {
			return nil, err
		}
		dirty = false
	}
	out, err := g.runSelectedGit(dir, dirty, args...)
	if err != nil {
		return nil, err
	}
	return dedup(append(parseNameStatusZ(out), untrackedFiles...)), nil
}

func parseNameStatusZ(out []byte) []FileChange {
	fields := strings.Split(string(out), "\x00")
	var changes []FileChange
	for i := 0; i+1 < len(fields); {
		status, path := fields[i], fields[i+1]
		if status == "" {
			break
		}
		i += 2
		oldPath := ""
		if status[0] == 'R' && i < len(fields) {
			oldPath, path = path, fields[i]
			i++
		}
		changes = append(changes, gitStatusChange(status[0], path, oldPath))
	}
	return changes
}
