package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

// fillComment sets every field of a Comment to a non-zero value.
func fillComment(t *testing.T) Comment {
	t.Helper()
	var c Comment
	v := reflect.ValueOf(&c).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x" + v.Type().Field(i).Name)
		case reflect.Int, reflect.Int64:
			f.SetInt(int64(i + 1))
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), 1, 1))
		default:
			t.Fatalf("fillComment: unhandled kind %s for %s", f.Kind(), v.Type().Field(i).Name)
		}
	}
	return c
}

// TestCarryForwardCommentCopiesEveryField fails when a new Comment field is
// not carried across rounds, the way tool and rule provenance once was.
func TestCarryForwardCommentCopiesEveryField(t *testing.T) {
	old := fillComment(t)
	old.DOMAnchor = nil // drift fields are carried only for code comments
	got := carryForwardComment(old, "now")
	exempt := map[string]bool{"UpdatedAt": true, "CarriedForward": true}
	gv, ov := reflect.ValueOf(got), reflect.ValueOf(old)
	for i := 0; i < gv.NumField(); i++ {
		name := gv.Type().Field(i).Name
		if exempt[name] {
			continue
		}
		if !reflect.DeepEqual(gv.Field(i).Interface(), ov.Field(i).Interface()) {
			t.Errorf("carryForwardComment drops %s", name)
		}
	}
}

func TestRoundCompleteKeepsToolAndRuleProvenance(t *testing.T) {
	dir := t.TempDir()
	identity := filepath.Join(dir, ".hunkyard")
	abs := filepath.Join(dir, "main.go")
	content := "a\nb\nc\n"
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	hunks := []vcs.DiffHunk{{OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 3, Lines: []vcs.DiffLine{
		{Type: "context", Content: "a", OldNum: 1, NewNum: 1}, {Type: "add", Content: "b", NewNum: 2}, {Type: "add", Content: "c", NewNum: 3},
	}}}
	v := &fakeWatchVCS{currentBranch: "feature", defaultBranch: "main",
		branchChanges: []vcs.FileChange{{Path: "main.go", Status: "modified"}},
		diffs:         map[string][]vcs.DiffHunk{"main.go": hunks}}
	s := &Session{Mode: "git", VCS: v, RepoRoot: dir, BaseRef: "main", Branch: "feature", ReviewFilePath: identity, ReviewRound: 1,
		subscribers: make(map[chan SSEEvent]struct{}),
		Files:       []*FileEntry{{Path: "main.go", AbsPath: abs, Status: "modified", FileType: "code", Content: content, DiffHunks: hunks, Comments: []Comment{}}}}
	s.ReplaceMachineComments("tool", "eslint", []MachineComment{{Path: "main.go", Line: 2, EndLine: 2, Body: "lint", Author: "eslint"}})
	s.SetRuleChecker(ruleOnC)
	s.RunRuleChecks()
	var ruleID string
	for _, c := range s.GetComments("main.go") {
		if c.Source == "rule" {
			ruleID = c.ID
		}
	}
	if _, err := s.SetRuleVerdict("main.go", ruleID, "dismissed"); err != nil {
		t.Fatal(err)
	}

	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	s.roundComplete = make(chan struct{}, 1)
	s.SignalRoundComplete()
	s.handleRoundCompleteGit()
	if again := s.RunRuleChecks(); again.Added != 0 {
		t.Fatalf("round complete re-posted a rule finding: %+v", again)
	}
	if again := s.ReplaceMachineComments("tool", "eslint", []MachineComment{{Path: "main.go", Line: 2, EndLine: 2, Body: "lint", Author: "eslint"}}); again.Added != 0 {
		t.Fatalf("round complete re-posted a tool finding: %+v", again)
	}
	for _, c := range s.GetComments("main.go") {
		if c.Source == "" {
			t.Fatalf("a machine comment lost its provenance across the round: %+v", c)
		}
		if c.Source == "rule" && (c.Verdict != "dismissed" || !c.Resolved) {
			t.Fatalf("dismissed rule finding came back open: %+v", c)
		}
	}
}
