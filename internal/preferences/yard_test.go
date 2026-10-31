package preferences

import (
	"testing"
)

func TestYardEmptyStore(t *testing.T) {
	s := testStore(t)
	rules, err := s.YardRules()
	if err != nil || len(rules) != 0 {
		t.Fatalf("rules on empty store: %v %v", rules, err)
	}
	persona, err := s.YardPersona()
	if err != nil || len(persona) != 0 {
		t.Fatalf("persona on empty store: %v %v", persona, err)
	}
	pipeline, err := s.YardPipeline()
	if err != nil || pipeline["active"] != 0 {
		t.Fatalf("pipeline on empty store: %v %v", pipeline, err)
	}
	if health := s.YardHealth(); health["checks"] == nil {
		t.Fatalf("health without checks: %v", health)
	}
}

func TestYardSeededStore(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Write Ruby method definitions over multiple lines.", "ruby")
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	rules, err := s.YardRules()
	if err != nil || len(rules) != 1 || rules[0].Status != "active" || rules[0].ScopeKind != "global" || rules[0].Observations != 1 {
		t.Fatalf("rules: %+v %v", rules, err)
	}
	if err := s.recordActivity(ActivityEvent{Kind: activityViolated, RuleID: rules[0].ID}); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.YardRules(); again[0].Violations != 1 {
		t.Fatalf("violations not counted: %+v", again[0])
	}
	persona, _ := s.YardPersona()
	if _, ok := persona["core.md"]; !ok {
		t.Fatalf("persona files: %v", persona)
	}
	pipeline, _ := s.YardPipeline()
	captured, _ := pipeline["captured"].([]map[string]any)
	if pipeline["active"] != 1 || len(captured) != 1 {
		t.Fatalf("pipeline: %v", pipeline)
	}
}

func TestYardPromoteAndRetire(t *testing.T) {
	s := testStore(t)
	input := CaptureInput{Feedback: "keep tests local", Directive: "Keep test files out of commits.", Scope: "repository", RepoKey: "orion-a1b2c3d4e5f6", Surfaces: []string{"testing"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "seed"}
	result, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.YardPromote(result.RuleID); err != nil {
		t.Fatal(err)
	}
	rules, _ := s.YardRules()
	global := ""
	for _, r := range rules {
		if r.ScopeKind == "global" && r.Directive == "Keep test files out of commits." && r.Status == "active" {
			global = r.ID
		}
	}
	if global == "" {
		t.Fatalf("promote did not create a global rule: %+v", rules)
	}
	if err := s.YardRetire(global); err != nil {
		t.Fatal(err)
	}
	rules, _ = s.YardRules()
	for _, r := range rules {
		if r.ID == global && r.Status == "active" {
			t.Fatal("retired rule still active")
		}
	}
	if _, err := s.YardCreateRule("Prefer early returns.", "global", "", []string{"ruby"}, "Better formatting is required here"); err != nil {
		t.Fatal(err)
	}
}

func TestYardPromoteRejectsGlobalRule(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Prefer CSS variables for colours.", "frontend")
	if err := s.YardPromote(ruleIDFor(t, s, "CSS variables")); err == nil {
		t.Fatal("promoting a global rule should fail")
	}
	rules, _ := s.YardRules()
	if len(rules) != 1 {
		t.Fatalf("promotion duplicated the rule: %d rules", len(rules))
	}
}
