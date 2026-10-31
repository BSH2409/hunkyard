package preferences

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestGuardExamplesRejectBadGuard(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Keep plans out of git.", "devops")
	current, _ := s.loadCurrent()
	ruleID := textField(current.Rules.rules[0]["id"])
	if _, err := s.addGuard("staged_paths", ruleID, []string{"docs/plans/**"}, []string{"docs/superpowers/a.md"}, nil); err == nil || !strings.Contains(err.Error(), "docs/superpowers/a.md") {
		t.Fatalf("guard missing its own match example was accepted: %v", err)
	}
	if _, err := s.addGuard("commit_message_format", ruleID, []string{`^Refs #\d+: `}, nil, []string{`git commit -m "fix it"`}); err == nil {
		t.Fatal("guard blocking its own not-match example was accepted")
	}
	if _, err := s.addGuard("commit_message_format", ruleID, []string{`^Refs #\d+: `}, []string{`git commit -m "fix it"`}, []string{`git commit -m "Refs #4: fix it"`, "git status"}); err != nil {
		t.Fatalf("valid examples rejected: %v", err)
	}
	if _, err := s.addGuard("diff_check", ruleID, []string{"ruby_single_line_def"}, []string{"app.rb:def a; 1; end"}, []string{"app.rb:def a"}); err != nil {
		t.Fatalf("diff check examples rejected: %v", err)
	}
}

func TestCheckReportsWithoutSideEffects(t *testing.T) {
	s, ruleID := guardedStore(t, "staged_paths", "*-plan.md")
	repo := guardRepo(t)
	stage(t, repo, "release-plan.md")
	if _, err := s.allowOnce(ruleID); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Activity(100)
	var out bytes.Buffer
	if exit := Run([]string{"check", "--root", s.config.Root, "--cwd", repo, "--", "git", "commit", "-m", "x"}, strings.NewReader(""), &out, &bytes.Buffer{}); exit != 0 {
		t.Fatalf("exit %d: %s", exit, out.String())
	}
	var result map[string]any
	_ = json.Unmarshal(out.Bytes(), &result)
	if result["blocked"] != true || result["rule_id"] != ruleID {
		t.Fatalf("check result: %v", result)
	}
	after, _ := s.Activity(100)
	if len(after) != len(before) || len(s.loadOverrides()) != 1 {
		t.Fatal("check recorded activity or consumed the override")
	}
}

func TestGuardTestRerunsExamples(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "No co-author trailers.", "devops")
	current, _ := s.loadCurrent()
	ruleID := textField(current.Rules.rules[0]["id"])
	if _, err := s.addGuard("no_coauthor_trailer", ruleID, nil, []string{"git commit -m \"x\n\nCo-authored-by: a <a@b>\""}, []string{`git commit -m "x"`}); err != nil {
		t.Fatal(err)
	}
	results := s.testGuards()
	if len(results) != 1 || results[0]["status"] != "pass" {
		t.Fatalf("guard test: %v", results)
	}
}
