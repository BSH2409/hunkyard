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
