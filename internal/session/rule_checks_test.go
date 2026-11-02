package session

import (
	"errors"
	"testing"

	"github.com/BSH2409/hunkyard/internal/preferences"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

// consoleChecker flags every added line containing "c" as violating rule_c.
func consoleChecker(calls *[][]preferences.AddedLine) RuleChecker {
	return func(lines []preferences.AddedLine) []preferences.DiffFinding {
		*calls = append(*calls, lines)
		var out []preferences.DiffFinding
		for _, l := range lines {
			if l.Text == "c" {
				out = append(out, preferences.DiffFinding{RuleID: "rule_c", Directive: "Avoid c.", Check: "x", Path: l.Path, Line: l.Number, Message: "c added"})
			}
		}
		return out
	}
}

func TestRunRuleChecksPostsFindingsOnAddedLines(t *testing.T) {
	s := machineFixture(t)
	var calls [][]preferences.AddedLine
	s.SetRuleChecker(consoleChecker(&calls))
	got := s.RunRuleChecks()
	if got.Added != 1 {
		t.Fatalf("result = %+v", got)
	}
	if len(calls) != 1 || len(calls[0]) != 2 || calls[0][0] != (preferences.AddedLine{Path: "server.go", Number: 2, Text: "b"}) {
		t.Fatalf("checker saw %+v", calls)
	}
	c := machineComments(s, "server.go")
	if len(c) != 1 || c[0].Source != "rule" || c[0].SourceRef != "rule_c" || c[0].StartLine != 3 || c[0].Author != "hunkyard" || c[0].Body != "Likely violates: Avoid c.\nc added" {
		t.Fatalf("rule comment = %+v", c)
	}
}

func TestRunRuleChecksRegeneratesEachRound(t *testing.T) {
	s := machineFixture(t)
	var calls [][]preferences.AddedLine
	s.SetRuleChecker(consoleChecker(&calls))
	s.RunRuleChecks()
	if again := s.RunRuleChecks(); again != (ReplaceResult{Kept: 1}) {
		t.Fatalf("second pass = %+v", again)
	}
	s.fileByPathLocked("server.go").DiffHunks[0].Lines[2].Content = "fixed"
	if fixed := s.RunRuleChecks(); fixed != (ReplaceResult{Removed: 1}) {
		t.Fatalf("pass after the fix = %+v", fixed)
	}
}

func TestRunRuleChecksKeepsDismissedFinding(t *testing.T) {
	s := machineFixture(t)
	var calls [][]preferences.AddedLine
	s.SetRuleChecker(consoleChecker(&calls))
	s.RunRuleChecks()
	id := machineComments(s, "server.go")[0].ID
	if _, err := s.SetRuleVerdict("server.go", id, "dismissed"); err != nil {
		t.Fatal(err)
	}
	s.RunRuleChecks()
	c := machineComments(s, "server.go")
	if len(c) != 1 || c[0].ID != id || !c[0].Resolved || c[0].Verdict != "dismissed" {
		t.Fatalf("dismissed finding re-posted or reopened: %+v", c)
	}
}

func TestRunRuleChecksWithoutCheckerDoesNothing(t *testing.T) {
	s := machineFixture(t)
	if got := s.RunRuleChecks(); got != (ReplaceResult{}) {
		t.Fatalf("result = %+v", got)
	}
}

func TestSetRuleVerdict(t *testing.T) {
	s := machineFixture(t)
	var calls [][]preferences.AddedLine
	s.SetRuleChecker(consoleChecker(&calls))
	var recorded []string
	s.SetRuleVerdictRecorder(func(ruleID, verdict string) error {
		recorded = append(recorded, ruleID+":"+verdict)
		return nil
	})
	s.RunRuleChecks()
	s.ReplaceMachineComments("tool", "lint", []MachineComment{{Path: "server.go", Line: 2, EndLine: 2, Body: "lint", Author: "lint"}})
	var ruleID, toolID string
	for _, c := range machineComments(s, "server.go") {
		if c.Source == "rule" {
			ruleID = c.ID
		} else {
			toolID = c.ID
		}
	}
	cases := []struct {
		name, id, verdict string
		wantErr           error
	}{
		{"bad verdict", ruleID, "maybe", ErrInvalidVerdict},
		{"unknown comment", "c_missing", "confirmed", ErrCommentNotFound},
		{"not a rule finding", toolID, "confirmed", ErrNotRuleFinding},
		{"confirm", ruleID, "confirmed", nil},
	}
	for _, tc := range cases {
		if _, err := s.SetRuleVerdict("server.go", tc.id, tc.verdict); !errors.Is(err, tc.wantErr) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
		}
	}
	if len(recorded) != 1 || recorded[0] != "rule_c:confirmed" {
		t.Fatalf("recorded = %v", recorded)
	}
}

// ruleOnC flags every added line "c" as violating rule_c.
func ruleOnC(lines []preferences.AddedLine) []preferences.DiffFinding {
	var calls [][]preferences.AddedLine
	return consoleChecker(&calls)(lines)
}

func TestDismissedFindingFollowsItsLine(t *testing.T) {
	s := machineFixture(t)
	var calls [][]preferences.AddedLine
	s.SetRuleChecker(consoleChecker(&calls))
	s.RunRuleChecks()
	id := machineComments(s, "server.go")[0].ID
	if _, err := s.SetRuleVerdict("server.go", id, "dismissed"); err != nil {
		t.Fatal(err)
	}
	// A line inserted above moves "c" from line 3 to line 4.
	f := s.fileByPathLocked("server.go")
	f.Content = "a\nnew\nb\nc\nd\n"
	f.DiffHunks = []vcs.DiffHunk{{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 5, Lines: []vcs.DiffLine{
		{Type: "context", Content: "a", OldNum: 1, NewNum: 1},
		{Type: "add", Content: "new", NewNum: 2},
		{Type: "add", Content: "b", NewNum: 3},
		{Type: "add", Content: "c", NewNum: 4},
		{Type: "context", Content: "d", OldNum: 2, NewNum: 5},
	}}}
	if got := s.RunRuleChecks(); got != (ReplaceResult{Kept: 1}) {
		t.Fatalf("moved finding = %+v", got)
	}
	c := machineComments(s, "server.go")
	if len(c) != 1 || c[0].ID != id || c[0].StartLine != 4 || c[0].Verdict != "dismissed" || !c[0].Resolved {
		t.Fatalf("finding = %+v", c)
	}
}
