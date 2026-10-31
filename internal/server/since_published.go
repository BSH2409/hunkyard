package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/session"
)

// serveSinceDiff answers /api/file/diff?since=published: the file's changes
// since the reviewer last published, instead of since the base ref. Before
// the first publication it returns the normal diff with since_unavailable.
func (s *Server) serveSinceDiff(w http.ResponseWriter, r *http.Request, path string) {
	sess := s.session.Load()
	normal, ok := sess.GetFileDiffSnapshotScoped(path, "", "", r.URL.Query().Get("w") == "1")
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	snapshot, ok := s.latestPublishedSnapshot()
	current, found := sess.GetFileSnapshot(path)
	if !ok || !found {
		normal["since_unavailable"] = true
		writeJSON(w, normal)
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
