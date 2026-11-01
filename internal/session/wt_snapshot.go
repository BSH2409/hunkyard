package session

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// wtSnapshot is one reading of the working tree shared by every request that
// needs it: the changes against HEAD (untracked included) and the scopes that
// have files. Its key moves when HEAD, the index or the base moves; edits to
// unstaged or untracked files do not move it, so the watcher marks it stale
// and reviewer-initiated requests only reuse a very recent one.
type wtSnapshot struct {
	key          wtSnapshotKey
	takenAt      time.Time
	changes      []vcs.FileChange
	changesKnown bool
	scopes       []string
}

type wtSnapshotKey struct {
	root, baseRef, head, index string
}

func (k wtSnapshotKey) String() string {
	return k.root + "\x00" + k.baseRef + "\x00" + k.head + "\x00" + k.index
}

var (
	// freshSnapshotWindow lets the requests of one reviewer action (a click
	// that loads the session, then its files) share one reading.
	freshSnapshotWindow = 2 * time.Second
	// snapshotMaxAge bounds reuse when the watcher is not running.
	snapshotMaxAge = 30 * time.Second
	// keyProbeReuse lets the several lookups of one request share a probe.
	keyProbeReuse = 250 * time.Millisecond
)

// snapshotKey probes HEAD and the index's stat: one git call, no tree walk.
func snapshotKey(vc vcs.VCS, root, baseRef string) wtSnapshotKey {
	key := wtSnapshotKey{root: root, baseRef: baseRef}
	if vc == nil || vc.Name() != "git" || root == "" {
		return key
	}
	out, err := vcs.RunGitInDir(root, "rev-parse", "--absolute-git-dir", "HEAD")
	lines := strings.Fields(out)
	if len(lines) == 0 {
		return key
	}
	if err == nil && len(lines) > 1 {
		key.head = lines[1]
	}
	if info, statErr := os.Stat(filepath.Join(lines[0], "index")); statErr == nil {
		key.index = strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
	}
	return key
}

// probedKey reuses a key probed moments ago for the same root and base.
func (s *Session) probedKey(vc vcs.VCS, root, baseRef string) wtSnapshotKey {
	s.snapMu.Lock()
	last, at := s.lastProbe, s.lastProbeAt
	s.snapMu.Unlock()
	if last.root == root && last.baseRef == baseRef && time.Since(at) < keyProbeReuse {
		return last
	}
	key := snapshotKey(vc, root, baseRef)
	s.snapMu.Lock()
	s.lastProbe, s.lastProbeAt = key, time.Now()
	s.snapMu.Unlock()
	return key
}

// snapshotFor returns a reading of the working tree for root and baseRef.
// fresh asks for one taken within freshSnapshotWindow (reviewer-initiated
// requests); otherwise any unexpired, unstale reading with the same key is
// reused. needChanges forces the change list into the reading.
func (s *Session) snapshotFor(ctx context.Context, vc vcs.VCS, root, baseRef string, fresh, needChanges bool) (*wtSnapshot, error) {
	key := s.probedKey(vc, root, baseRef)
	s.snapMu.Lock()
	cur, stale := s.snap, s.snapStale
	s.snapMu.Unlock()
	if cur != nil && cur.key == key && !stale && (cur.changesKnown || !needChanges) {
		age := time.Since(cur.takenAt)
		if age < freshSnapshotWindow || !fresh && age < snapshotMaxAge {
			return cur, nil
		}
	}
	value, err := s.snapFlight.Do(ctx, key.String(), func() (any, error) {
		s.snapMu.Lock()
		gen := s.snapGen
		s.snapMu.Unlock()
		snap, err := readWorkingTree(vc, root, baseRef, key)
		if err != nil {
			return nil, err
		}
		s.snapMu.Lock()
		s.snap, s.snapStale = snap, s.snapGen != gen
		s.snapMu.Unlock()
		return snap, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*wtSnapshot), nil
}

func readWorkingTree(vc vcs.VCS, root, baseRef string, key wtSnapshotKey) (*wtSnapshot, error) {
	snap := &wtSnapshot{key: key, takenAt: time.Now(), scopes: []string{"all"}}
	if vc == nil {
		snap.changesKnown = true
		return snap, nil
	}
	changes, err := vc.ChangedFilesOnDefaultInDir(root)
	if err != nil {
		return nil, err
	}
	snap.changes, snap.changesKnown = changes, true
	if baseRef != "" {
		if files, err := vc.ChangedFilesScoped("branch", baseRef); err == nil && len(files) > 0 {
			snap.scopes = append(snap.scopes, "branch")
		}
	}
	if vc.HasStagingArea() {
		if files, err := vc.ChangedFilesScoped("staged", baseRef); err == nil && len(files) > 0 {
			snap.scopes = append(snap.scopes, "staged")
		}
		// Untracked files already prove unstaged changes without another walk.
		unstaged := false
		for _, fc := range changes {
			if fc.Status == "untracked" {
				unstaged = true
				break
			}
		}
		if !unstaged && len(changes) > 0 {
			files, err := vc.ChangedFilesScoped("unstaged", baseRef)
			unstaged = err == nil && len(files) > 0
		}
		if unstaged {
			snap.scopes = append(snap.scopes, "unstaged")
		}
	}
	return snap, nil
}

// seedSnapshotScopes records the scopes a new session already computed, so
// the first page load does not walk the tree again.
func (s *Session) seedSnapshotScopes(vc vcs.VCS, root, baseRef string, scopes []string) {
	snap := &wtSnapshot{key: snapshotKey(vc, root, baseRef), takenAt: time.Now(), scopes: append([]string(nil), scopes...)}
	s.snapMu.Lock()
	s.snap, s.snapStale = snap, false
	s.snapMu.Unlock()
}

// invalidateWorkingTreeSnapshot marks the current reading stale; the next
// request reads the tree again.
func (s *Session) invalidateWorkingTreeSnapshot() {
	s.snapMu.Lock()
	s.snapStale = true
	s.snapGen++
	s.lastProbeAt = time.Time{}
	s.snapMu.Unlock()
}

// snapshotScopes returns the scopes with files, or just "all" when the tree
// cannot be read.
func (s *Session) snapshotScopes(ctx context.Context, vc vcs.VCS, root, baseRef string, fresh bool) []string {
	snap, err := s.snapshotFor(ctx, vc, root, baseRef, fresh, false)
	if err != nil {
		return []string{"all"}
	}
	return append([]string(nil), snap.scopes...)
}

// snapshotChanges returns a copy of the working-tree changes against HEAD.
func (s *Session) snapshotChanges(ctx context.Context, vc vcs.VCS, root, baseRef string, fresh bool) ([]vcs.FileChange, error) {
	snap, err := s.snapshotFor(ctx, vc, root, baseRef, fresh, true)
	if err != nil {
		return nil, err
	}
	return append([]vcs.FileChange(nil), snap.changes...), nil
}
