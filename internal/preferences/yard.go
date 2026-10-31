package preferences

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// YardRule is a rule as the Preference Yard lists it.
type YardRule struct {
	ID           string   `json:"id"`
	Directive    string   `json:"directive"`
	Status       string   `json:"status"`
	Origin       string   `json:"origin"`
	ScopeKind    string   `json:"scope_kind"`
	RepoKey      string   `json:"repo_key,omitempty"`
	Surfaces     []string `json:"surfaces"`
	Observations int      `json:"observations"`
	Violations   int      `json:"violations"`
	UpdatedAt    string   `json:"updated_at"`
}

// YardRules lists every rule of the current generation with its evidence
// and violation counts (violations and guard blocks from the activity log).
func (s *Store) YardRules() ([]YardRule, error) {
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return []YardRule{}, err
	}
	violations := map[string]int{}
	events, _ := s.readActivity()
	for _, e := range events {
		if e.Kind == activityViolated || e.Kind == activityBlocked {
			violations[e.RuleID]++
		}
	}
	rules := []YardRule{}
	for _, r := range current.Rules.rules {
		scope := record(r["scope"])
		observations, _ := r["observations"].([]any)
		id := textField(r["id"])
		rules = append(rules, YardRule{ID: id, Directive: textField(r["directive"]), Status: textField(r["status"]), Origin: textField(r["origin"]), ScopeKind: textField(scope["kind"]), RepoKey: textField(scope["repo_key"]), Surfaces: stringValues(r["surfaces"]), Observations: len(observations), Violations: violations[id], UpdatedAt: textField(r["updated_at"])})
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].UpdatedAt > rules[j].UpdatedAt })
	return rules, nil
}

// YardPersona returns the generated persona files agents receive, keyed by
// path (core.md, naming.md, repositories/<key>/…).
func (s *Store) YardPersona() (map[string]string, error) {
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return map[string]string{}, err
	}
	return current.PersonaFiles, nil
}

const yardCapturedLimit = 20

// YardPipeline summarises where observations and rules stand: recent
// captured observations still waiting for maintenance, and rule counts.
func (s *Store) YardPipeline() (map[string]any, error) {
	current, err := s.loadCurrent()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{"candidate": 0, "active": 0, "retired": 0}
	if current != nil {
		for _, r := range current.Rules.rules {
			switch r["status"] {
			case "candidate":
				counts["candidate"]++
			case "active", "core":
				counts["active"]++
			default:
				counts["retired"]++
			}
		}
	}
	processed := map[string]bool{}
	if current != nil {
		for _, id := range current.ProcessedIDs {
			processed[id] = true
		}
	}
	captured := []map[string]any{}
	entries, _ := s.pendingEntries()
	for i := len(entries) - 1; i >= 0 && len(captured) < yardCapturedLimit; i-- {
		o := entries[i]
		if processed[o.ID] {
			continue
		}
		excerpt, repo := "", ""
		if o.SafeExcerpt != nil {
			excerpt = *o.SafeExcerpt
		}
		if o.RepoKey != nil {
			repo = *o.RepoKey
		}
		captured = append(captured, map[string]any{"id": o.ID, "source": o.Source, "repo_key": repo, "excerpt": excerpt, "captured_at": o.CapturedAt})
	}
	return map[string]any{"captured": captured, "candidate": counts["candidate"], "active": counts["active"], "retired": counts["retired"]}, nil
}

// YardHealth combines doctor checks, store status, the last maintenance run
// and guard example results.
func (s *Store) YardHealth() map[string]any {
	health := s.doctor()
	if status, err := s.status(); err == nil {
		health["store"] = status
	}
	health["maintenance_run"] = s.lastMaintenanceRun()
	health["guards"] = s.testGuards()
	return health
}

// YardRetire stops applying a rule; its evidence is kept.
func (s *Store) YardRetire(id string) error {
	_, err := s.retire(id)
	return err
}

// YardPromote turns a repository rule into a global one by capturing the
// same directive explicitly at global scope.
func (s *Store) YardPromote(id string) error {
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return fmt.Errorf("no preferences yet")
	}
	for _, r := range current.Rules.rules {
		if r["id"] == id {
			_, err := s.YardCreateRule(textField(r["directive"]), "global", "", stringValues(r["surfaces"]), "Promoted to global in the Preference Yard")
			return err
		}
	}
	return fmt.Errorf("unknown rule")
}

// YardCreateRule records an explicit rule the user created in the Yard, for
// example from a review comment that was ignored as too vague.
func (s *Store) YardCreateRule(directive, scope, repoKey string, surfaces []string, feedback string) (CaptureResult, error) {
	return s.Capture(CaptureInput{Feedback: feedback, Directive: directive, Scope: scope, RepoKey: repoKey, Surfaces: surfaces, Explicit: true, Source: "cli", SessionID: "yard", TurnID: randomID()})
}

// YardRunMaintenance starts automatic maintenance in the background.
func (s *Store) YardRunMaintenance() (map[string]any, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if err := ensureDirectory(s.dataPath("logs")); err != nil {
		return nil, err
	}
	if err := startDetached([]string{binary, "preferences", "maintain", "--auto", "--force", "--json", "--root", s.config.Root}, s.dataPath("logs", "auto-maintenance.log")); err != nil {
		return nil, err
	}
	return map[string]any{"status": "started"}, nil
}

// pendingEntries reads the pending observation queue without locking; the
// Yard only displays it.
func (s *Store) pendingEntries() ([]observation, error) {
	data, err := regularFile(s.dataPath("observations", "pending.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []observation
	for _, line := range bytes.Split(data, []byte("\n")) {
		var o observation
		if json.Unmarshal(line, &o) == nil && o.ID != "" {
			entries = append(entries, o)
		}
	}
	return entries, nil
}

// YardLoad is one host session's injected rules and why each was loaded.
type YardLoad struct {
	Session   string           `json:"session"`
	UpdatedAt string           `json:"updated_at"`
	Rules     []YardLoadedRule `json:"rules"`
}

// YardLoadedRule is a rule a session received and the reason it was loaded.
type YardLoadedRule struct {
	ID        string `json:"id"`
	Directive string `json:"directive"`
	Reason    string `json:"reason"`
}

const yardLoadsLimit = 20

// YardLoads lists recent sessions that received rules, newest first.
func (s *Store) YardLoads() ([]YardLoad, error) {
	loads := []YardLoad{}
	entries, err := os.ReadDir(s.dataPath("sessions"))
	if os.IsNotExist(err) {
		return loads, nil
	}
	if err != nil {
		return loads, err
	}
	directives := map[string]string{}
	if current, err := s.loadCurrent(); err == nil && current != nil {
		for _, r := range current.Rules.rules {
			directives[textField(r["id"])] = textField(r["directive"])
		}
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		var state sessionState
		data, err := regularFile(s.dataPath("sessions", entry.Name()))
		if err != nil || json.Unmarshal(data, &state) != nil || len(state.InjectedRuleIDs) == 0 {
			continue
		}
		load := YardLoad{Session: name, UpdatedAt: state.UpdatedAt, Rules: []YardLoadedRule{}}
		for _, id := range state.InjectedRuleIDs {
			load.Rules = append(load.Rules, YardLoadedRule{ID: id, Directive: directives[id], Reason: state.LoadReasons[id]})
		}
		loads = append(loads, load)
	}
	sort.SliceStable(loads, func(i, j int) bool { return loads[i].UpdatedAt > loads[j].UpdatedAt })
	if len(loads) > yardLoadsLimit {
		loads = loads[:yardLoadsLimit]
	}
	return loads, nil
}
