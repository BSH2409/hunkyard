package session

import (
	"os"
	"strings"

	"github.com/BSH2409/hunkyard/internal/diff"
)

// ReviewerEdit is one run of lines the reviewer changed on disk while
// reviewing. Lines are numbered in the agent's version the reviewer saw; a
// pure insertion anchors to the line before it.
type ReviewerEdit struct {
	Path      string
	StartLine int
	EndLine   int
	Before    []string
	After     []string
}

// ReviewerEdits compares each reviewed text file on disk with the content the
// review displays and returns the reviewer's changed runs. Binary, deleted,
// unloaded and orphaned files are skipped. It reports nothing when disk is not
// the reviewed source (range focus shows commit blobs) or when the agent
// worked during this round, since its edits would look like the reviewer's.
func (s *Session) ReviewerEdits() []ReviewerEdit {
	type displayed struct{ path, abs, content string }
	var reviewed []displayed
	s.mu.RLock()
	if s.Focus.Kind == FocusRange || s.agentActiveInRoundLocked() {
		s.mu.RUnlock()
		return nil
	}
	for _, f := range s.Files {
		if f == nil || f.Lazy || f.Orphaned || f.FileType == "binary" || f.Status == "deleted" || f.AbsPath == "" {
			continue
		}
		reviewed = append(reviewed, displayed{f.Path, f.AbsPath, f.Content})
	}
	s.mu.RUnlock()
	var edits []ReviewerEdit
	for _, f := range reviewed {
		disk, err := os.ReadFile(f.abs)
		if err != nil || string(disk) == f.content {
			continue
		}
		edits = append(edits, editRuns(f.path, f.content, string(disk))...)
	}
	return edits
}

func editRuns(path, before, after string) []ReviewerEdit {
	var edits []ReviewerEdit
	var run *ReviewerEdit
	lastOld := 0
	flush := func() {
		if run != nil {
			edits = append(edits, *run)
			run = nil
		}
	}
	// Line-ending conversions by an editor are not edits.
	normalize := func(text string) string { return strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n") }
	for _, e := range diff.ComputeLineDiff(normalize(before), normalize(after)) {
		if e.Type == "unchanged" {
			flush()
			lastOld = e.OldLine
			continue
		}
		if run == nil {
			anchor := max(lastOld, 1)
			run = &ReviewerEdit{Path: path, StartLine: anchor, EndLine: anchor}
		}
		if e.Type == "removed" {
			if len(run.Before) == 0 {
				run.StartLine = e.OldLine
			}
			run.EndLine = e.OldLine
			run.Before = append(run.Before, e.Text)
		} else {
			run.After = append(run.After, e.Text)
		}
	}
	flush()
	return edits
}

// agentActiveInRoundLocked reports an agent request sent or an agent reply
// posted during the current round.
func (s *Session) agentActiveInRoundLocked() bool {
	active := func(comments []Comment) bool {
		for _, c := range comments {
			if c.Live {
				return true
			}
			for _, reply := range c.Replies {
				if reply.AuthorKind == "agent" && reply.ReviewRound == s.ReviewRound {
					return true
				}
			}
		}
		return false
	}
	for _, f := range s.Files {
		if f != nil && active(f.Comments) {
			return true
		}
	}
	return active(s.reviewComments)
}
