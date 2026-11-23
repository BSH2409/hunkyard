package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/session"
)

// Published snapshots contain whole file contents, which only match the unpinned All comparison.
func sincePublishedComparisonSupported(sess *Session, scope, commit string) bool {
	sess.RLock()
	defer sess.RUnlock()
	return (scope == "" || scope == "all") && commit == "" && sess.Focus.Kind != FocusRange
}

// serveSinceDiff answers /api/file/diff?since=published: the file's changes
// since the reviewer last published. Before the first publication it returns
// the requested comparison with since_unavailable.
func (s *Server) serveSinceDiff(w http.ResponseWriter, sess *Session, path string, fullDiff map[string]any) {
	snapshot, ok := s.latestPublishedSnapshot()
	current, found := sess.GetFileSnapshot(path)
	if !ok || !found {
		fullDiff["since_unavailable"] = true
		writeJSON(w, fullDiff)
		return
	}
	content, _ := current["content"].(string)
	writeJSON(w, map[string]any{"hunks": sinceSnapshotHunks(snapshot, path, content), "previous_content": publishedContent(snapshot, path), "since": "published"})
}

func (s *Server) latestPublishedSnapshot() (session.ReviewSnapshot, bool) {
	store, err := s.feedbackStore()
	if err != nil {
		return session.ReviewSnapshot{}, false
	}
	id, err := store.LatestSnapshotID()
	if err != nil || id == "" {
		return session.ReviewSnapshot{}, false
	}
	dir := s.reviewPath
	if dir == "" {
		dir = filepath.Dir(s.session.Load().HunkyardJSONPath())
	}
	snapshot, err := session.LoadReviewSnapshot(dir, id)
	return snapshot, err == nil
}

// sinceSnapshotHunks diffs content against the path's bytes in snapshot; a
// path the snapshot lacks diffs against empty, so the whole file is added.
func sinceSnapshotHunks(snapshot session.ReviewSnapshot, path, content string) []diff.DiffHunk {
	previous := publishedContent(snapshot, path)
	// SplitLines turns a final newline into an empty last line; drop it so a
	// new file does not show a phantom added line.
	hunks := diff.DiffEntriesToHunks(diff.ComputeLineDiff(strings.TrimSuffix(previous, "\n"), strings.TrimSuffix(content, "\n")))
	if hunks == nil {
		hunks = []diff.DiffHunk{}
	}
	return hunks
}

func publishedContent(snapshot session.ReviewSnapshot, path string) string {
	for _, file := range snapshot.Files {
		if file.Path == path {
			return string(file.Content)
		}
	}
	return ""
}
