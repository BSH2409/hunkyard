package preferences

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type Selection struct {
	Text            string   `json:"text"`
	RuleIDs         []string `json:"rule_ids"`
	OmittedCount    int      `json:"omitted_count"`
	EstimatedTokens int      `json:"estimated_tokens"`
	// Reasons maps each selected rule to why it was loaded (hook paths only).
	Reasons map[string]string `json:"-"`
}

const precedenceSentence = "Current user instructions and repository standards win. Repository preferences override global preferences; personal preferences override source-pack defaults."

var selectionGroups = []string{"Repository surface preferences", "Repository core preferences", "Global surface preferences", "Global core preferences", "Source-pack surface defaults", "Source-pack core defaults"}
var omissionOrder = []int{4, 5, 2, 3, 0, 1}
var personaSurfacePatterns = map[string]*regexp.Regexp{
	"api":           regexp.MustCompile(`(?i)\b(?:apis?|controllers?|endpoints?|serializers?|request contracts?|response shapes?)\b`),
	"architecture":  regexp.MustCompile(`(?i)\b(?:architecture|architectural|mcp|tool contracts?)\b`),
	"communication": regexp.MustCompile(`(?i)\b(?:communication|documentation|docs?|comments?|commit messages?|pull request descriptions?)\b`),
	"database":      regexp.MustCompile(`(?i)\b(?:database|migrations?|query|queries|sql|schema|index|indexes)\b`),
	"devops":        regexp.MustCompile(`(?i)\b(?:devops|deployments?|pipelines?|ci/cd|continuous integration|docker|containers?|kubernetes|terraform|infrastructure)\b`),
	"frontend":      regexp.MustCompile(`(?i)\b(?:front[- ]?end|ui|ux|react|javascript|typescript|css|components?|horizon)\b`),
	"jobs":          regexp.MustCompile(`(?i)\b(?:background jobs?|jobs?|retry|retries|sidekiq|workers?)\b`),
	"naming":        regexp.MustCompile(`(?i)\b(?:naming|rename|names?|identifiers?)\b`),
	"performance":   regexp.MustCompile(`(?i)\b(?:performance|optimi[sz]e|latency|memory|n\+1|throughput|profiling)\b`),
	"rails":         regexp.MustCompile(`(?i)(?:\.rb\b|\brails\b|\bactive\s*record\b)`),
	"reviews":       regexp.MustCompile(`(?i)\b(?:review|reviews|pull requests?|prs?|diffs?|findings?)\b`),
	"ruby":          regexp.MustCompile(`(?i)(?:\.rb\b|\bruby\b|\brails\b)`),
	"security":      regexp.MustCompile(`(?i)\b(?:security|secure|authentication|authorization|permissions?|secrets?|credentials?|vulnerabilit(?:y|ies)|csrf|xss)\b`),
	"services":      regexp.MustCompile(`(?i)\b(?:service|services|service object|mcp|tool contracts?)\b`),
	"testing":       regexp.MustCompile(`(?i)\b(?:test|tests|testing|spec|specs|rspec|minitest|retryable failures?)\b`),
	"wordpress":     regexp.MustCompile(`(?i)\b(?:word[- ]?press|wp-cli|wp cli)\b`),
}

func routedSurfaces(prompt string) []string {
	selected := []string{"naming"}
	for surface, pattern := range personaSurfacePatterns {
		if pattern.MatchString(prompt) {
			selected = append(selected, surface)
		}
	}
	return sortedUnique(selected)
}
func selectionGroup(r rule) int {
	core := isCoreRule(r)
	if r["origin"] == "source_pack" {
		if core {
			return 5
		}
		return 4
	}
	if record(r["scope"])["kind"] == "repository" {
		if core {
			return 1
		}
		return 0
	}
	if core {
		return 3
	}
	return 2
}
func selectionText(rules []rule) string {
	sections := []string{precedenceSentence}
	for group, title := range selectionGroups {
		var body strings.Builder
		for _, r := range rules {
			if selectionGroup(r) == group {
				body.WriteString(personaBullet(r))
			}
		}
		if body.Len() > 0 {
			sections = append(sections, title+":\n"+body.String())
		}
	}
	return strings.Join(sections, "\n\n") + "\n"
}
func overlapCount(left, right []string) int {
	count := 0
	for _, value := range sortedUnique(left) {
		if slices.Contains(right, value) {
			count++
		}
	}
	return count
}
func removalRank(r rule, surfaces []string) []int {
	evidence := record(r["evidence"])
	explicit, _ := integer(evidence["explicit_count"])
	sessions, _ := integer(evidence["session_count"])
	observations, _ := integer(evidence["observation_count"])
	naming, implicit := 1, 1
	if slices.Contains(stringValues(r["surfaces"]), "naming") {
		naming = 0
	}
	if explicit > 0 {
		implicit = 0
	}
	return []int{naming, -overlapCount(stringValues(r["surfaces"]), surfaces), implicit, -sessions, -observations}
}

//nolint:gocyclo // Selection preserves the exact six-group precedence and omission order.
func selectPersona(rules []rule, prompt, repoKey string) Selection {
	surfaces := routedSurfaces(prompt)
	active, relevant := []rule{}, []rule{}
	personalIDs, personalDirectives, blocked := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range rules {
		scope := record(r["scope"])
		if scope["kind"] != "global" && (scope["kind"] != "repository" || scope["repo_key"] != repoKey) {
			continue
		}
		if r["status"] != "active" && r["status"] != "core" {
			continue
		}
		active = append(active, r)
		if isCoreRule(r) || overlapCount(stringValues(r["surfaces"]), surfaces) > 0 {
			relevant = append(relevant, r)
			if r["origin"] != "source_pack" {
				personalDirectives[directiveKey(r)] = true
			}
		}
		if r["origin"] != "source_pack" {
			personalIDs[textField(r["id"])] = true
			for _, field := range []string{"conflicts_with", "supersedes"} {
				for _, id := range stringValues(r[field]) {
					blocked[id] = true
				}
			}
		}
	}
	for _, r := range active {
		if r["origin"] != "source_pack" {
			continue
		}
		id := textField(r["id"])
		if personalDirectives[directiveKey(r)] {
			blocked[id] = true
		}
		refs := append(append([]string{}, stringValues(r["conflicts_with"])...), stringValues(r["supersedes"])...)
		if successor := textField(r["superseded_by"]); successor != "" {
			refs = append(refs, successor)
		}
		for _, ref := range refs {
			if personalIDs[ref] {
				blocked[id] = true
			}
		}
	}
	selected := []rule{}
	for _, r := range relevant {
		if r["origin"] != "source_pack" || !blocked[textField(r["id"])] {
			selected = append(selected, r)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		if x, y := selectionGroup(a), selectionGroup(b); x != y {
			return x < y
		}
		if a["directive"] != b["directive"] {
			return textField(a["directive"]) < textField(b["directive"])
		}
		return textField(a["id"]) < textField(b["id"])
	})
	omitted := 0
	for estimatedTokens(selectionText(selected)) > 1500 && len(selected) > 0 {
		removable := []int{}
		for i, r := range selected {
			if r["origin"] == "source_pack" || !slices.Contains(stringValues(r["surfaces"]), "naming") {
				removable = append(removable, i)
			}
		}
		if len(removable) == 0 {
			for i := range selected {
				removable = append(removable, i)
			}
		}
		group := -1
		for _, candidate := range omissionOrder {
			for _, i := range removable {
				if selectionGroup(selected[i]) == candidate {
					group = candidate
					break
				}
			}
			if group >= 0 {
				break
			}
		}
		worst := -1
		for _, i := range removable {
			if selectionGroup(selected[i]) != group {
				continue
			}
			if worst < 0 {
				worst = i
				continue
			}
			comparison := slices.Compare(removalRank(selected[i], surfaces), removalRank(selected[worst], surfaces))
			if comparison > 0 || comparison == 0 && textField(selected[i]["id"]) > textField(selected[worst]["id"]) {
				worst = i
			}
		}
		selected = append(selected[:worst], selected[worst+1:]...)
		omitted++
	}
	text := selectionText(selected)
	ids := []string{}
	for _, r := range selected {
		ids = append(ids, textField(r["id"]))
	}
	return Selection{Text: text, RuleIDs: ids, OmittedCount: omitted, EstimatedTokens: estimatedTokens(text)}
}

func (s *Store) SelectPersona(prompt, cwd, repoKey string) (Selection, error) {
	current, err := s.loadCurrent()
	if err != nil {
		return Selection{}, err
	}
	if repoKey == "" {
		repoKey, err = repositoryKey(cwd)
		if err != nil {
			return Selection{}, err
		}
	} else if !repositoryKeyPattern.MatchString(repoKey) {
		return Selection{}, fmt.Errorf("invalid preference repository identity")
	}
	rules := []rule{}
	if current != nil {
		rules = current.Rules.rules
	}
	return selectPersona(rules, prompt, repoKey), nil
}

// selectCore picks the always-on rules loaded once per session: in-scope
// rules marked core, grouped like the per-prompt persona.
func selectCore(rules []rule, repoKey string) Selection {
	selected := []rule{}
	for _, r := range rules {
		scope := record(r["scope"])
		if scope["kind"] != "global" && (scope["kind"] != "repository" || scope["repo_key"] != repoKey) {
			continue
		}
		if isCoreRule(r) {
			selected = append(selected, r)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if x, y := selectionGroup(selected[i]), selectionGroup(selected[j]); x != y {
			return x < y
		}
		return textField(selected[i]["directive"]) < textField(selected[j]["directive"])
	})
	ids := []string{}
	for _, r := range selected {
		ids = append(ids, textField(r["id"]))
	}
	if len(selected) == 0 {
		return Selection{RuleIDs: ids}
	}
	text := selectionText(selected)
	return Selection{Text: text, RuleIDs: ids, EstimatedTokens: estimatedTokens(text)}
}
