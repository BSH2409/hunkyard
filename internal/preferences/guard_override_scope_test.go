package preferences

import (
	"testing"
)

func hasOverrideFor(s *Store, ruleID string) bool {
	for _, o := range s.loadOverrides() {
		if o.RuleID == ruleID {
			return true
		}
	}
	return false
}

// Two rules block one command: an allow-once for only one of them leaves the
// command blocked and must not be spent.
func TestAllowOnceIsKeptWhileAnotherRuleStillBlocks(t *testing.T) {
	s, planRule := guardedStore(t, "staged_paths", "*-plan.md")
	captureRule(t, s, "Never add co-author trailers.", "devops")
	current, _ := s.loadCurrent()
	var trailerRule string
	for _, r := range current.Rules.rules {
		if id := textField(r["id"]); id != planRule {
			trailerRule = id
		}
	}
	if _, err := s.addGuard("no_coauthor_trailer", trailerRule, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	repo := guardRepo(t)
	stage(t, repo, "release-plan.md")
	command := "git commit -m \"x\n\nCo-authored-by: A <a@example.com>\""
	if _, err := s.allowOnce(planRule); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, command)); !blocked {
		t.Fatal("the rule without an override did not block")
	}
	if !hasOverrideFor(s, planRule) {
		t.Fatal("the allow-once was spent on a command that stayed blocked")
	}
	if _, err := s.allowOnce(trailerRule); err != nil {
		t.Fatal(err)
	}
	if blocked, reason := denied(preToolUse(t, s, "s1", repo, command)); blocked {
		t.Fatalf("both rules allowed once, still blocked: %s", reason)
	}
	if hasOverrideFor(s, planRule) || hasOverrideFor(s, trailerRule) {
		t.Fatal("overrides not consumed when the command passed")
	}
}

// `git -C other-repo` is checked against the other repository's rules.
func TestGuardScopeFollowsTheCommandsRepository(t *testing.T) {
	s := testStore(t)
	other := guardRepo(t)
	key, _ := RepositoryKey(other)
	result, err := s.Capture(CaptureInput{Feedback: "Keep plans out of git.", Directive: "Keep plans out of git.", Scope: "repository", RepoKey: key, Surfaces: []string{"devops"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "seed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addGuard("staged_paths", result.RuleID, []string{"*-plan.md"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	stage(t, other, "release-plan.md")
	here := guardRepo(t)
	if blocked, _ := denied(preToolUse(t, s, "s1", here, "git -C "+other+" commit -m x")); !blocked {
		t.Fatal("the other repository's guard did not apply to a commit there")
	}
}
