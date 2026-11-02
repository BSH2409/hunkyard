package session

import (
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// machineFixture is a session with one git-mode file whose diff adds lines
// 2-3 next to context lines 1 and 4.
func machineFixture(t *testing.T) *Session {
	t.Helper()
	s := NewTestSession(t)
	s.Mode = "git"
	s.Files = append(s.Files, &FileEntry{
		Path: "server.go", Status: "modified", FileType: "code",
		Content: "a\nb\nc\nd\n", Comments: []Comment{},
		DiffHunks: []vcs.DiffHunk{{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 4, Lines: []vcs.DiffLine{
			{Type: "context", Content: "a", OldNum: 1, NewNum: 1},
			{Type: "add", Content: "b", NewNum: 2},
			{Type: "add", Content: "c", NewNum: 3},
			{Type: "context", Content: "d", OldNum: 2, NewNum: 4},
		}}},
	})
	return s
}

func machineComments(s *Session, path string) []Comment {
	out := []Comment{}
	for _, c := range s.fileByPathLocked(path).Comments {
		if c.Source != "" {
			out = append(out, c)
		}
	}
	return out
}

func TestReplaceMachineCommentsKeepsMatchedThreads(t *testing.T) {
	s := machineFixture(t)
	first := s.ReplaceMachineComments("tool", "golangci-lint", []MachineComment{
		{Path: "server.go", Line: 2, EndLine: 2, Body: "x is unused", Author: "golangci-lint"},
		{Path: "server.go", Line: 3, EndLine: 3, Body: "shadowed err", Author: "golangci-lint"},
	})
	if first != (ReplaceResult{Added: 2}) {
		t.Fatalf("first run = %+v", first)
	}
	kept := machineComments(s, "server.go")[0]
	if _, ok := s.AddReply("server.go", kept.ID, "Intentional.", "human"); !ok {
		t.Fatal("reply rejected")
	}
	second := s.ReplaceMachineComments("tool", "golangci-lint", []MachineComment{
		{Path: "server.go", Line: 2, EndLine: 2, Body: "x is unused", Author: "golangci-lint"},
	})
	if second != (ReplaceResult{Kept: 1, Removed: 1}) {
		t.Fatalf("second run = %+v", second)
	}
	left := machineComments(s, "server.go")
	if len(left) != 1 || left[0].ID != kept.ID || len(left[0].Replies) != 1 {
		t.Fatalf("reply thread lost: %+v", left)
	}
	if left[0].Source != "tool" || left[0].SourceRef != "golangci-lint" || left[0].Author != "golangci-lint" {
		t.Fatalf("provenance missing: %+v", left[0])
	}
}

func TestReplaceMachineCommentsLeavesOtherSourcesAlone(t *testing.T) {
	s := machineFixture(t)
	s.ReplaceMachineComments("tool", "eslint", []MachineComment{{Path: "server.go", Line: 2, EndLine: 2, Body: "a", Author: "eslint"}})
	s.ReplaceMachineComments("tool", "golangci-lint", nil)
	if got := len(machineComments(s, "server.go")); got != 1 {
		t.Fatalf("another tool's comments were removed: %d left", got)
	}
}

func TestReplaceMachineCommentsDropsOffDiffLines(t *testing.T) {
	s := machineFixture(t)
	got := s.ReplaceMachineComments("tool", "lint", []MachineComment{
		{Path: "server.go", Line: 9, EndLine: 9, Body: "beyond the file", Author: "lint"},
		{Path: "missing.go", Line: 1, EndLine: 1, Body: "not in review", Author: "lint"},
		{Path: "server.go", Line: 4, EndLine: 4, Body: "context line is fine", Author: "lint"},
	})
	if got != (ReplaceResult{Added: 1, Dropped: 2}) {
		t.Fatalf("result = %+v", got)
	}
}

func TestReplaceMachineCommentsFileLevel(t *testing.T) {
	s := machineFixture(t)
	s.ReplaceMachineComments("tool", "lint", []MachineComment{{Path: "server.go", Body: "missing header", Author: "lint"}})
	c := machineComments(s, "server.go")
	if len(c) != 1 || c[0].Scope != "file" || c[0].StartLine != 0 {
		t.Fatalf("file-level finding stored as %+v", c)
	}
}

func TestSetToolMarksCountsOnlyAddedLines(t *testing.T) {
	s := machineFixture(t)
	coverage := s.SetToolMarks("coverage", map[string][]MarkRange{
		"server.go":  {{Start: 1, End: 2, Kind: "uncovered"}},
		"missing.go": {{Start: 1, End: 1, Kind: "uncovered"}},
	})
	if coverage["server.go"] != (Coverage{Changed: 2, Uncovered: 1}) || len(coverage) != 1 {
		t.Fatalf("coverage = %+v", coverage)
	}
	marks := s.ToolMarks()["coverage"]
	if marks.Round != 1 || len(marks.Marks["server.go"]) != 1 || marks.Marks["server.go"][0] != (MarkRange{Start: 2, End: 2, Kind: "uncovered"}) {
		t.Fatalf("stored marks = %+v", marks)
	}
}

func TestToolMarksRoundTripReviewFile(t *testing.T) {
	s := machineFixture(t)
	s.ReviewFilePath = filepath.Join(t.TempDir(), "review")
	s.SetToolMarks("coverage", map[string][]MarkRange{"server.go": {{Start: 2, End: 3, Kind: "uncovered"}}})
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	cj, err := readHunkyardJSONFromDisk(s.ReviewFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cj.ToolMarks["coverage"].Coverage["server.go"]; got != (Coverage{Changed: 2, Uncovered: 2}) {
		t.Fatalf("persisted coverage = %+v", got)
	}
	reloaded := machineFixture(t)
	reloaded.ReviewFilePath = s.ReviewFilePath
	reloaded.loadHunkyardJSON()
	if _, ok := reloaded.ToolMarks()["coverage"]; !ok {
		t.Fatal("tool marks not restored on load")
	}
}

func TestClearAllCommentsClearsToolMarks(t *testing.T) {
	s := machineFixture(t)
	s.ReviewFilePath = filepath.Join(t.TempDir(), "review")
	s.SetToolMarks("coverage", map[string][]MarkRange{"server.go": {{Start: 2, End: 2, Kind: "uncovered"}}})
	s.ClearAllComments()
	if len(s.ToolMarks()) != 0 {
		t.Fatal("tool marks survived clearing")
	}
}
