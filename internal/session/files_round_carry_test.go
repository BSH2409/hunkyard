package session

import (
	"os"
	"path/filepath"
	"testing"
)

func commentLine(t *testing.T, s *Session, path string) int {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.Files {
		if filepath.Base(f.Path) == path && len(f.Comments) == 1 {
			return f.Comments[0].StartLine
		}
	}
	t.Fatalf("%s: comment missing", path)
	return 0
}

// The agent inserts two lines above a commented line in a markdown file and
// in a code file, and signals round complete before the watcher polls.
func TestFilesRoundCompleteRemapsCommentsEditedBeforeAPoll(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.md")
	code := filepath.Join(dir, "main.go")
	if err := os.WriteFile(plan, []byte("# Plan\n\nstep one\nstep two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package main\n\nfunc a() {}\nfunc b() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewSessionFromFiles([]string{plan, code}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.ReviewDir = filepath.Join(dir, ".hunkyard")
	s.captureBaselineAndPersist()
	for _, f := range s.Files {
		quote := "step one"
		if filepath.Base(f.Path) == "main.go" {
			quote = "func a() {}"
		}
		if _, ok := s.AddComment(f.Path, 3, 3, "", "look here", quote, "reviewer"); !ok {
			t.Fatalf("%s: comment not added", f.Path)
		}
	}
	s.WriteFiles()

	if err := os.WriteFile(plan, []byte("# Plan\n\nintro\nmore\nstep one\nstep two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package main\n\nimport \"fmt\"\n\nfunc a() {}\nfunc b() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.SignalRoundComplete()
	s.handleRoundCompleteFiles()

	if got := commentLine(t, s, "plan.md"); got != 5 {
		t.Errorf("plan.md comment at line %d, want 5", got)
	}
	if got := commentLine(t, s, "main.go"); got != 5 {
		t.Errorf("main.go comment at line %d, want 5", got)
	}
	s.mu.RLock()
	r2 := s.RoundSnapshots[s.Files[0].Path][2].Content
	s.mu.RUnlock()
	if r2 != "# Plan\n\nintro\nmore\nstep one\nstep two\n" {
		t.Errorf("round 2 snapshot = %q, want the agent's new content", r2)
	}
}

// A round in which a file did not change must not run the previous round's
// line mapping over comments that were already moved. The repeated TODO lines
// make a wrong destination pass anchor verification.
func TestFilesRoundWithoutEditsLeavesCommentsWhereTheyAre(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(notes, []byte("# Notes\n\na\nb\nc\nTODO\nfirst\nTODO\nsecond\nTODO\nthird\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := NewSessionFromFiles([]string{notes}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.ReviewDir = filepath.Join(dir, ".hunkyard")
	s.captureBaselineAndPersist()
	if _, ok := s.AddComment(s.Files[0].Path, 8, 8, "", "this one", "TODO", "reviewer"); !ok {
		t.Fatal("comment not added")
	}
	s.WriteFiles()
	// Round 1 to 2: the agent deletes three lines at the top.
	if err := os.WriteFile(notes, []byte("# Notes\n\nTODO\nfirst\nTODO\nsecond\nTODO\nthird\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.SignalRoundComplete()
	s.handleRoundCompleteFiles()
	if got := commentLine(t, s, "notes.md"); got != 5 {
		t.Fatalf("after the edit the comment is at line %d, want 5", got)
	}
	// Round 2 to 3 and 3 to 4: no edits at all.
	for round := 3; round <= 4; round++ {
		s.SignalRoundComplete()
		s.handleRoundCompleteFiles()
		if got := commentLine(t, s, "notes.md"); got != 5 {
			t.Fatalf("an unedited round (%d) moved the comment to line %d, want it to stay at 5", round, got)
		}
	}
}
