package preferences

import (
	"strconv"
	"testing"
	"time"
)

func ruleFixture(scope string) rule {
	var repo any
	if scope == "repository" {
		repo = "blogvault-a1b2c3d4e5f6"
	}
	return rule{"id": "rule-1", "directive": "Prefer early returns over nested conditionals.", "scope": map[string]any{"kind": scope, "repo_key": repo}, "surfaces": []string{"ruby"}, "origin": "inferred", "status": "candidate", "observations": []any{}, "evidence": map[string]any{"observation_count": 0, "session_count": 0, "repository_count": 0, "explicit_count": 0}, "examples": []string{}, "created_at": "2026-08-19T12:00:00Z", "updated_at": "2026-08-19T12:00:00Z"}
}

func observationFixture(id, session, repo string) map[string]any {
	return map[string]any{"id": id, "captured_at": "2026-08-19T12:00:00Z", "source": "hook", "repo_key": repo, "session_key": session, "turn_key": "t1", "safe_excerpt": "Prefer early returns", "signals": map[string]any{"correction": true, "durable_language": false, "likely_task_local": false}, "suggested_surfaces": []string{"ruby"}}
}

func testCatalog(t *testing.T, s *Store, rules ...rule) *catalog {
	t.Helper()
	if rules == nil {
		rules = []rule{}
	}
	normalized, err := normalizeRules(rules, s.config.Clock())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.catalogFromGeneration(&generation{Rules: validatedRuleSet{issuer: s, rules: normalized}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRuleExplicitAndDuplicateEvidence(t *testing.T) {
	s := testStore(t)
	c := testCatalog(t, s)
	o := observationFixture("obs-first", "s1", "alpha-a1b2c3d4e5f6")
	r, err := c.upsertExplicit(o, "Prefer early returns over nested conditionals.", map[string]any{"kind": "global", "repo_key": nil}, []string{"ruby"})
	if err != nil {
		t.Fatal(err)
	}
	if r["status"] != "active" {
		t.Fatal("explicit evidence did not activate rule")
	}
	r, err = c.confirm(r["id"].(string), o)
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := integer(r["evidence"].(map[string]any)["explicit_count"]); count != 1 {
		t.Fatal("duplicate erased explicit evidence")
	}
	if count, _ := integer(r["evidence"].(map[string]any)["observation_count"]); count != 1 {
		t.Fatal("duplicate increased evidence")
	}
	set := c.ruleSet()
	r["directive"] = "forged"
	c.rules[0]["directive"] = "changed later"
	if set.rules[0]["directive"] != "Prefer early returns over nested conditionals." {
		t.Fatal("issued snapshot is mutable through caller result")
	}
}

func TestRuleIndependentEvidence(t *testing.T) {
	for _, kind := range []string{"repository", "global"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			c := testCatalog(t, s, ruleFixture(kind))
			var r rule
			for i, session := range []string{"s1", "s1", "s2"} {
				var err error
				r, err = c.confirm("rule-1", observationFixture(strconv.Itoa(i), session, "alpha-a1b2c3d4e5f6"))
				if err != nil {
					t.Fatal(err)
				}
				want := "candidate"
				if i == 2 && kind == "repository" {
					want = "active"
				}
				if r["status"] != want {
					t.Fatalf("status = %v, want %s", r["status"], want)
				}
			}
			if kind == "global" {
				var err error
				r, err = c.confirm("rule-1", observationFixture("other", "s3", "beta-a1b2c3d4e5f6"))
				if err != nil || r["status"] != "active" {
					t.Fatalf("independent repository evidence: %v", err)
				}
			}
		})
	}
}

func TestRuleCoreDormancyAndRetirement(t *testing.T) {
	s := testStore(t)
	c := testCatalog(t, s, ruleFixture("repository"))
	var r rule
	for i, session := range []string{"s1", "s1", "s2", "s2", "s3"} {
		var err error
		r, err = c.confirm("rule-1", observationFixture(strconv.Itoa(i), session, "alpha-a1b2c3d4e5f6"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if r["status"] != "core" {
		t.Fatal("five observations in three sessions did not promote core")
	}
	r, err := c.retire("rule-1")
	if err != nil || r["status"] != "superseded" {
		t.Fatalf("retire: %v", err)
	}
	if count, _ := integer(r["evidence"].(map[string]any)["observation_count"]); count != 5 {
		t.Fatal("retirement discarded evidence")
	}
	old := ruleFixture("repository")
	old["updated_at"] = s.config.Clock().Add(-180 * 24 * time.Hour).Format(time.RFC3339)
	if err := recomputeRule(old, s.config.Clock()); err != nil || old["status"] != "dormant" {
		t.Fatalf("dormancy: %v", err)
	}
}

func TestRuleGraphRejectsInvalidEdges(t *testing.T) {
	for _, kind := range []string{"cycle", "inverse-cycle", "self-conflict", "unknown-conflict", "one-way-conflict", "active-conflict"} {
		t.Run(kind, func(t *testing.T) {
			a := ruleFixture("repository")
			b := ruleFixture("repository")
			b["id"] = "rule-2"
			switch kind {
			case "cycle":
				a["supersedes"] = []string{"rule-2"}
				b["supersedes"] = []string{"rule-1"}
			case "inverse-cycle":
				a["supersedes"] = []string{"rule-2"}
				a["superseded_by"] = "rule-2"
			case "self-conflict":
				a["conflicts_with"] = []string{"rule-1"}
			case "unknown-conflict":
				a["conflicts_with"] = []string{"missing"}
			case "one-way-conflict":
				a["conflicts_with"] = []string{"rule-2"}
			case "active-conflict":
				a["conflicts_with"] = []string{"rule-2"}
				b["conflicts_with"] = []string{"rule-1"}
				a["status"] = "active"
			}
			if err := validateRuleGraph([]rule{a, b}); err == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
}

func TestRuleStrictFieldsAndExamples(t *testing.T) {
	for _, mutate := range []func(rule){func(r rule) { r["model_note"] = "trust me" }, func(r rule) { r["created_at"] = time.Now() }, func(r rule) { r["source_pack"] = "code-like-him" }, func(r rule) { r["origin"] = "source_pack" }, func(r rule) { r["examples"] = []string{"password=synthetic"} }, func(r rule) { r["examples"] = []string{"a", "b", "c"} }} {
		r := ruleFixture("repository")
		mutate(r)
		if err := validateRule(r); err == nil {
			t.Fatal("invalid rule accepted")
		}
	}
}

func TestRuleReferenceParity(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Name          string
			Seed, Initial []rule
			Steps         []struct {
				Action   map[string]any
				Expected rule
			}
			ExpectedRules []rule `json:"expected_rules"`
		}
	}
	readContract(t, "rules.json", &fixtures)
	for _, fixture := range fixtures.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			s := testStore(t)
			c := testCatalog(t, s, fixture.Seed...)
			if !canonicalEqual(c.rules, fixture.Initial) {
				t.Fatal("initial rule normalization differs from Ruby")
			}
			for _, step := range fixture.Steps {
				var got rule
				var err error
				action := step.Action
				switch action["type"] {
				case "upsert":
					surfaces, decodeErr := stringArray(action["surfaces"])
					if decodeErr != nil {
						t.Fatal(decodeErr)
					}
					got, err = c.upsertExplicit(action["observation"].(map[string]any), action["directive"].(string), action["scope"].(map[string]any), surfaces)
				case "confirm":
					got, err = c.confirm(action["rule_id"].(string), action["observation"].(map[string]any))
				case "retire":
					got, err = c.retire(action["rule_id"].(string))
				default:
					t.Fatal("unknown fixture action")
				}
				if err != nil {
					t.Fatal(err)
				}
				if !canonicalEqual(got, step.Expected) {
					t.Fatalf("rule transition %v differs from Ruby", action["type"])
				}
			}
			if !canonicalEqual(c.rules, fixture.ExpectedRules) {
				t.Fatal("final catalog differs from Ruby")
			}
		})
	}
}
