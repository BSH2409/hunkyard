package session

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/impact"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

// ErrImpactUnavailable means the review cannot compute impact for the path:
// the file is not part of a git-mode review.
var ErrImpactUnavailable = errors.New("impact unavailable")

const impactTimeout = 3 * time.Second

type impactEntry struct {
	key    string
	result impact.Result
}

// FileImpact lists the files that mention the symbols this file's diff
// defines or removes. Results are cached per file version and round.
func (s *Session) FileImpact(ctx context.Context, path string) (impact.Result, error) {
	s.mu.RLock()
	f := s.fileByPathLocked(path)
	git := s.Mode == "git" && s.VCS != nil
	s.mu.RUnlock()
	if f == nil || !git {
		return impact.Result{}, ErrImpactUnavailable
	}
	if err := s.ensureFileLoaded(f); err != nil {
		return impact.Result{}, err
	}
	s.mu.RLock()
	key := strconv.Itoa(s.ReviewRound) + "\x00" + f.FileHash + "\x00" + s.Focus.BaseSHA + ".." + s.Focus.HeadSHA + "\x00" + strconv.Itoa(len(f.DiffHunks))
	if hit, ok := s.impactCache[path]; ok && hit.key == key {
		s.mu.RUnlock()
		return hit.result, nil
	}
	symbols := impact.Symbols(path, f.DiffHunks)
	changed := make(map[string]bool, len(s.Files))
	for _, other := range s.Files {
		changed[other.Path] = true
	}
	root, patterns, grep := s.RepoRoot, s.IgnorePatterns, s.impactGrep
	s.mu.RUnlock()
	if grep == nil {
		grep = vcs.GrepTracked
	}
	ctx, cancel := context.WithTimeout(ctx, impactTimeout)
	defer cancel()
	result, err := impact.Search(ctx, func(ctx context.Context, words []string, exclude string) ([]vcs.GrepHit, error) {
		return grep(ctx, root, words, exclude)
	}, path, symbols, changed, func(p string) bool {
		return len(config.FilterPathsIgnored([]string{p}, patterns)) == 0
	})
	if err != nil {
		return impact.Result{}, err
	}
	s.mu.Lock()
	if s.impactCache == nil {
		s.impactCache = map[string]impactEntry{}
	}
	s.impactCache[path] = impactEntry{key: key, result: result}
	s.mu.Unlock()
	return result, nil
}
