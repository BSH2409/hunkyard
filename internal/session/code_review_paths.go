package session

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// NewSessionFromPaths defaults changed code in Git to a path-restricted diff review.
// Documents, unchanged files, and files outside Git remain file reviews.
func NewSessionFromPaths(v vcs.VCS, paths, ignorePatterns []string) (*Session, error) {
	git, ok := v.(*vcs.GitVCS)
	if !ok {
		return NewSessionFromFiles(paths, ignorePatterns)
	}
	root, err := git.RepoRoot()
	if err != nil {
		return nil, err
	}
	root = canonicalRepoRoot(root)
	selected := make([]string, 0, len(paths))
	for _, path := range paths {
		abs, err := canonicalAbsPath(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return NewSessionFromFiles(paths, ignorePatterns)
		}
		selected = append(selected, filepath.ToSlash(rel))
	}
	if !git.HasObject("HEAD", root) {
		return NewSessionFromFiles(paths, ignorePatterns)
	}
	sess, err := NewGitSession(git.ForPaths(selected), ignorePatterns)
	if errors.Is(err, ErrNoChangedFiles) {
		return NewSessionFromFiles(paths, ignorePatterns)
	}
	if err != nil {
		return nil, err
	}
	for _, file := range sess.Files {
		if file.FileType != "markdown" {
			return sess, nil
		}
	}
	return NewSessionFromFiles(paths, ignorePatterns)
}
