package preferences

import (
	"testing"
)

func TestDiffFindingsUsesOnlyActiveDiffCheckGuards(t *testing.T) {
	s, ruleID := guardedStore(t, "diff_check", "js_console_log")
	if _, err := s.addGuard("staged_paths", ruleID, []string{"docs/**"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	lines := []AddedLine{
		{Path: "web/app.js", Number: 7, Text: "  console.log('debug');"},
		{Path: "web/app.js", Number: 8, Text: "  return x;"},
		{Path: "README.md", Number: 3, Text: "console.log in docs is fine"},
	}
	findings := s.DiffFindings("", lines)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v", findings)
	}
	f := findings[0]
	if f.RuleID != ruleID || f.Path != "web/app.js" || f.Line != 7 || f.Check != "js_console_log" || f.Directive == "" || f.Message == "" {
		t.Fatalf("finding = %+v", f)
	}
	if _, err := s.retire(ruleID); err != nil {
		t.Fatal(err)
	}
	if got := s.DiffFindings("", lines); len(got) != 0 {
		t.Fatalf("retired rule still checked: %+v", got)
	}
}

func TestDiffFindingsNewChecks(t *testing.T) {
	cases := []struct {
		check, path, bad, good string
	}{
		{"js_console_log", "a.ts", "console.debug(x)", "logger.debug(x)"},
		{"todo_without_owner", "a.go", "// TODO fix later", "// TODO(bhartik): fix later"},
		{"skipped_test_added", "a_test.go", "\tt.Skip(\"flaky\")", "\tt.Run(\"x\", f)"},
		{"skipped_test_added", "a.spec.ts", "test.skip('x', () => {})", "test('x', () => {})"},
		{"skipped_test_added", "spec/a_spec.rb", "  xit 'works' do", "  it 'works' do"},
	}
	for _, tc := range cases {
		t.Run(tc.check+" "+tc.path, func(t *testing.T) {
			check := diffChecks[tc.check]
			if check == nil {
				t.Fatalf("missing check %s", tc.check)
			}
			if got := check(tc.path, []string{tc.bad}); len(got) != 1 {
				t.Fatalf("%q not flagged: %v", tc.bad, got)
			}
			if got := check(tc.path, []string{tc.good}); len(got) != 0 {
				t.Fatalf("%q flagged: %v", tc.good, got)
			}
		})
	}
}

func TestRecordRuleVerdictAppendsActivity(t *testing.T) {
	s, ruleID := guardedStore(t, "diff_check", "js_console_log")
	if err := s.RecordRuleVerdict(ruleID, "", "dismissed"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRuleVerdict(ruleID, "", "confirmed"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRuleVerdict(ruleID, "", "maybe"); err == nil {
		t.Fatal("unknown verdict accepted")
	}
	rules, err := s.YardRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.ID == ruleID && (r.Dismissals != 1 || r.Violations != 1) {
			t.Fatalf("yard counts = %+v", r)
		}
	}
}

func TestRulesForObservations(t *testing.T) {
	s := testStore(t)
	result, err := s.Capture(CaptureInput{Feedback: "Name things well.", Directive: "Name things well.", Scope: "global", Surfaces: []string{"naming"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	got := s.RulesForObservations([]string{result.ID, "obs_missing"})
	if len(got) != 1 || got[result.ID].Directive != "Name things well." || got[result.ID].ID == "" {
		t.Fatalf("lookup = %+v (observation %s)", got, result.ID)
	}
}

func TestRuleVerdictCounts(t *testing.T) {
	s, ruleID := guardedStore(t, "diff_check", "js_console_log")
	_ = s.RecordRuleVerdict(ruleID, "", "dismissed")
	_ = s.RecordRuleVerdict(ruleID, "", "dismissed")
	_ = s.RecordRuleVerdict(ruleID, "", "confirmed")
	if got := s.RuleVerdictCounts()[ruleID]; got != (VerdictCount{Confirmed: 1, Dismissed: 2}) {
		t.Fatalf("counts = %+v", got)
	}
}
