package preferences

import (
	"slices"
	"time"
)

func applyProposal(proposal validatedProposal, batch maintenanceBatch) (proposalApplication, error) {
	if batch.catalog == nil || batch.catalog.store == nil || proposal.catalog != batch.catalog || proposal.digest != batch.digest {
		return proposalApplication{}, proposalError("application needs the original authorized catalog")
	}
	encoded, err := canonicalJSON(proposal.document)
	if err != nil {
		return proposalApplication{}, err
	}
	checked, err := validateProposal(encoded, batch)
	if err != nil {
		return proposalApplication{}, err
	}
	actions, _ := mapArray(checked.document["actions"])
	var packs map[string]any
	if batch.source != nil {
		if err := batch.source.revalidate(); err != nil {
			return proposalApplication{}, err
		}
		packs = cloneMap(batch.source.metadata)
		files := record(record(packs["code-like-him"])["files"])
		for _, action := range actions {
			if action["type"] == "refresh_source_pack" {
				citation := record(action["source_ref"])
				if files[textField(citation["file"])] != citation["digest"] {
					return proposalApplication{}, proposalError("source metadata changed during application")
				}
			}
		}
	}
	working := &catalog{store: batch.catalog.store, rules: cloneRules(batch.catalog.rules)}
	if err := working.applyActions(actions, batch.observations); err != nil {
		return proposalApplication{}, err
	}
	ids := []string{}
	for _, action := range actions {
		ids = append(ids, stringValues(action["observation_ids"])...)
	}
	return proposalApplication{rules: working.ruleSet(), processedIDs: sortedUnique(ids), sourcePacks: packs, counts: cloneMap(record(checked.document["summary"]))}, nil
}

func (c *catalog) mutableRule(id string) (rule, error) {
	for _, r := range c.rules {
		if r["id"] == id {
			return r, nil
		}
	}
	return nil, proposalError("unknown rule identity")
}

//nolint:gocyclo // All actions are checked and applied to one defensive catalog copy.
func (c *catalog) applyActions(actions []map[string]any, observations []map[string]any) error {
	if c == nil || c.store == nil {
		return proposalError("missing catalog authority")
	}
	index := map[string]map[string]any{}
	for _, o := range observations {
		index[textField(o["id"])] = o
	}
	for _, action := range actions {
		kind := textField(action["type"])
		id := textField(action["rule_id"])
		switch kind {
		case "confirm_rule", "mark_dormant", "reactivate_rule":
			r, err := c.mutableRule(id)
			if err != nil {
				return err
			}
			switch kind {
			case "confirm_rule":
				if r["origin"] == "source_pack" || !slices.Contains([]string{"candidate", "active", "core"}, textField(r["status"])) {
					return proposalError("cannot confirm an inactive or source rule")
				}
			case "mark_dormant":
				if r["status"] != "candidate" || !dormancyEligible(r, c.store.config.Clock()) {
					return proposalError("dormancy needs an old candidate")
				}
			case "reactivate_rule":
				if r["status"] != "dormant" {
					return proposalError("only a dormant rule may reactivate")
				}
				previous, _ := mapArray(r["observations"])
				known := map[string]bool{}
				for _, o := range previous {
					known[textField(o["id"])] = true
				}
				fresh := []map[string]any{}
				for _, id := range stringValues(action["observation_ids"]) {
					if !known[id] {
						if index[id] == nil {
							return proposalError("unknown reactivation evidence")
						}
						fresh = append(fresh, index[id])
					}
				}
				if len(fresh) == 0 {
					return proposalError("reactivation needs new evidence")
				}
				_, explicit := repositoriesAndExplicit(fresh)
				reason := textField(action["reason_code"])
				if reason == "explicit_instruction" && !explicit || !slices.Contains([]string{"explicit_instruction", "confirming_evidence", "durable_preference"}, reason) {
					return proposalError("invalid reactivation evidence or reason")
				}
			}
		}
	}
	working := &catalog{store: c.store, rules: cloneRules(c.rules)}
	for _, action := range actions {
		if err := working.applyAction(action, index); err != nil {
			return err
		}
	}
	for _, r := range working.rules {
		if err := recomputeRule(r, c.store.config.Clock()); err != nil {
			return err
		}
	}

	if err := validateRuleGraph(working.rules); err != nil {
		return err
	}
	c.rules = working.rules
	return nil
}

func actionKind(action map[string]any) string {
	if kind := textField(action["action"]); kind != "" {
		return kind
	}
	return textField(action["type"])
}
func actionIDs(action map[string]any) []string {
	if ids, ok := action["observation_ids"]; ok {
		return stringValues(ids)
	}
	if id := textField(action["observation_id"]); id != "" {
		return []string{id}
	}
	return []string{}
}
func (c *catalog) addActionObservations(r rule, action map[string]any, index map[string]map[string]any, allowEmpty, fromSource bool) error {
	ids := actionIDs(action)
	if len(ids) == 0 && !allowEmpty {
		return proposalError("action needs observations")
	}
	for _, id := range ids {
		o := index[id]
		if o == nil {
			return proposalError("unknown observation")
		}
		if err := addObservation(r, o, fromSource && explicitObservation(o), c.store.config.Clock()); err != nil {
			return err
		}
	}
	return nil
}
func validateActionRule(action map[string]any, origin string) (map[string]any, error) {
	proposed := record(action["proposed_rule"])
	if proposed == nil || proposed["id"] != action["rule_id"] || proposed["origin"] != origin {
		return nil, proposalError("action changes stable identity or provenance")
	}
	if err := validateProposedRule(proposed); err != nil {
		return nil, err
	}
	if err := validateProposedDirective(textField(proposed["directive"])); err != nil {
		return nil, err
	}
	if origin == "source_pack" {
		if !slices.Contains([]string{"core", "surface"}, textField(proposed["load_tier"])) || textField(proposed["source_pack"]) == "" || record(proposed["source_ref"]) == nil {
			return nil, proposalError("source rule needs complete provenance")
		}
	} else if proposed["load_tier"] != nil || proposed["source_pack"] != nil || proposed["source_ref"] != nil {
		return nil, proposalError("personal rule cannot claim source provenance")
	}
	return proposed, nil
}
func updateFromProposal(r rule, proposed map[string]any, now time.Time) {
	r["directive"] = proposed["directive"]
	r["scope"] = cloneMap(record(proposed["scope"]))
	r["surfaces"] = sortedUnique(stringValues(proposed["surfaces"]))
	r["origin"] = proposed["origin"]
	for _, field := range []string{"load_tier", "source_pack", "source_ref"} {
		if proposed["origin"] == "source_pack" {
			r[field] = cloneValue(proposed[field])
		} else {
			delete(r, field)
		}
	}
	r["updated_at"] = now.UTC().Format(time.RFC3339)
}
func cloneValue(value any) any { return cloneMap(map[string]any{"value": value})["value"] }
func (c *catalog) unlinkConflicts(r rule) {
	id := textField(r["id"])
	for _, other := range stringValues(r["conflicts_with"]) {
		if target, err := c.mutableRule(other); err == nil {
			refs := []string{}
			for _, ref := range stringValues(target["conflicts_with"]) {
				if ref != id {
					refs = append(refs, ref)
				}
			}
			target["conflicts_with"] = refs
		}
	}
	r["conflicts_with"] = []string{}
}
func (c *catalog) upsertAction(proposed map[string]any, action map[string]any, index map[string]map[string]any) error {
	if proposed == nil {
		return proposalError("missing upsert rule")
	}
	id := textField(proposed["id"])
	r, err := c.mutableRule(id)
	if err != nil {
		origin := "inferred"
		if proposed["origin"] == "source_pack" {
			origin = "source_pack"
		}
		r = newRule(textField(proposed["directive"]), record(proposed["scope"]), sortedUnique(stringValues(proposed["surfaces"])), origin, c.store.config.Clock())
		r["id"] = id
		for _, field := range []string{"load_tier", "source_pack", "source_ref"} {
			if value, exists := proposed[field]; exists {
				r[field] = cloneValue(value)
			}
		}
		c.rules = append(c.rules, r)
	}
	if err := c.addActionObservations(r, action, index, true, false); err != nil {
		return err
	}
	return recomputeRule(r, c.store.config.Clock())
}
func (c *catalog) createProposed(action map[string]any, index map[string]map[string]any) error {
	proposed, err := validateActionRule(action, "inferred")
	if err != nil {
		return err
	}
	if _, err := c.mutableRule(textField(proposed["id"])); err == nil {
		return proposalError("new rule identity already exists")
	}
	personal := cloneMap(proposed)
	for field, value := range personal {
		if value == nil {
			delete(personal, field)
		}
	}
	return c.upsertAction(personal, action, index)
}

//nolint:gocyclo // All actions are checked and applied to one defensive catalog copy.
func (c *catalog) applyAction(action map[string]any, index map[string]map[string]any) error {
	kind, id := actionKind(action), textField(action["rule_id"])
	now := c.store.config.Clock()
	stamp := now.UTC().Format(time.RFC3339)
	switch kind {
	case "ignore_observation":
		for _, id := range actionIDs(action) {
			if index[id] == nil {
				return proposalError("unknown ignored observation")
			}
		}
	case "create_rule":
		return c.createProposed(action, index)
	case "upsert", "create":
		return c.upsertAction(record(action["rule"]), action, index)
	case "confirm_rule", "confirm", "reactivate_rule":
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		if kind == "confirm_rule" && action["proposed_rule"] != nil {
			return proposalError("confirmation cannot rewrite a rule")
		}
		return c.addActionObservations(r, action, index, false, kind != "confirm")
	case "refine_rule", "change_scope":
		targets := stringValues(action["target_rule_ids"])
		if len(targets) != 1 || targets[0] != id {
			return proposalError("rewrite needs one matching target")
		}
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		proposed, err := validateActionRule(action, textField(r["origin"]))
		if err != nil {
			return err
		}
		updateFromProposal(r, proposed, now)
		return c.addActionObservations(r, action, index, true, true)
	case "merge_rules":
		targets := stringValues(action["target_rule_ids"])
		if len(targets) < 2 || !slices.Contains(targets, id) {
			return proposalError("merge must retain a target")
		}
		rules := []rule{}
		for _, target := range targets {
			r, err := c.mutableRule(target)
			if err != nil {
				return err
			}
			rules = append(rules, r)
		}
		retained, _ := c.mutableRule(id)
		proposed, err := validateActionRule(action, mergedOrigin(rules))
		if err != nil {
			return err
		}
		updateFromProposal(retained, proposed, now)
		predecessors := stringValues(proposed["supersedes"])
		for _, r := range rules {
			if r["id"] == id {
				continue
			}
			observations, _ := mapArray(r["observations"])
			for _, o := range observations {
				if err := addObservation(retained, o, o["explicit"] == true, now); err != nil {
					return err
				}
			}
			c.unlinkConflicts(r)
			r["superseded_by"] = id
			r["updated_at"] = stamp
			predecessors = append(predecessors, textField(r["id"]))
		}
		retained["supersedes"] = sortedUnique(predecessors)
		return c.addActionObservations(retained, action, index, true, true)
	case "supersede_rule":
		targets := stringValues(action["target_rule_ids"])
		if len(targets) != 1 || id == "" || targets[0] == id {
			return proposalError("invalid supersession targets")
		}
		predecessor, err := c.mutableRule(targets[0])
		if err != nil {
			return err
		}
		successor, successorErr := c.mutableRule(id)
		if successorErr == nil && successor["origin"] == "source_pack" {
			return proposalError("cannot rewrite source successor")
		}
		if action["proposed_rule"] != nil {
			if successorErr == nil {
				proposed, err := validateActionRule(action, textField(successor["origin"]))
				if err != nil {
					return err
				}
				updateFromProposal(successor, proposed, now)
				if err := c.addActionObservations(successor, action, index, true, true); err != nil {
					return err
				}
			} else {
				if err := c.createProposed(action, index); err != nil {
					return err
				}
				successor, _ = c.mutableRule(id)
			}
		} else if successorErr != nil || len(actionIDs(action)) > 0 {
			return proposalError("existing successor must be known and consume no evidence")
		}
		predecessor["superseded_by"] = id
		predecessor["updated_at"] = stamp
		successor["supersedes"] = sortedUnique(append(stringValues(successor["supersedes"]), targets[0]))
	case "quarantine_conflict":
		targets := stringValues(action["target_rule_ids"])
		if len(targets) < 2 {
			return proposalError("conflict needs two targets")
		}
		if _, err := c.mutableRule(id); action["proposed_rule"] != nil && err != nil {
			if err := c.createProposed(action, index); err != nil {
				return err
			}
		} else if len(actionIDs(action)) > 0 {
			return proposalError("existing conflict cannot consume observations")
		}
		for _, left := range targets {
			a, err := c.mutableRule(left)
			if err != nil {
				return err
			}
			for _, right := range targets {
				if left == right {
					continue
				}
				if _, err := c.mutableRule(right); err != nil {
					return err
				}
				a["conflicts_with"] = sortedUnique(append(stringValues(a["conflicts_with"]), right))
				a["updated_at"] = stamp
			}
		}
	case "mark_dormant":
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		if len(actionIDs(action)) > 0 || action["proposed_rule"] != nil || !slices.Contains([]string{"candidate", "dormant"}, textField(r["status"])) || !dormancyEligible(r, now) {
			return proposalError("dormancy needs an old candidate")
		}
	case "refresh_source_pack":
		if len(actionIDs(action)) > 0 {
			return proposalError("source refresh cannot consume evidence")
		}
		if action["proposed_rule"] == nil {
			return nil
		}
		proposed, err := validateActionRule(action, "source_pack")
		if err != nil {
			return err
		}
		refreshed, refreshedErr := c.mutableRule(id)
		if refreshedErr == nil {
			if refreshed["origin"] != "source_pack" {
				return proposalError("source refresh cannot replace personal provenance")
			}
			for _, field := range []string{"directive", "scope", "surfaces", "load_tier", "source_pack", "source_ref"} {
				refreshed[field] = cloneValue(proposed[field])
			}
		} else {
			source := cloneMap(proposed)
			for field, value := range source {
				if value == nil {
					delete(source, field)
				}
			}
			if err := c.upsertAction(source, map[string]any{"observation_ids": []string{}}, index); err != nil {
				return err
			}
			refreshed, _ = c.mutableRule(id)
		}
		if refs := stringValues(proposed["supersedes"]); len(refs) > 0 {
			refreshed["supersedes"] = sortedUnique(refs)
		}
		for _, target := range stringValues(refreshed["supersedes"]) {
			predecessor, err := c.mutableRule(target)
			if err != nil || predecessor["origin"] != "source_pack" {
				return proposalError("source refresh can supersede only source rules")
			}
			predecessor["superseded_by"] = id
			predecessor["updated_at"] = stamp
		}
	case "retire":
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		r["retired_at"] = stamp
	case "supersede":
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		successor, err := c.mutableRule(textField(action["superseded_by"]))
		if err != nil {
			return err
		}
		r["superseded_by"] = successor["id"]
		successor["supersedes"] = sortedUnique(append(stringValues(successor["supersedes"]), id))
	case "conflict", "resolve_conflict":
		r, err := c.mutableRule(id)
		if err != nil {
			return err
		}
		otherID := textField(action["conflicts_with"])
		other, err := c.mutableRule(otherID)
		if err != nil {
			return err
		}
		if kind == "conflict" {
			r["conflicts_with"] = sortedUnique(append(stringValues(r["conflicts_with"]), otherID))
			other["conflicts_with"] = sortedUnique(append(stringValues(other["conflicts_with"]), id))
		} else {
			r["conflicts_with"] = without(stringValues(r["conflicts_with"]), otherID)
			other["conflicts_with"] = without(stringValues(other["conflicts_with"]), id)
		}
	default:
		return proposalError("unknown catalog action")
	}
	return nil
}
func without(values []string, excluded string) []string {
	out := []string{}
	for _, value := range values {
		if value != excluded {
			out = append(out, value)
		}
	}
	return out
}
