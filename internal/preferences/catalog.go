package preferences

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

type catalog struct {
	store *Store
	rules []rule
}

func (s *Store) catalogFromGeneration(current *generation) (*catalog, error) {
	rules := []rule{}
	if current != nil {
		if current.Rules.issuer != s {
			return nil, fmt.Errorf("generation was not validated by this store")
		}
		var err error
		rules, err = normalizeRules(current.Rules.rules, s.config.Clock())
		if err != nil {
			return nil, err
		}
	}
	return &catalog{store: s, rules: rules}, nil
}

func (c *catalog) ruleSet() validatedRuleSet {
	return validatedRuleSet{issuer: c.store, rules: cloneRules(c.rules)}
}

func ruleID(directive string, scope map[string]any, surfaces []string) string {
	material := []string{strings.ToLower(directive), fmt.Sprint(scope["kind"]), ""}
	if repo, ok := scope["repo_key"].(string); ok {
		material[2] = repo
	}
	material = append(material, surfaces...)
	return "rule_" + digest(strings.Join(material, "\x00"))
}

func newRule(directive string, scope map[string]any, surfaces []string, origin string, now time.Time) rule {
	stamp := now.UTC().Format(time.RFC3339)
	return rule{"id": ruleID(directive, scope, surfaces), "directive": directive, "scope": cloneMap(scope), "surfaces": surfaces, "origin": origin, "status": "candidate", "observations": []any{}, "evidence": map[string]any{}, "examples": []string{}, "created_at": stamp, "updated_at": stamp}
}

func (c *catalog) findRule(id string) (rule, error) {
	for _, r := range c.rules {
		if r["id"] == id {
			return rule(cloneMap(r)), nil
		}
	}
	return nil, fmt.Errorf("unknown rule")
}

func (c *catalog) replaceRule(next rule) (rule, error) {
	rules := cloneRules(c.rules)
	found := false
	for i, r := range rules {
		if r["id"] == next["id"] {
			rules[i] = rule(cloneMap(next))
			found = true
			break
		}
	}
	if !found {
		rules = append(rules, rule(cloneMap(next)))
	}
	if err := validateRuleGraph(rules); err != nil {
		return nil, err
	}
	c.rules = rules
	return rule(cloneMap(next)), nil
}

func (c *catalog) upsertExplicit(observation map[string]any, directive string, scope map[string]any, surfaces []string) (rule, error) {
	directive = normalize(directive)
	privacy := sanitize(directive)
	if !privacy.Accepted {
		return nil, fmt.Errorf("directive contains sensitive content")
	}
	if err := validateScope(scope); err != nil {
		return nil, err
	}
	surfaces = sortedUnique(surfaces)
	if err := validateSurfaces(surfaces); err != nil {
		return nil, err
	}
	id := ruleID(directive, scope, surfaces)
	next, err := c.findRule(id)
	if err != nil {
		next = newRule(directive, scope, surfaces, "feedback", c.store.config.Clock())
	}
	next["origin"] = "feedback"
	delete(next, "load_tier")
	if err := addObservation(next, observation, true, c.store.config.Clock()); err != nil {
		return nil, err
	}
	if err := recomputeRule(next, c.store.config.Clock()); err != nil {
		return nil, err
	}
	return c.replaceRule(next)
}

func addObservation(r rule, input map[string]any, explicit bool, now time.Time) error {
	id, ok := input["id"].(string)
	if !ok || id == "" {
		return invalidRecord("observation needs id")
	}
	captured, ok := input["captured_at"]
	if !ok {
		captured = now.UTC().Format(time.RFC3339)
	}
	if err := validateTimestamp(captured); err != nil {
		return err
	}
	var excerpt any
	if value := input["safe_excerpt"]; value != nil {
		text, ok := value.(string)
		if !ok {
			return invalidRecord("observation excerpt must be text")
		}
		privacy := sanitize(text)
		if privacy.Accepted {
			excerpt = privacy.Excerpt
		}
	}
	reference := map[string]any{"id": id, "captured_at": captured, "session_key": input["session_key"], "repo_key": input["repo_key"], "explicit": explicit, "safe_excerpt": excerpt}
	if context := contextFromMap(input["context"]); context != nil {
		reference["context"] = context
	}
	if err := validateObservationReference(reference); err != nil {
		return err
	}
	existing, err := mapArray(r["observations"])
	if err != nil {
		return err
	}
	updated := []any{}
	for _, o := range existing {
		if o["id"] == id {
			if o["explicit"] == true {
				reference["explicit"] = true
			}
			continue
		}
		updated = append(updated, o)
	}
	updated = append(updated, reference)
	r["observations"] = updated
	r["updated_at"] = now.UTC().Format(time.RFC3339)
	return nil
}

func contextFromMap(value any) *NeighboringContext {
	if context, ok := value.(*NeighboringContext); ok {
		if context == nil {
			return nil
		}
		return sanitizeContext(*context)
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	source, _ := fields["source"].(string)
	previous, _ := fields["previous_user_request"].(string)
	agent, _ := fields["agent_response"].(string)
	return sanitizeContext(NeighboringContext{Source: source, PreviousUserRequest: previous, AgentResponse: agent})
}

func (c *catalog) confirm(id string, observation map[string]any) (rule, error) {
	next, err := c.findRule(id)
	if err != nil {
		return nil, err
	}
	if err := addObservation(next, observation, false, c.store.config.Clock()); err != nil {
		return nil, err
	}
	if err := recomputeRule(next, c.store.config.Clock()); err != nil {
		return nil, err
	}
	return c.replaceRule(next)
}

func (c *catalog) retire(id string) (rule, error) {
	next, err := c.findRule(id)
	if err != nil {
		return nil, err
	}
	if next["origin"] == "source_pack" {
		return nil, fmt.Errorf("source-pack rules cannot be retired directly")
	}
	if next["status"] == "superseded" {
		return next, nil
	}
	stamp := c.store.config.Clock().UTC().Format(time.RFC3339)
	next["retired_at"] = stamp
	next["updated_at"] = stamp
	if err := recomputeRule(next, c.store.config.Clock()); err != nil {
		return nil, err
	}
	return c.replaceRule(next)
}

func (c *catalog) orderedActive() []rule {
	active := []rule{}
	for _, r := range c.rules {
		if r["status"] == "active" || r["status"] == "core" {
			active = append(active, rule(cloneMap(r)))
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return slices.Compare(rulePrecedence(active[i]), rulePrecedence(active[j])) < 0 })
	return active
}

func rulePrecedence(r rule) []string {
	scope, _ := r["scope"].(map[string]any)
	kind := "1"
	if scope["kind"] == "repository" {
		kind = "0"
	}
	origin := "2"
	if r["origin"] == "feedback" || r["origin"] == "inferred" {
		origin = "0"
	} else if r["origin"] == "source_pack" {
		origin = "1"
	}
	tier := "1"
	if r["load_tier"] == "core" {
		tier = "0"
	}
	return []string{kind, origin, tier, fmt.Sprint(r["directive"]), fmt.Sprint(r["id"])}
}
