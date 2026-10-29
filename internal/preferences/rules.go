package preferences

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

func invalidRecord(message string) error { return fmt.Errorf("invalid preference record: %s", message) }

func integer(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		if int64(int(v)) == v {
			return int(v), true
		}
	case uint64:
		if v <= math.MaxInt {
			return int(v), true
		}
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err == nil && int64(int(parsed)) == parsed {
			return int(parsed), true
		}
	}
	return 0, false
}

func stringArray(value any) ([]string, error) {
	switch values := value.(type) {
	case []string:
		if values != nil {
			return append([]string{}, values...), nil
		}
	case []any:
		if values == nil {
			return nil, invalidRecord("expected string array")
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, invalidRecord("expected string array")
			}
			out = append(out, text)
		}
		return out, nil
	}
	return nil, invalidRecord("expected string array")
}

func mapArray(value any) ([]map[string]any, error) {
	entries, ok := value.([]any)
	if !ok || entries == nil {
		return nil, invalidRecord("expected record array")
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		record, ok := entry.(map[string]any)
		if !ok {
			return nil, invalidRecord("expected record")
		}
		out = append(out, record)
	}
	return out, nil
}

func validateScope(value any) error {
	scope, ok := value.(map[string]any)
	if !ok {
		return invalidRecord("scope must be a hash")
	}
	if err := exactFields(scope, []string{"kind", "repo_key"}, nil); err != nil {
		return err
	}
	switch scope["kind"] {
	case "global":
		if scope["repo_key"] != nil {
			return invalidRecord("global scope cannot have repository key")
		}
	case "repository":
		key, ok := scope["repo_key"].(string)
		if !ok || !repositoryKeyPattern.MatchString(key) {
			return invalidRecord("invalid repository scope key")
		}
	default:
		return invalidRecord("invalid scope kind")
	}
	return nil
}

func validateSurfaces(value any) error {
	values, err := stringArray(value)
	if err != nil {
		return err
	}
	if len(values) == 0 || len(sortedUnique(values)) != len(values) {
		return invalidRecord("surfaces must be nonempty and unique")
	}
	for _, surface := range values {
		if !slices.Contains(surfaces, surface) {
			return invalidRecord("unknown surface")
		}
	}
	return nil
}

//nolint:gocyclo // The complete canonical rule and graph invariants are validated together.
func validateObservationReference(value map[string]any) error {
	if err := exactFields(value, strings.Fields("captured_at explicit id repo_key safe_excerpt session_key"), []string{"context"}); err != nil {
		return err
	}
	if id, ok := value["id"].(string); !ok || id == "" {
		return invalidRecord("observation needs id")
	}
	if err := validateTimestamp(value["captured_at"]); err != nil {
		return err
	}
	if value["session_key"] != nil {
		if _, ok := value["session_key"].(string); !ok {
			return invalidRecord("invalid session key")
		}
	}
	if value["repo_key"] != nil {
		if key, ok := value["repo_key"].(string); !ok || !repositoryKeyPattern.MatchString(key) {
			return invalidRecord("invalid observation repository key")
		}
	}
	if _, ok := value["explicit"].(bool); !ok {
		return invalidRecord("explicit marker must be boolean")
	}
	if excerpt := value["safe_excerpt"]; excerpt != nil {
		text, ok := excerpt.(string)
		if !ok {
			return invalidRecord("excerpt must be text")
		}
		privacy := sanitize(text)
		if !privacy.Accepted || privacy.Excerpt != text {
			return invalidRecord("excerpt must already be sanitized")
		}
	}
	if context, ok := value["context"]; ok {
		if err := validateContext(context); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // The complete canonical rule and graph invariants are validated together.
func validateRule(r rule) error {
	if r == nil {
		return invalidRecord("rule must be a hash")
	}
	if err := exactFields(r, strings.Fields("created_at directive evidence examples id observations origin scope status surfaces updated_at"), strings.Fields("conflicts_with load_tier retired_at source_pack source_ref superseded_by supersedes")); err != nil {
		return err
	}
	if id, ok := r["id"].(string); !ok || !generationIDPattern.MatchString(id) {
		return invalidRecord("invalid rule id")
	}
	directive, ok := r["directive"].(string)
	if !ok || strings.TrimSpace(directive) == "" || len([]rune(directive)) > 500 {
		return invalidRecord("invalid directive length")
	}
	if err := validateScope(r["scope"]); err != nil {
		return err
	}
	if err := validateSurfaces(r["surfaces"]); err != nil {
		return err
	}
	origin, ok := r["origin"].(string)
	if !ok || !slices.Contains([]string{"feedback", "inferred", "legacy_seed", "source_pack"}, origin) {
		return invalidRecord("invalid origin")
	}
	status, ok := r["status"].(string)
	if !ok || !slices.Contains([]string{"active", "candidate", "conflicted", "core", "dormant", "superseded"}, status) {
		return invalidRecord("invalid status")
	}
	for _, field := range []string{"created_at", "updated_at", "retired_at"} {
		if value, ok := r[field]; ok {
			if err := validateTimestamp(value); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"load_tier", "source_pack", "source_ref"} {
		_, present := r[field]
		if present != (origin == "source_pack") {
			return invalidRecord("incomplete or misplaced source provenance")
		}
	}
	if origin == "source_pack" {
		if r["load_tier"] != "core" && r["load_tier"] != "surface" {
			return invalidRecord("invalid source tier")
		}
		if name, ok := r["source_pack"].(string); !ok || name == "" {
			return invalidRecord("source pack needs name")
		}
		ref, ok := r["source_ref"].(map[string]any)
		if !ok {
			return invalidRecord("invalid source citation")
		}
		if err := exactFields(ref, []string{"digest", "file"}, nil); err != nil {
			return err
		}
		hash, ok := ref["digest"].(string)
		if !ok || !sha256Pattern.MatchString(hash) {
			return invalidRecord("invalid source digest")
		}
		if _, ok := ref["file"].(string); !ok {
			return invalidRecord("invalid citation file")
		}
	}
	evidence, ok := r["evidence"].(map[string]any)
	if !ok {
		return invalidRecord("evidence must be a hash")
	}
	if err := exactFields(evidence, strings.Fields("explicit_count observation_count repository_count session_count"), nil); err != nil {
		return err
	}
	for _, value := range evidence {
		count, ok := integer(value)
		if !ok || count < 0 {
			return invalidRecord("evidence must contain nonnegative integers")
		}
	}
	observations, err := mapArray(r["observations"])
	if err != nil {
		return err
	}
	for _, observation := range observations {
		if err := validateObservationReference(observation); err != nil {
			return err
		}
	}
	examples, err := stringArray(r["examples"])
	if err != nil {
		return err
	}
	if len(examples) > 2 {
		return invalidRecord("too many examples")
	}
	for _, example := range examples {
		privacy := sanitize(example)
		if !privacy.Accepted || privacy.Excerpt != example {
			return invalidRecord("example must be sanitized")
		}
	}
	for _, field := range []string{"conflicts_with", "supersedes"} {
		if value, ok := r[field]; ok {
			refs, err := stringArray(value)
			if err != nil || len(sortedUnique(refs)) != len(refs) {
				return invalidRecord("references must be unique strings")
			}
		}
	}
	if value, ok := r["superseded_by"]; ok {
		if _, ok := value.(string); !ok {
			return invalidRecord("successor must be a rule id")
		}
	}
	return nil
}

//nolint:gocyclo // The complete canonical rule and graph invariants are validated together.
func validateRuleGraph(rules []rule) error {
	byID := map[string]rule{}
	for _, r := range rules {
		if err := validateRule(r); err != nil {
			return err
		}
		id := r["id"].(string)
		if _, ok := byID[id]; ok {
			return invalidRecord("duplicate rule id")
		}
		byID[id] = r
	}
	graph := map[string][]string{}
	for id, r := range byID {
		graph[id] = []string{}
		for _, field := range []string{"supersedes", "conflicts_with"} {
			value, ok := r[field]
			if !ok {
				continue
			}
			refs, _ := stringArray(value)
			for _, ref := range refs {
				other, ok := byID[ref]
				if !ok || ref == id {
					return invalidRecord("unknown or self reference")
				}
				if field == "supersedes" {
					graph[id] = append(graph[id], ref)
				} else {
					reverse, _ := stringArray(other["conflicts_with"])
					if !slices.Contains(reverse, id) {
						return invalidRecord("conflicts must be reciprocal")
					}
					if r["status"] == "active" || r["status"] == "core" {
						return invalidRecord("active rule has unresolved conflicts")
					}
				}
			}
		}
		if successor, ok := r["superseded_by"].(string); ok {
			if _, ok := byID[successor]; !ok || successor == id {
				return invalidRecord("unknown or self successor")
			}
		}
	}
	for id, r := range byID {
		if successor, ok := r["superseded_by"].(string); ok {
			graph[successor] = append(graph[successor], id)
		}
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return invalidRecord("supersession cycle")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, next := range graph[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range graph {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // The complete canonical rule and graph invariants are validated together.
func recomputeRule(r rule, now time.Time) error {
	observations, err := mapArray(r["observations"])
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	sessions := map[string]bool{}
	repositories := map[string]bool{}
	explicit := 0
	unique := []map[string]any{}
	for _, o := range observations {
		id, _ := o["id"].(string)
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, o)
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i]["captured_at"] != unique[j]["captured_at"] {
			return fmt.Sprint(unique[i]["captured_at"]) < fmt.Sprint(unique[j]["captured_at"])
		}
		return fmt.Sprint(unique[i]["id"]) < fmt.Sprint(unique[j]["id"])
	})
	canonical := []any{}
	examples := []string{}
	for _, o := range unique {
		canonical = append(canonical, o)
		if session, ok := o["session_key"].(string); ok {
			sessions[session] = true
		}
		if repo, ok := o["repo_key"].(string); ok {
			repositories[repo] = true
		}
		if o["explicit"] == true {
			explicit++
		}
		if text, ok := o["safe_excerpt"].(string); ok && len(examples) < 2 && !slices.Contains(examples, text) {
			examples = append(examples, text)
		}
	}
	r["observations"] = canonical
	r["examples"] = examples
	r["evidence"] = map[string]any{"observation_count": len(unique), "session_count": len(sessions), "repository_count": len(repositories), "explicit_count": explicit}
	scope, _ := r["scope"].(map[string]any)
	globalReady := scope["kind"] != "global" || r["origin"] == "feedback" || len(repositories) >= 2
	conflicts, _ := stringArray(r["conflicts_with"])
	_, retired := r["retired_at"]
	_, superseded := r["superseded_by"]
	updated, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(r["updated_at"]))
	switch {
	case retired || superseded:
		r["status"] = "superseded"
	case len(conflicts) > 0:
		r["status"] = "conflicted"
	case r["origin"] == "source_pack":
		r["status"] = "active"
	case len(unique) >= 5 && len(sessions) >= 3 && globalReady:
		r["status"] = "core"
	case explicit > 0 || len(sessions) >= 2 && globalReady:
		r["status"] = "active"
	case now.Sub(updated) >= 180*24*time.Hour:
		r["status"] = "dormant"
	default:
		r["status"] = "candidate"
	}
	return validateRule(r)
}

func normalizeRules(rules []rule, now time.Time) ([]rule, error) {
	result := cloneRules(rules)
	for _, r := range result {
		if err := validateRule(r); err != nil {
			return nil, err
		}
		if value, ok := r["conflicts_with"]; ok {
			values, _ := stringArray(value)
			r["conflicts_with"] = sortedUnique(values)
		}
		if err := recomputeRule(r, now); err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i]["id"].(string) < result[j]["id"].(string) })
	return result, validateRuleGraph(result)
}
