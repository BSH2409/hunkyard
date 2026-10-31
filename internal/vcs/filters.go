package vcs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// UnsupportedFilterError keeps a dirty comparison from silently using unnormalized bytes.
type UnsupportedFilterError struct{ Path string }

func (e *UnsupportedFilterError) Error() string {
	return fmt.Sprintf("local-only review cannot compare working-tree path %q with a Git filter; executable clean/process filters are disabled. Use a committed --range comparison", e.Path)
}

// CheckWorkingTreeFilters inspects attributes without invoking their executable filters.
func CheckWorkingTreeFilters(dir string, paths ...string) error {
	var input []byte
	if len(paths) == 0 {
		var err error
		input, err = runGit(context.Background(), dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
	} else {
		root, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		for _, path := range paths {
			if filepath.IsAbs(path) {
				relative, err := filepath.Rel(root, path)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					continue
				}
				path = relative
			}
			input = append(input, []byte(path)...)
			input = append(input, 0)
		}
	}
	return checkFilterAttributes(dir, input)
}

// checkFilterAttributes runs one check-attr over NUL-separated paths.
func checkFilterAttributes(dir string, input []byte) error {
	if len(input) == 0 {
		return nil
	}
	cmd := exec.Command("git", "check-attr", "-z", "--stdin", "filter")
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cannot validate Git filter attributes: %w", err)
	}
	fields := bytes.Split(out, []byte{0})
	for i := 0; i+2 < len(fields); i += 3 {
		if !bytes.Equal(fields[i+2], []byte("unspecified")) && !bytes.Equal(fields[i+2], []byte("unset")) {
			return &UnsupportedFilterError{Path: string(fields[i])}
		}
	}
	return nil
}

// workingTreeListing lists tracked and untracked paths once (ls-files -t),
// checks all of them for filter attributes, and returns the untracked ones.
// It replaces a separate filter listing plus a second untracked scan.
func workingTreeListing(dir string, paths []string) ([]FileChange, error) {
	args := []string{"ls-files", "-t", "-z", "--cached", "--others", "--exclude-standard"}
	if len(paths) > 0 {
		args = append(append([]string{"--literal-pathspecs"}, args...), "--")
		args = append(args, paths...)
	}
	out, err := runGit(context.Background(), dir, args...)
	if err != nil {
		return nil, fmt.Errorf("ls-files failed: %w", err)
	}
	var input []byte
	var untracked []FileChange
	seen := map[string]bool{}
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) < 3 || entry[1] != ' ' {
			continue
		}
		path := string(entry[2:])
		if entry[0] == '?' {
			untracked = append(untracked, FileChange{Path: path, Status: "untracked"})
		}
		if !seen[path] {
			seen[path] = true
			input = append(append(input, path...), 0)
		}
	}
	if err := checkFilterAttributes(dir, input); err != nil {
		return nil, err
	}
	return untracked, nil
}
