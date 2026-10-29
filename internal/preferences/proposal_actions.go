package preferences

import (
	"slices"
	"time"
)

//nolint:gocyclo // Each schema action stays beside the common evidence and provenance guards.
func (v *proposalValidator) validateAction(action map[string]any) error {
	kind, reason, id := textField(action["type"]), textField(action["reason_code"]), textField(action["rule_id"])
	targets := stringValues(action["target_rule_ids"])
	evidence := v.actionObservations(action)
	proposed := record(action["proposed_rule"])
	existing := v.rules[id]
	if !slices.Contains(actionReasons[kind], reason) {
		return proposalError("reason does not match action")
	}
	_, explicit := repositoriesAndExplicit(evidence)
	if reason == "explicit_instruction" && kind != "reactivate_rule" && !explicit {
		return proposalError("explicit instruction needs explicit evidence")
	}
	if kind != "refresh_source_pack" && action["source_ref"] != nil {
		return proposalError("only source refresh can cite a source")
	}
	if proposed != nil && proposed["id"] != action["rule_id"] {
		return proposalError("proposed identity does not match action")
	}
	switch kind {
	case "ignore_observation":
		if len(evidence) == 0 || action["rule_id"] != nil || len(targets) > 0 || proposed != nil {
			return proposalError("ignore action cannot contain rule data")
		}
	case "create_rule":
		if len(evidence) == 0 || len(targets) > 0 || proposed == nil || existing != nil {
			return proposalError("invalid new rule action")
		}
		if len(stringValues(proposed["supersedes"]))+len(stringValues(proposed["conflicts_with"])) > 0 {
			return proposalError("new rule needs explicit relationship action")
		}
		return v.personalProposed(proposed, evidence, true)
	case "confirm_rule":
		if len(evidence) == 0 || len(targets) > 0 || existing == nil || proposed != nil || existing["origin"] == "source_pack" || !slices.Contains([]string{"candidate", "active", "core"}, textField(existing["status"])) {
			return proposalError("confirmation needs a live personal rule and evidence")
		}
		return validateScopeEvidence(record(existing["scope"]), evidence)
	case "merge_rules":
		if len(targets) < 2 || !slices.Contains(targets, id) || proposed == nil {
			return proposalError("merge must retain one of at least two existing targets")
		}
		rules := []rule{}
		scopes := []any{}
		for _, target := range targets {
			r := v.rules[target]
			if r == nil || r["status"] == "conflicted" || r["status"] == "superseded" {
				return proposalError("merge has incompatible targets")
			}
			rules = append(rules, r)
			found := false
			for _, scope := range scopes {
				found = found || sameValue(scope, r["scope"])
			}
			if !found {
				scopes = append(scopes, r["scope"])
			}
		}
		merged := v.mergedObservations(rules, action)
		if len(scopes) > 1 {
			scopeChange := v.scopeChanges[id]
			if scopeChange == nil || !sameValue(proposed["scope"], record(scopeChange["proposed_rule"])["scope"]) {
				return proposalError("cross-scope merge needs a matching scope change")
			}
			combined := append(append([]map[string]any{}, merged...), v.actionObservations(scopeChange)...)
			scope := record(proposed["scope"])
			if err := validateScopeEvidence(scope, combined); err != nil {
				return err
			}
			count, explicit := repositoriesAndExplicit(combined)
			if scope["kind"] == "global" && ((proposed["origin"] == "feedback" && !explicit && count < 2) || (proposed["origin"] == "inferred" && count < 2)) {
				return proposalError("global merge lacks cross-repository or explicit evidence")
			}
		}
		if proposed["origin"] != mergedOrigin(rules) {
			return proposalError("merge cannot change provenance")
		}
		if err := v.personalOrSource(proposed, merged, existing); err != nil {
			return err
		}
		supersedes := []string{}
		for _, r := range rules {
			supersedes = append(supersedes, stringValues(r["supersedes"])...)
		}
		for _, target := range targets {
			if target != id {
				supersedes = append(supersedes, target)
			}
		}
		if !sameValue(sortedUnique(supersedes), sortedUnique(stringValues(proposed["supersedes"]))) || len(stringValues(proposed["conflicts_with"])) > 0 {
			return proposalError("merge must preserve merged identities without adding conflicts")
		}
	case "refine_rule", "change_scope":
		if existing == nil || len(targets) != 1 || targets[0] != id || proposed == nil || existing["origin"] == "source_pack" || !sameProvenance(existing, proposed) {
			return proposalError("rewrite needs one matching personal target and preserved provenance")
		}
		if !preservedRelationships(existing, proposed) {
			return proposalError("rewrite cannot change relationships")
		}
		if kind == "change_scope" {
			if sameValue(existing["scope"], proposed["scope"]) || existing["directive"] != proposed["directive"] || !sameValue(existing["surfaces"], proposed["surfaces"]) {
				return proposalError("scope change cannot rewrite meaning")
			}
			all := v.mergedObservations([]rule{existing}, action)
			scope := record(proposed["scope"])
			if scope["kind"] == "global" {
				count, explicit := repositoriesAndExplicit(all)
				if count < 2 && !explicit {
					return proposalError("global scope change lacks evidence")
				}
			} else {
				return validateScopeEvidence(scope, all)
			}
		} else {
			if !sameValue(existing["scope"], proposed["scope"]) {
				return proposalError("refinement cannot change scope")
			}
			for _, surface := range stringValues(proposed["surfaces"]) {
				if !slices.Contains(stringValues(existing["surfaces"]), surface) {
					return proposalError("refinement cannot expand surfaces")
				}
			}
			changed := existing["directive"] != proposed["directive"] || !sameValue(existing["surfaces"], proposed["surfaces"])
			corrected := false
			for _, o := range evidence {
				corrected = corrected || record(o["signals"])["correction"] == true
			}
			if changed && (len(evidence) == 0 || !corrected) {
				return proposalError("semantic refinement needs correction evidence")
			}
			before, after := textField(existing["directive"]), textField(proposed["directive"])
			if before != after && (directivePolarity(before) != directivePolarity(after) || !lexicallyContinuous(before, after)) {
				return proposalError("refinement cannot invert polarity or replace rule meaning")
			}
			return v.personalOrSource(proposed, v.mergedObservations([]rule{existing}, action), existing)
		}
	case "supersede_rule":
		if len(targets) != 1 || v.rules[targets[0]] == nil || id == "" || id == targets[0] || existing != nil && existing["origin"] == "source_pack" {
			return proposalError("supersession needs a distinct personal successor and one predecessor")
		}
		if proposed != nil {
			if !slices.Contains(stringValues(proposed["supersedes"]), targets[0]) {
				return proposalError("replacement must declare its predecessor")
			}
			if existing != nil {
				if !sameProvenance(existing, proposed) {
					return proposalError("supersession cannot invent successor provenance")
				}
			} else {
				if len(evidence) == 0 {
					return proposalError("new successor needs evidence")
				}
				return v.personalProposed(proposed, evidence, true)
			}
		} else if existing == nil || len(evidence) > 0 {
			return proposalError("existing successor must be known and consume no evidence")
		}
	case "quarantine_conflict":
		if len(targets) < 2 {
			return proposalError("conflict needs at least two targets")
		}
		if proposed != nil {
			if !slices.Contains(targets, id) {
				return proposalError("incoming conflict must be a target")
			}
			if err := v.personalProposed(proposed, evidence, existing == nil); err != nil {
				return err
			}
			refs := []string{}
			for _, target := range targets {
				if target != id {
					refs = append(refs, target)
				}
			}
			if !sameValue(sortedUnique(refs), sortedUnique(stringValues(proposed["conflicts_with"]))) {
				return proposalError("incoming conflict links must match targets")
			}
		} else if len(evidence) > 0 {
			return proposalError("existing conflict cannot consume evidence")
		}
		for _, target := range targets {
			if v.rules[target] == nil && v.proposed[target] == nil {
				return proposalError("unknown conflict target")
			}
		}
		if id != "" && !slices.Contains(targets, id) {
			return proposalError("conflict identity must be a target")
		}
	case "mark_dormant":
		if existing == nil || len(targets) > 0 || proposed != nil || len(evidence) > 0 || existing["status"] != "candidate" || !dormancyEligible(existing, v.now) {
			return proposalError("only an old candidate may become dormant")
		}
	case "reactivate_rule":
		if existing == nil || len(targets) > 0 || proposed != nil || len(evidence) == 0 || existing["status"] != "dormant" {
			return proposalError("reactivation needs a dormant rule and new evidence")
		}
		if reason == "explicit_instruction" && !explicit {
			return proposalError("reactivation lacks explicit evidence")
		}
		previous, _ := mapArray(existing["observations"])
		for _, o := range evidence {
			for _, p := range previous {
				if o["id"] == p["id"] {
					return proposalError("reactivation cannot reuse old evidence")
				}
			}
		}
		return validateScopeEvidence(record(existing["scope"]), evidence)
	case "refresh_source_pack":
		if v.batch.source == nil || action["source_ref"] == nil || len(targets) > 0 {
			return proposalError("source refresh needs changed source authority and exact citation")
		}
		if err := v.batch.source.validateCitation(record(action["source_ref"])); err != nil {
			return err
		}
		if proposed == nil {
			if action["rule_id"] != nil || reason != "source_no_actionable_rule" {
				return proposalError("empty source refresh needs exact no-action reason")
			}
			return nil
		}
		if reason != "source_refresh" || !sameValue(action["source_ref"], proposed["source_ref"]) || existing != nil && existing["origin"] != "source_pack" {
			return proposalError("source refresh cannot change personal provenance or citation")
		}
		if err := v.sourceProposed(proposed); err != nil {
			return err
		}
		for _, target := range stringValues(proposed["supersedes"]) {
			if v.rules[target] == nil || v.rules[target]["origin"] != "source_pack" {
				return proposalError("source refresh can supersede only source rules")
			}
		}
	default:
		return proposalError("unknown action")
	}
	return nil
}

func dormancyEligible(r rule, now time.Time) bool {
	timestamp := textField(r["updated_at"])
	if timestamp == "" {
		timestamp = textField(r["created_at"])
	}
	stamp, err := time.Parse(time.RFC3339, timestamp)
	return err == nil && now.Sub(stamp) >= 180*24*time.Hour
}
func (v *proposalValidator) validateSourceCoverage() error {
	cited := map[string]bool{}
	for _, action := range v.actions {
		if action["type"] == "refresh_source_pack" {
			cited[textField(record(action["source_ref"])["file"])] = true
		}
	}
	if v.batch.source == nil {
		if len(cited) > 0 {
			return proposalError("unchanged source cannot be refreshed")
		}
		return nil
	}
	for path := range v.batch.source.files {
		if !cited[path] {
			return proposalError("every allowlisted source file must be classified")
		}
	}
	return nil
}

//nolint:gocyclo // Each schema action stays beside the common evidence and provenance guards.
func (v *proposalValidator) validateGraphs() error {
	supersedes, conflicts := map[string][]string{}, map[string][]string{}
	for id, r := range v.rules {
		supersedes[id] = append([]string{}, stringValues(r["supersedes"])...)
		conflicts[id] = append([]string{}, stringValues(r["conflicts_with"])...)
	}
	for id, r := range v.rules {
		if successor := textField(r["superseded_by"]); successor != "" {
			supersedes[successor] = append(supersedes[successor], id)
		}
	}
	for id := range v.proposed {
		if supersedes[id] == nil {
			supersedes[id] = []string{}
		}
		if conflicts[id] == nil {
			conflicts[id] = []string{}
		}
	}
	for id, proposed := range v.proposed {
		for _, field := range []string{"supersedes", "conflicts_with"} {
			for _, ref := range stringValues(proposed[field]) {
				if ref == id || v.rules[ref] == nil && v.proposed[ref] == nil {
					return proposalError("invalid graph reference")
				}
			}
		}
		supersedes[id] = stringValues(proposed["supersedes"])
		if refs := stringValues(proposed["conflicts_with"]); len(refs) > 0 {
			conflicts[id] = refs
		}
	}
	for _, action := range v.actions {
		id := textField(action["rule_id"])
		targets := stringValues(action["target_rule_ids"])
		switch action["type"] {
		case "merge_rules", "supersede_rule":
			for _, target := range targets {
				if target != id {
					supersedes[id] = append(supersedes[id], target)
				}
			}
		case "quarantine_conflict":
			for _, left := range targets {
				for _, right := range targets {
					if left != right {
						conflicts[left] = append(conflicts[left], right)
					}
				}
			}
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return proposalError("supersession cycle")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, next := range supersedes[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range supersedes {
		if err := visit(id); err != nil {
			return err
		}
	}
	for id, refs := range conflicts {
		for _, other := range refs {
			if other == id || !slices.Contains(conflicts[other], id) {
				return proposalError("conflicts must be symmetric without self-links")
			}
		}
	}
	return nil
}

func (v *proposalValidator) sourceProposed(proposed map[string]any) error {
	if v.batch.source == nil || proposed["origin"] != "source_pack" || proposed["source_pack"] != "code-like-him" || !slices.Contains([]string{"core", "surface"}, textField(proposed["load_tier"])) || proposed["source_ref"] == nil {
		return proposalError("source proposal lacks exact authority")
	}
	if err := validateProposedDirective(textField(proposed["directive"])); err != nil {
		return err
	}
	return v.batch.source.validateProposedRule(proposed)
}
