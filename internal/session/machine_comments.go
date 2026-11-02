package session

import (
	"strconv"
	"strings"
	"time"
)

// MachineComment is a finding a tool or a preference check wants on the
// review. Line 0 makes it a file-level comment.
type MachineComment struct {
	Path      string
	Line      int
	EndLine   int
	Body      string
	Author    string
	SourceRef string // per-comment source ref; empty uses the replacement's ref
}

// ReplaceResult counts what one replacement did.
type ReplaceResult struct {
	Added   int `json:"added"`
	Kept    int `json:"kept"`
	Removed int `json:"removed"`
	Dropped int `json:"dropped"`
}

// MarkRange is a line range a tool flags without a message.
type MarkRange struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
}

// Coverage counts a file's added lines and how many of them a tool marked.
type Coverage struct {
	Changed   int `json:"changed"`
	Uncovered int `json:"uncovered"`
}

// ToolMarks is one tool's marks for the round they were set in.
type ToolMarks struct {
	Round    int                    `json:"round"`
	Marks    map[string][]MarkRange `json:"marks"`
	Coverage map[string]Coverage    `json:"coverage"`
}

func machineKey(path string, line, endLine int, body string) string {
	return computeFileHash([]byte(strings.Join([]string{path, strconv.Itoa(line), strconv.Itoa(endLine), body}, "\x00")))
}

// commentableLines returns the new-side line numbers a comment may sit on:
// added and context lines of the diff, or every line when the file has no
// diff (files mode, or a lazily loaded file without hunks yet).
func commentableLines(f *FileEntry) func(int) bool {
	if len(f.DiffHunks) == 0 {
		count := strings.Count(f.Content, "\n")
		if f.Content != "" && !strings.HasSuffix(f.Content, "\n") {
			count++
		}
		return func(n int) bool { return n >= 1 && n <= count }
	}
	lines := map[int]bool{}
	for _, h := range f.DiffHunks {
		for _, l := range h.Lines {
			if l.Type != "del" && l.NewNum > 0 {
				lines[l.NewNum] = true
			}
		}
	}
	return func(n int) bool { return lines[n] }
}

// addedLines returns the new-side numbers of the file's added lines.
func addedLines(f *FileEntry) map[int]bool {
	lines := map[int]bool{}
	for _, h := range f.DiffHunks {
		for _, l := range h.Lines {
			if l.Type == "add" && l.NewNum > 0 {
				lines[l.NewNum] = true
			}
		}
	}
	return lines
}

// loadFilesFor makes sure every referenced file has its content and hunks.
// It must run without s.mu held.
func (s *Session) loadFilesFor(paths []string) {
	for _, p := range paths {
		s.mu.RLock()
		f := s.fileByPathLocked(p)
		s.mu.RUnlock()
		if f != nil {
			_ = s.ensureFileLoaded(f)
		}
	}
}

// ReplaceMachineComments makes the comments from one source (a tool or a
// rule check) match incoming. An empty sourceRef replaces every comment of
// the source, whatever its ref. A comment whose path, lines and body are
// unchanged is kept with its replies, resolution and verdict; the others are
// removed, and new ones are added. Findings off the diff or on files outside
// the review are dropped.
func (s *Session) ReplaceMachineComments(source, sourceRef string, incoming []MachineComment) ReplaceResult {
	paths := make([]string, 0, len(incoming))
	for _, m := range incoming {
		paths = append(paths, m.Path)
	}
	s.loadFilesFor(paths)

	s.mu.Lock()
	result := ReplaceResult{}
	wanted, order := s.acceptMachineCommentsLocked(sourceRef, incoming, &result)
	refs := map[string]bool{sourceRef: true}
	for _, m := range wanted {
		refs[m.SourceRef] = true
	}
	present := s.matchMachineCommentsLocked(source, sourceRef, refs, wanted, &result)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, key := range order {
		if present[key] {
			continue
		}
		m := wanted[key]
		f := s.fileByPathLocked(m.Path)
		c := Comment{
			ID: RandomCommentID(), ContentID: commentContentID(f), ViewID: s.viewIDLocked(),
			Body: m.Body, Author: m.Author, Source: source, SourceRef: m.SourceRef,
			CreatedAt: now, UpdatedAt: now, ReviewRound: s.ReviewRound,
		}
		if m.Line > 0 {
			c.StartLine, c.EndLine, c.Scope = m.Line, m.EndLine, "line"
			c.Anchor = extractAnchor(f.Content, m.Line, m.EndLine)
		} else {
			c.Scope = "file"
		}
		f.Comments = append(f.Comments, StampWithFocus(c, s.Focus))
		result.Added++
	}
	if result.Added+result.Removed > 0 {
		s.scheduleWrite()
	}
	s.mu.Unlock()
	if result.Added+result.Removed > 0 {
		s.notify(SSEEvent{Type: "comments-changed"})
	}
	return result
}

// matchMachineCommentsLocked keeps the existing comments of this source that
// match a wanted finding and removes the rest. A comment matches by path,
// lines and body; failing that, by rule or tool, path and the text of its
// lines, so a finding whose line moved keeps its thread, resolution and
// verdict and is moved to the new line. Caller holds s.mu.
func (s *Session) matchMachineCommentsLocked(source, sourceRef string, refs map[string]bool, wanted map[string]MachineComment, result *ReplaceResult) map[string]bool {
	owned := func(c Comment) bool { return c.Source == source && (sourceRef == "" || refs[c.SourceRef]) }
	present := map[string]bool{}
	matched := map[string]string{} // comment id → wanted key
	match := func(keyOf func(path string, c Comment) (string, bool)) {
		for _, f := range s.Files {
			for _, c := range f.Comments {
				if _, done := matched[c.ID]; done || !owned(c) {
					continue
				}
				if key, ok := keyOf(f.Path, c); ok && !present[key] {
					present[key], matched[c.ID] = true, key
				}
			}
		}
	}
	match(func(path string, c Comment) (string, bool) {
		key := machineKey(path, c.StartLine, c.EndLine, c.Body)
		_, ok := wanted[key]
		return key, ok
	})
	byAnchor := s.machineAnchorsLocked(wanted)
	match(func(path string, c Comment) (string, bool) {
		key, ok := byAnchor[c.SourceRef+"\x00"+path+"\x00"+c.Anchor]
		return key, ok && c.Anchor != ""
	})
	s.applyMachineMatchesLocked(owned, matched, wanted, result)
	return present
}

// applyMachineMatchesLocked removes this source's unmatched comments and
// moves matched ones to their finding's current line. Caller holds s.mu.
func (s *Session) applyMachineMatchesLocked(owned func(Comment) bool, matched map[string]string, wanted map[string]MachineComment, result *ReplaceResult) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range s.Files {
		kept := f.Comments[:0:0]
		for _, c := range f.Comments {
			key, ok := matched[c.ID]
			switch {
			case !owned(c):
			case !ok:
				result.Removed++
				s.trackDeletedComment(f.Path, c.ID)
				continue
			default:
				if m := wanted[key]; c.StartLine != m.Line || c.Body != m.Body {
					c.StartLine, c.EndLine, c.Body, c.UpdatedAt = m.Line, m.EndLine, m.Body, now
				}
				result.Kept++
			}
			kept = append(kept, c)
		}
		f.Comments = kept
	}
}

// machineAnchorsLocked indexes wanted line findings by source ref, path and
// the text of their lines. Caller holds s.mu.
func (s *Session) machineAnchorsLocked(wanted map[string]MachineComment) map[string]string {
	byAnchor := map[string]string{}
	for key, m := range wanted {
		if f := s.fileByPathLocked(m.Path); f != nil && m.Line > 0 {
			if anchor := extractAnchor(f.Content, m.Line, m.EndLine); anchor != "" {
				byAnchor[m.SourceRef+"\x00"+m.Path+"\x00"+anchor] = key
			}
		}
	}
	return byAnchor
}

// acceptMachineCommentsLocked validates incoming findings against the review
// (known file, line on the diff) and returns them keyed by identity, in input
// order. Rejected findings are counted as dropped. Caller holds s.mu.
func (s *Session) acceptMachineCommentsLocked(sourceRef string, incoming []MachineComment, result *ReplaceResult) (map[string]MachineComment, []string) {
	wanted := map[string]MachineComment{}
	order := []string{}
	for _, m := range incoming {
		f := s.fileByPathLocked(m.Path)
		if f == nil || m.Line < 0 {
			result.Dropped++
			continue
		}
		if m.Line > 0 {
			m.EndLine = max(m.EndLine, m.Line)
			if !commentableLines(f)(m.EndLine) {
				result.Dropped++
				continue
			}
		} else {
			m.EndLine = 0
		}
		if m.SourceRef == "" {
			m.SourceRef = sourceRef
		}
		key := machineKey(m.Path, m.Line, m.EndLine, m.Body)
		if _, dup := wanted[key]; !dup {
			order = append(order, key)
		}
		wanted[key] = m
	}
	return wanted, order
}

// SetToolMarks stores one tool's marks, keeping only the parts that cover
// added lines of files in the review, and returns per-file coverage.
func (s *Session) SetToolMarks(tool string, marks map[string][]MarkRange) map[string]Coverage {
	paths := make([]string, 0, len(marks))
	for p := range marks {
		paths = append(paths, p)
	}
	s.loadFilesFor(paths)

	s.mu.Lock()
	stored := ToolMarks{Round: s.ReviewRound, Marks: map[string][]MarkRange{}, Coverage: map[string]Coverage{}}
	for path, ranges := range marks {
		f := s.fileByPathLocked(path)
		if f == nil {
			continue
		}
		added := addedLines(f)
		if len(added) == 0 {
			continue
		}
		uncovered := 0
		var kept []MarkRange
		for _, r := range ranges {
			start := -1
			for n := r.Start; n <= r.End+1; n++ {
				if n <= r.End && added[n] {
					uncovered++
					if start < 0 {
						start = n
					}
					continue
				}
				if start >= 0 {
					kept = append(kept, MarkRange{Start: start, End: n - 1, Kind: r.Kind})
					start = -1
				}
			}
		}
		stored.Coverage[path] = Coverage{Changed: len(added), Uncovered: uncovered}
		if len(kept) > 0 {
			stored.Marks[path] = kept
		}
	}
	if s.toolMarks == nil {
		s.toolMarks = map[string]ToolMarks{}
	}
	s.toolMarks[tool] = stored
	s.scheduleWrite()
	s.mu.Unlock()
	s.notify(SSEEvent{Type: "annotations-changed"})
	return stored.Coverage
}

// ToolMarks returns a copy of every tool's marks.
func (s *Session) ToolMarks() map[string]ToolMarks {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneToolMarks(s.toolMarks)
}

func cloneToolMarks(in map[string]ToolMarks) map[string]ToolMarks {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ToolMarks, len(in))
	for tool, tm := range in {
		marks := make(map[string][]MarkRange, len(tm.Marks))
		for p, r := range tm.Marks {
			marks[p] = append([]MarkRange(nil), r...)
		}
		coverage := make(map[string]Coverage, len(tm.Coverage))
		for p, c := range tm.Coverage {
			coverage[p] = c
		}
		out[tool] = ToolMarks{Round: tm.Round, Marks: marks, Coverage: coverage}
	}
	return out
}
