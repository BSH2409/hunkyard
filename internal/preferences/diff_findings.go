package preferences

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// AddedLine is one line a change adds, as the review daemon sees it.
type AddedLine struct {
	Path   string
	Number int
	Text   string
}

// DiffFinding is one added line that an active diff_check guard flags.
type DiffFinding struct {
	RuleID    string `json:"rule_id"`
	Directive string `json:"directive"`
	Check     string `json:"check"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Message   string `json:"message"`
}

const (
	activityRuleConfirmed = "rule_confirmed"
	activityRuleDismissed = "rule_dismissed"
)

type diffCheckBinding struct {
	ruleID, directive, name string
}

// diffChecksFor returns the diff_check guards whose rule is active and
// applies to repoKey (global rules always do).
func (s *Store) diffChecksFor(repoKey string) []diffCheckBinding {
	var checks []diffCheckBinding
	for _, g := range s.loadGuards() {
		if g.Kind != "diff_check" || len(g.Patterns) == 0 || diffChecks[g.Patterns[0]] == nil {
			continue
		}
		r := s.activeRule(g.RuleID)
		if r == nil {
			continue
		}
		scope := record(r["scope"])
		if scope["kind"] == "repository" && textField(scope["repo_key"]) != repoKey {
			continue
		}
		checks = append(checks, diffCheckBinding{ruleID: g.RuleID, directive: textField(r["directive"]), name: g.Patterns[0]})
	}
	return checks
}

// DiffFindings runs every applicable diff_check guard over lines, file by
// file.
func (s *Store) DiffFindings(repoKey string, lines []AddedLine) []DiffFinding {
	checks := s.diffChecksFor(repoKey)
	if len(checks) == 0 {
		return nil
	}
	byPath := map[string][]AddedLine{}
	paths := []string{}
	for _, l := range lines {
		if _, seen := byPath[l.Path]; !seen {
			paths = append(paths, l.Path)
		}
		byPath[l.Path] = append(byPath[l.Path], l)
	}
	sort.Strings(paths)
	var findings []DiffFinding
	for _, c := range checks {
		for _, path := range paths {
			findings = append(findings, runDiffCheck(c, path, byPath[path])...)
		}
	}
	return findings
}

func runDiffCheck(c diffCheckBinding, path string, added []AddedLine) []DiffFinding {
	texts := make([]string, len(added))
	for i, l := range added {
		texts[i] = l.Text
	}
	var findings []DiffFinding
	for _, raw := range diffChecks[c.name](path, texts) {
		index, message, _ := strings.Cut(raw, ": ")
		i, err := strconv.Atoi(index)
		if err != nil || i < 0 || i >= len(added) {
			continue
		}
		findings = append(findings, DiffFinding{RuleID: c.ruleID, Directive: c.directive, Check: c.name, Path: path, Line: added[i].Number, Message: message})
	}
	return findings
}

// RecordRuleVerdict records a reviewer's verdict on a rule finding: a
// confirmation counts as a violation of the rule, a dismissal as evidence
// against the check.
func (s *Store) RecordRuleVerdict(ruleID, repoKey, verdict string) error {
	kind := map[string]string{"confirmed": activityRuleConfirmed, "dismissed": activityRuleDismissed}[verdict]
	if kind == "" {
		return fmt.Errorf("unknown verdict %q", verdict)
	}
	if s.activeRule(ruleID) == nil {
		return fmt.Errorf("unknown or inactive rule")
	}
	return s.recordActivity(ActivityEvent{Kind: kind, RuleID: ruleID, RepoKey: repoKey, Detail: "review " + verdict + " a rule finding"})
}

// RuleRef names a rule for reports.
type RuleRef struct {
	ID        string `json:"id"`
	Directive string `json:"directive"`
}

// VerdictCount is how often reviewers confirmed or dismissed a rule finding.
type VerdictCount struct {
	Confirmed int `json:"confirmed"`
	Dismissed int `json:"dismissed"`
}

// RulesForObservations maps observation IDs (as `feedback learn` reports
// them) to the current rule that holds each one.
func (s *Store) RulesForObservations(ids []string) map[string]RuleRef {
	out := map[string]RuleRef{}
	current, err := s.loadCurrent()
	if err != nil || current == nil || len(ids) == 0 {
		return out
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, r := range current.Rules.rules {
		observations, _ := r["observations"].([]any)
		for _, o := range observations {
			id := textField(record(o)["id"])
			if wanted[id] {
				out[id] = RuleRef{ID: textField(r["id"]), Directive: textField(r["directive"])}
			}
		}
	}
	return out
}

// RuleVerdictCounts counts reviewer verdicts per rule from the activity log
// (the log keeps 30 days).
func (s *Store) RuleVerdictCounts() map[string]VerdictCount {
	out := map[string]VerdictCount{}
	events, _ := s.readActivity()
	for _, e := range events {
		c := out[e.RuleID]
		switch e.Kind {
		case activityRuleConfirmed:
			c.Confirmed++
		case activityRuleDismissed:
			c.Dismissed++
		default:
			continue
		}
		out[e.RuleID] = c
	}
	return out
}
