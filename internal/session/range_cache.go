package session

import (
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// rangeCacheCap bounds the commit-range cache by entries: a change list, a
// count map or one file's hunks each take one.
const rangeCacheCap = 512

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// rangeKey names an immutable comparison: one commit (Head only) or a range
// between two commits. A session's path selection never changes, so it is
// not part of the key.
type rangeKey struct {
	Base, Head string
	IgnoreWS   bool
}

func (k rangeKey) String(kind, path string) string {
	return kind + "\x00" + k.Base + "\x00" + k.Head + "\x00" + strconv.FormatBool(k.IgnoreWS) + "\x00" + path
}

// immutableRange reports whether commit names fixed history: a full SHA or a
// full-SHA range. Refs (HEAD, branches), short SHAs and the working tree are
// never cached.
func immutableRange(commit string) (rangeKey, bool) {
	if base, head, ok := vcs.SplitCommitRange(commit); ok {
		return rangeKey{Base: base, Head: head}, fullSHA.MatchString(base) && fullSHA.MatchString(head)
	}
	return rangeKey{Head: commit}, fullSHA.MatchString(commit)
}

// rangeCached returns a cached value for an immutable commit or range, or
// computes and stores it. Mutable selections always compute.
func rangeCached[T any](s *Session, commit, kind, path string, ignoreWS bool, compute func() (T, error)) (T, error) {
	key, ok := immutableRange(commit)
	if !ok {
		return compute()
	}
	key.IgnoreWS = ignoreWS
	name := key.String(kind, path)
	s.snapMu.Lock()
	if s.rangeCache == nil {
		s.rangeCache = newBytesLRU(rangeCacheCap)
	}
	cache := s.rangeCache
	s.snapMu.Unlock()
	var value T
	if data, hit := cache.Get(name); hit && json.Unmarshal(data, &value) == nil {
		return value, nil
	}
	value, err := compute()
	if err != nil {
		return value, err
	}
	if data, encodeErr := json.Marshal(value); encodeErr == nil {
		cache.Put(name, data)
	}
	return value, nil
}
