package session

import (
	"encoding/json"
	"os"
)

// writtenState is what the daemon last wrote to (or loaded from) review.json:
// every comment and reply ID, and each comment's resolved flag. Comparing
// disk against it tells an external change (`hunkyard comment`) from a
// change the reviewer made in memory: an ID on disk that was never written
// is an addition from outside, and a written ID missing from memory was
// deleted in the browser and must stay deleted.
type writtenState struct {
	ids      map[string]struct{}
	resolved map[string]bool
}

func newWrittenState(cj HunkyardJSON) *writtenState {
	w := &writtenState{ids: map[string]struct{}{}, resolved: map[string]bool{}}
	add := func(c Comment) {
		w.ids[c.ID] = struct{}{}
		w.resolved[c.ID] = c.Resolved
		for _, r := range c.Replies {
			w.ids[r.ID] = struct{}{}
		}
	}
	for _, f := range cj.Files {
		for _, c := range f.Comments {
			add(c)
		}
	}
	for _, c := range cj.ReviewComments {
		add(c)
	}
	return w
}

func (w *writtenState) has(id string) bool {
	_, ok := w.ids[id]
	return ok
}

// mergeExternalInto brings the external additions in disk into mem: new
// replies, a changed resolved flag the reviewer did not also change, and new
// comments. It reports whether mem changed.
func (w *writtenState) mergeExternalInto(mem *[]Comment, disk []Comment, deleted map[string]struct{}) bool {
	changed := false
	index := make(map[string]int, len(*mem))
	for i, c := range *mem {
		index[c.ID] = i
	}
	for _, dc := range disk {
		i, inMem := index[dc.ID]
		if !inMem {
			_, tombstoned := deleted[dc.ID]
			if !w.has(dc.ID) && !tombstoned {
				*mem = append(*mem, dc)
				changed = true
			}
			continue
		}
		mc := &(*mem)[i]
		known := make(map[string]struct{}, len(mc.Replies))
		for _, r := range mc.Replies {
			known[r.ID] = struct{}{}
		}
		for _, r := range dc.Replies {
			if _, ok := known[r.ID]; !ok && !w.has(r.ID) {
				mc.Replies = append(mc.Replies, r)
				changed = true
			}
		}
		if was, ok := w.resolved[dc.ID]; ok && dc.Resolved != was && mc.Resolved == was {
			mc.Resolved = dc.Resolved
			changed = true
		}
	}
	return changed
}

// absorbExternalReviewChanges merges changes another process wrote to
// review.json since the daemon's last write or load into memory, so the
// daemon's next write keeps them. Callers hold the review lock.
func (s *Session) absorbExternalReviewChanges(identity string) {
	reviewPath := ReviewPathsFor(identity).Review
	info, err := os.Stat(reviewPath)
	s.mu.RLock()
	written, lastMtime := s.lastWritten, s.lastHunkyardJSONMtime
	s.mu.RUnlock()
	if err != nil || written == nil || info.ModTime().Equal(lastMtime) {
		return
	}
	data, err := ReadFileShared(reviewPath)
	if err != nil {
		return
	}
	var disk HunkyardJSON
	if json.Unmarshal(data, &disk) != nil {
		return
	}
	s.mu.Lock()
	changed := false
	for _, f := range s.Files {
		if df, ok := disk.Files[f.Path]; ok && written.mergeExternalInto(&f.Comments, df.Comments, s.deletedCommentIDs[f.Path]) {
			changed = true
		}
	}
	if written.mergeExternalInto(&s.reviewComments, disk.ReviewComments, nil) {
		changed = true
	}
	s.mu.Unlock()
	if changed {
		s.notify(SSEEvent{Type: "comments-changed"})
	}
}
