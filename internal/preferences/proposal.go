package preferences

import (
	_ "embed"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

//go:embed assets/maintenance-proposal.json
var proposalSchema []byte

var errStaleProposal = errors.New("proposal input digest does not match the locked batch")

var actionReasons = map[string][]string{
	"ignore_observation": {"task_local", "factual_correction", "too_vague", "duplicate"},
	"create_rule":        {"durable_preference"}, "confirm_rule": {"durable_preference", "confirming_evidence", "duplicate"},
	"merge_rules": {"conditional_merge", "duplicate"}, "refine_rule": {"factual_correction", "too_vague"},
	"change_scope": {"scope_evidence", "explicit_instruction"}, "supersede_rule": {"explicit_supersession", "factual_correction"},
	"quarantine_conflict": {"conflict_detected", "factual_correction"}, "mark_dormant": {"stale_candidate"},
	"reactivate_rule": {"confirming_evidence", "explicit_instruction", "durable_preference"}, "refresh_source_pack": {"source_refresh", "source_no_actionable_rule"},
}
var summaryActions = map[string]string{"ignore_observation": "ignored", "create_rule": "created", "confirm_rule": "confirmed", "merge_rules": "merged", "refine_rule": "refined", "change_scope": "scoped", "supersede_rule": "superseded", "quarantine_conflict": "conflicted", "mark_dormant": "dormant", "reactivate_rule": "reactivated", "refresh_source_pack": "source_pack_refreshed"}
var summaryFields = strings.Fields("processed ignored created confirmed merged refined scoped superseded conflicted dormant reactivated source_pack_refreshed")
var proposedFields = strings.Fields("id directive scope surfaces load_tier origin source_pack source_ref supersedes conflicts_with")

type maintenanceBatch struct {
	localDate    string
	generationID *string
	sourceDigest string
	digest       string
	observations []map[string]any
	rules        []rule
	catalog      *catalog
	source       *sourceSnapshot
}
type validatedProposal struct {
	document map[string]any
	catalog  *catalog
	digest   string
}
type proposalApplication struct {
	rules        validatedRuleSet
	processedIDs []string
	sourcePacks  map[string]any
	counts       map[string]any
}

func record(value any) map[string]any { result, _ := value.(map[string]any); return result }
func textField(value any) string      { result, _ := value.(string); return result }
func stringValues(value any) []string { result, _ := stringArray(value); return result }
func sameValue(a, b any) bool {
	x, err := canonicalJSON(a)
	if err != nil {
		return false
	}
	y, err := canonicalJSON(b)
	return err == nil && string(x) == string(y)
}
func proposalError(message string) error {
	return fmt.Errorf("invalid preference proposal: %s", message)
}

func uniqueProposalStrings(value any, ruleIDs bool) error {
	values, err := stringArray(value)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, id := range values {
		if id == "" || seen[id] || (ruleIDs && !generationIDPattern.MatchString(id)) {
			return proposalError("invalid or duplicate reference")
		}
		seen[id] = true
	}
	return nil
}
func validateProposalCitation(value any) error {
	citation := record(value)
	if err := exactFields(citation, []string{"file", "digest"}, nil); err != nil {
		return err
	}
	if textField(citation["file"]) == "" || !sha256Pattern.MatchString(textField(citation["digest"])) {
		return proposalError("invalid source citation")
	}
	return nil
}
func validateProposedRule(value any) error {
	proposed := record(value)
	if err := exactFields(proposed, proposedFields, nil); err != nil {
		return err
	}
	directive, ok := proposed["directive"].(string)
	if !generationIDPattern.MatchString(textField(proposed["id"])) || !ok || directive == "" || utf8.RuneCountInString(directive) > 240 {
		return proposalError("invalid proposed rule identity or directive")
	}
	if err := validateScope(proposed["scope"]); err != nil {
		return err
	}
	if err := validateSurfaces(proposed["surfaces"]); err != nil {
		return err
	}
	if !optionalEnum(proposed["load_tier"], []string{"core", "surface"}) || !slices.Contains([]string{"feedback", "inferred", "legacy_seed", "source_pack"}, textField(proposed["origin"])) || !optionalEnum(proposed["source_pack"], []string{"code-like-him"}) {
		return proposalError("invalid proposed provenance")
	}
	if proposed["source_ref"] != nil {
		if err := validateProposalCitation(proposed["source_ref"]); err != nil {
			return err
		}
	}
	for _, field := range []string{"supersedes", "conflicts_with"} {
		if err := uniqueProposalStrings(proposed[field], true); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // Proposal validation checks the complete strict schema and evidence contract.
func parseProposal(input []byte) (map[string]any, error) {
	var document map[string]any
	if err := strictjson.Decode(input, &document); err != nil {
		return nil, proposalError("invalid JSON")
	}
	if err := exactFields(document, strings.Fields("schema_version input_digest actions summary"), nil); err != nil {
		return nil, err
	}
	version, ok := integer(document["schema_version"])
	if !ok || version != schemaVersion || !sha256Pattern.MatchString(textField(document["input_digest"])) {
		return nil, proposalError("invalid schema or input digest")
	}
	actions, err := mapArray(document["actions"])
	if err != nil {
		return nil, err
	}
	reasons := map[string]bool{}
	for _, values := range actionReasons {
		for _, reason := range values {
			reasons[reason] = true
		}
	}
	for _, action := range actions {
		if err := exactFields(action, strings.Fields("type observation_ids rule_id target_rule_ids reason_code source_ref proposed_rule"), nil); err != nil {
			return nil, err
		}
		if actionReasons[textField(action["type"])] == nil || !reasons[textField(action["reason_code"])] {
			return nil, proposalError("invalid action or reason")
		}
		if err := uniqueProposalStrings(action["observation_ids"], false); err != nil {
			return nil, err
		}
		if err := uniqueProposalStrings(action["target_rule_ids"], true); err != nil {
			return nil, err
		}
		if action["rule_id"] != nil && !generationIDPattern.MatchString(textField(action["rule_id"])) {
			return nil, proposalError("invalid action rule identity")
		}
		if action["source_ref"] != nil {
			if err := validateProposalCitation(action["source_ref"]); err != nil {
				return nil, err
			}
		}
		if action["proposed_rule"] != nil {
			if err := validateProposedRule(action["proposed_rule"]); err != nil {
				return nil, err
			}
		}
	}
	summary := record(document["summary"])
	if err := exactFields(summary, summaryFields, nil); err != nil {
		return nil, err
	}
	for _, value := range summary {
		if count, ok := integer(value); !ok || count < 0 {
			return nil, proposalError("summary counts must be nonnegative integers")
		}
	}
	return document, nil
}

type proposalValidator struct {
	batch        maintenanceBatch
	actions      []map[string]any
	observations map[string]map[string]any
	rules        map[string]rule
	proposed     map[string]map[string]any
	scopeChanges map[string]map[string]any
	now          time.Time
}

//nolint:gocyclo // Proposal validation checks the complete strict schema and evidence contract.
func validateProposal(input []byte, batch maintenanceBatch) (validatedProposal, error) {
	document, err := parseProposal(input)
	if err != nil {
		return validatedProposal{}, err
	}
	if !sha256Pattern.MatchString(batch.digest) || batch.observations == nil {
		return validatedProposal{}, proposalError("invalid maintenance batch authority")
	}
	if document["input_digest"] != batch.digest {
		return validatedProposal{}, errStaleProposal
	}
	if batch.catalog != nil {
		if batch.catalog.store == nil || !sameValue(batch.rules, batch.catalog.rules) {
			return validatedProposal{}, proposalError("rules do not match the authorized catalog")
		}
	}
	if err := validateRuleGraph(batch.rules); err != nil {
		return validatedProposal{}, err
	}
	if batch.source != nil && (batch.source.issuer == nil || (batch.catalog != nil && batch.source.issuer != batch.catalog.store)) {
		return validatedProposal{}, proposalError("invalid source-pack authority")
	}
	actions, _ := mapArray(document["actions"])
	v := proposalValidator{batch: batch, actions: actions, observations: map[string]map[string]any{}, rules: map[string]rule{}, proposed: map[string]map[string]any{}, scopeChanges: map[string]map[string]any{}, now: time.Now().UTC()}
	if batch.catalog != nil {
		v.now = batch.catalog.store.config.Clock()
	}
	for _, observation := range batch.observations {
		id, ok := observation["id"].(string)
		if !ok || id == "" || v.observations[id] != nil {
			return validatedProposal{}, proposalError("invalid or duplicate observation")
		}
		v.observations[id] = observation
	}
	for _, r := range batch.rules {
		v.rules[textField(r["id"])] = r
	}
	seen := map[string]bool{}
	expected := map[string]any{}
	for _, field := range summaryFields {
		expected[field] = 0
	}
	for _, action := range actions {
		kind := textField(action["type"])
		ids := stringValues(action["observation_ids"])
		if kind == "refresh_source_pack" && len(ids) > 0 {
			return validatedProposal{}, proposalError("source refresh cannot consume observations")
		}
		for _, id := range ids {
			if v.observations[id] == nil || seen[id] {
				return validatedProposal{}, proposalError("unknown or repeated observation")
			}
			seen[id] = true
		}
		expected["processed"] = expected["processed"].(int) + len(ids)
		field := summaryActions[kind]
		expected[field] = expected[field].(int) + 1
		if proposed := record(action["proposed_rule"]); proposed != nil {
			id := textField(proposed["id"])
			if v.rules[id] == nil && v.proposed[id] != nil {
				return validatedProposal{}, proposalError("new rule identity introduced twice")
			}
			v.proposed[id] = proposed
		}
		if kind == "change_scope" {
			id := textField(action["rule_id"])
			if v.scopeChanges[id] != nil {
				return validatedProposal{}, proposalError("repeated scope change")
			}
			v.scopeChanges[id] = action
		}
	}
	if len(seen) != len(v.observations) {
		return validatedProposal{}, proposalError("every pending observation must be classified exactly once")
	}
	for _, action := range actions {
		if err := v.validateAction(action); err != nil {
			return validatedProposal{}, err
		}
	}
	if err := v.validateSourceCoverage(); err != nil {
		return validatedProposal{}, err
	}
	if err := v.validateGraphs(); err != nil {
		return validatedProposal{}, err
	}
	if !sameValue(expected, document["summary"]) {
		return validatedProposal{}, proposalError("summary differs from recomputed counts")
	}
	return validatedProposal{document: cloneMap(document), catalog: batch.catalog, digest: batch.digest}, nil
}

func explicitObservation(observation map[string]any) bool {
	return observation["source"] == "explicit" || observation["explicit"] == true
}
func (v *proposalValidator) actionObservations(action map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, id := range stringValues(action["observation_ids"]) {
		out = append(out, v.observations[id])
	}
	return out
}
func (v *proposalValidator) mergedObservations(rules []rule, action map[string]any) []map[string]any {
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, r := range rules {
		values, _ := mapArray(r["observations"])
		for _, o := range values {
			id := textField(o["id"])
			if !seen[id] {
				seen[id] = true
				out = append(out, o)
			}
		}
	}
	for _, o := range v.actionObservations(action) {
		id := textField(o["id"])
		if !seen[id] {
			seen[id] = true
			out = append(out, o)
		}
	}
	return out
}
func validateScopeEvidence(scope map[string]any, observations []map[string]any) error {
	if scope["kind"] == "global" {
		return nil
	}
	if len(observations) == 0 {
		return proposalError("repository scope needs matching evidence")
	}
	for _, o := range observations {
		if o["repo_key"] != scope["repo_key"] {
			return proposalError("repository scope does not match evidence")
		}
	}
	return nil
}
func repositoriesAndExplicit(observations []map[string]any) (int, bool) {
	repositories := map[string]bool{}
	explicit := false
	for _, o := range observations {
		if repo := textField(o["repo_key"]); repo != "" {
			repositories[repo] = true
		}
		explicit = explicit || explicitObservation(o)
	}
	return len(repositories), explicit
}
func validateProposedDirective(directive string) error {
	privacy := sanitize(directive)
	if directive != normalize(directive) || !privacy.Accepted || privacy.Excerpt != directive {
		return proposalError("directive must already be normalized and privacy-safe")
	}
	return nil
}
func (v *proposalValidator) personalProposed(proposed map[string]any, observations []map[string]any, isNew bool) error {
	origin := textField(proposed["origin"])
	if !slices.Contains([]string{"inferred", "feedback", "legacy_seed"}, origin) || (isNew && origin != "inferred") || proposed["load_tier"] != nil || proposed["source_pack"] != nil || proposed["source_ref"] != nil {
		return proposalError("personal rule cannot invent provenance")
	}
	if err := validateProposedDirective(textField(proposed["directive"])); err != nil {
		return err
	}
	if isNew {
		scope := record(proposed["scope"])
		if err := validateScopeEvidence(scope, observations); err != nil {
			return err
		}
		if scope["kind"] == "global" && origin == "inferred" {
			count, _ := repositoriesAndExplicit(observations)
			if count < 2 {
				return proposalError("global inference needs two repositories")
			}
		}
	}
	return nil
}
func (v *proposalValidator) personalOrSource(proposed map[string]any, observations []map[string]any, existing rule) error {
	if proposed["origin"] == "source_pack" {
		return v.sourceProposed(proposed)
	}
	return v.personalProposed(proposed, observations, existing == nil)
}
func preservedRelationships(existing rule, proposed map[string]any) bool {
	for _, field := range []string{"supersedes", "conflicts_with"} {
		if !reflect.DeepEqual(sortedUnique(stringValues(existing[field])), sortedUnique(stringValues(proposed[field]))) {
			return false
		}
	}
	return true
}
func sameProvenance(existing rule, proposed map[string]any) bool {
	for _, field := range []string{"origin", "load_tier", "source_pack", "source_ref"} {
		if !sameValue(existing[field], proposed[field]) {
			return false
		}
	}
	return true
}
func mergedOrigin(rules []rule) string {
	for _, origin := range []string{"feedback", "inferred", "legacy_seed"} {
		for _, r := range rules {
			if r["origin"] == origin {
				return origin
			}
		}
	}
	return "source_pack"
}

var refinementTokensPattern = regexp.MustCompile(`[a-z0-9]+`)
var negativeDirective = regexp.MustCompile(`(?i)^(?:avoid|never(?:\s+use)?|do\s+not(?:\s+use)?|don't(?:\s+use)?|must\s+not|should\s+not)\b`)
var positiveDirective = regexp.MustCompile(`(?i)^(?:prefer|use|always(?:\s+use)?|choose|favor)\b`)

func directivePolarity(text string) int {
	text = strings.TrimSpace(text)
	if negativeDirective.MatchString(text) {
		return -1
	}
	if positiveDirective.MatchString(text) {
		return 1
	}
	return 0
}
func refinementTokens(text string) []string {
	stop := strings.Fields("a an and are as at be by for from in is it of on or prefer should that the this to use with")
	tokens := []string{}
	for _, token := range refinementTokensPattern.FindAllString(strings.ToLower(text), -1) {
		if slices.Contains(stop, token) {
			continue
		}
		if len(token) > 3 {
			token = strings.TrimSuffix(token, "s")
		}
		tokens = append(tokens, token)
	}
	return sortedUnique(tokens)
}
func lexicallyContinuous(before, after string) bool {
	a, b := refinementTokens(before), refinementTokens(after)
	shorter := min(len(a), len(b))
	if shorter == 0 {
		return false
	}
	shared := 0
	for _, token := range a {
		if slices.Contains(b, token) {
			shared++
		}
	}
	minimum := 1
	if shorter > 1 {
		minimum = max(2, (shorter*6+9)/10)
	}
	return shared >= minimum
}

func optionalEnum(value any, allowed []string) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	return ok && slices.Contains(allowed, text)
}
