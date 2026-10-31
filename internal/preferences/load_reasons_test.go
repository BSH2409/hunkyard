package preferences

import (
	"strings"
	"testing"
	"time"
)

func ruleIDFor(t *testing.T, s *Store, fragment string) string {
	t.Helper()
	rules, err := s.YardRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if strings.Contains(r.Directive, fragment) {
			return r.ID
		}
	}
	t.Fatalf("no rule contains %q", fragment)
	return ""
}

func TestLoadReasonsRecordedPerPath(t *testing.T) {
	s := testStore(t)
	captureCoreRule(t, s)
	captureRule(t, s, "Use only info and error log levels in Ruby services.", "ruby")
	captureRule(t, s, "Prefer domain names for database columns.", "database")
	captureRule(t, s, "Prefer CSS variables for colours.", "frontend")
	sessionStartResponse(t, s, t.TempDir())
	promptContext(t, s, "s1", "add logging to the ruby service for slow requests")
	promptContext(t, s, "s1", "now design the database migration for this table")
	editContext(t, s, "Edit", map[string]any{"file_path": "/repo/web/theme.css"})
	reasons := s.loadSession("s1").LoadReasons
	want := map[string]string{
		ruleIDFor(t, s, "naming-analyzer"):  "core (session start)",
		ruleIDFor(t, s, "log levels"):       `matched "ruby" in your prompt`,
		ruleIDFor(t, s, "database columns"): `matched "database" in your prompt`,
		ruleIDFor(t, s, "CSS variables"):    "editing /repo/web/theme.css",
	}
	for id, reason := range want {
		if reasons[id] != reason {
			t.Errorf("rule %s: got %q, want %q (all: %v)", id, reasons[id], reason, reasons)
		}
	}
}

func TestStickyRuleWithoutReasonIsKept(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Use only info and error log levels in Ruby services.", "ruby")
	id := ruleIDFor(t, s, "log levels")
	if err := s.updateSession("s1", func(state *sessionState) { state.InjectedRuleIDs = []string{id} }); err != nil {
		t.Fatal(err)
	}
	promptContext(t, s, "s1", "now design the database migration for this table")
	if got := s.loadSession("s1").LoadReasons[id]; got != "kept from earlier in this session" {
		t.Fatalf("sticky reason: %q", got)
	}
}

func TestYardLoadsNewestSessionFirst(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Use only info and error log levels in Ruby services.", "ruby")
	promptContext(t, s, "older", "add logging to the ruby service")
	s.config.Clock = func() time.Time { return fixedNow.Add(time.Hour) }
	promptContext(t, s, "newer", "add logging to the ruby service")
	loads, err := s.YardLoads()
	if err != nil {
		t.Fatal(err)
	}
	if len(loads) != 2 || loads[0].UpdatedAt <= loads[1].UpdatedAt {
		t.Fatalf("not newest first: %+v", loads)
	}
	if len(loads[0].Rules) != 1 || !strings.Contains(loads[0].Rules[0].Directive, "log levels") || loads[0].Rules[0].Reason != `matched "ruby" in your prompt` {
		t.Fatalf("load entry: %+v", loads[0].Rules)
	}
}
