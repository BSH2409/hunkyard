package preferences

import (
	"strings"
	"testing"
	"time"
)

var packNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func teamRule(t *testing.T, directive string, surfaces ...string) rule {
	t.Helper()
	r := newRule(directive, map[string]any{"kind": "global", "repo_key": nil}, surfaces, "team_pack", packNow)
	r["pack"] = "shared-backend"
	if err := recomputeRule(r, packNow); err != nil {
		t.Fatal(err)
	}
	return r
}

func personalRule(t *testing.T, directive string, surfaces ...string) rule {
	t.Helper()
	r := newRule(directive, map[string]any{"kind": "global", "repo_key": nil}, surfaces, "feedback", packNow)
	if err := addObservation(r, map[string]any{"id": "obs_" + digest(directive)[:8], "session_key": "s", "repo_key": nil, "explicit": true}, true, packNow); err != nil {
		t.Fatal(err)
	}
	if err := recomputeRule(r, packNow); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTeamPackRuleValidation(t *testing.T) {
	r := teamRule(t, "Return early from handlers.", "api")
	if err := validateRule(r); err != nil {
		t.Fatalf("team pack rule rejected: %v", err)
	}
	if r["status"] != "active" {
		t.Fatalf("team pack rule status = %v, want active without observations", r["status"])
	}
	missing := rule(cloneMap(r))
	delete(missing, "pack")
	if validateRule(missing) == nil {
		t.Fatal("team pack rule without its pack accepted")
	}
	misplaced := personalRule(t, "Name things well.", "naming")
	misplaced["pack"] = "shared-backend"
	if validateRule(misplaced) == nil {
		t.Fatal("pack field accepted on a personal rule")
	}
}

func TestSelectPersonaPlacesTeamPacksBetweenPersonalAndSource(t *testing.T) {
	personal := personalRule(t, "Prefer table tests in Go.", "testing")
	team := teamRule(t, "Return early from handlers.", "testing")
	shadowed := teamRule(t, "Prefer table tests in Go.", "testing")
	shadowed["id"] = "rule_shadowed"
	source := newRule("Keep functions small.", map[string]any{"kind": "global", "repo_key": nil}, []string{"testing", "naming"}, "source_pack", packNow)
	source["load_tier"], source["source_pack"], source["source_ref"] = "surface", "code-like-him", map[string]any{"file": "SKILL.md", "digest": strings.Repeat("a", 64)}
	_ = recomputeRule(source, packNow)
	got := selectPersona([]rule{source, team, shadowed, personal}, "write tests", "")
	text := got.Text
	p, tm, src := strings.Index(text, "Prefer table tests"), strings.Index(text, "Return early"), strings.Index(text, "Keep functions small")
	if p < 0 || tm < 0 || src < 0 || !(p < tm && tm < src) {
		t.Fatalf("order personal < team < source broken:\n%s", text)
	}
	if strings.Count(text, "Prefer table tests") != 1 {
		t.Fatalf("a team rule repeating a personal one was not blocked:\n%s", text)
	}
	if !strings.Contains(text, "Team pack defaults:") {
		t.Fatalf("missing team group title:\n%s", text)
	}
}

func TestTeamPackRuleCannotBeRetiredDirectly(t *testing.T) {
	s := testStore(t)
	c := &catalog{store: s, rules: []rule{teamRule(t, "Return early from handlers.", "api")}}
	if _, err := c.retire(textField(c.rules[0]["id"])); err == nil {
		t.Fatal("team pack rule retired one by one")
	}
}
