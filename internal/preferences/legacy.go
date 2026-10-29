package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
	"gopkg.in/yaml.v3"
)

type legacyDocument struct {
	root, digest string
	rules        []map[string]any
	rejected     int
}
type migrationPlan struct {
	global, project legacyDocument
	seeds           []rule
	observations    []map[string]any
	report          map[string]any
	digests         []string
	personaPaths    []string
}

var legacyFence = regexp.MustCompile("(?mi)^```yaml[ \\t]*\\r?\\n([\\s\\S]*?)^```[ \\t]*$")
var categorySurfaces = map[string]string{"architecture": "architecture", "code_review": "reviews", "communication": "communication", "database": "database", "devops": "devops", "general": "architecture", "naming": "naming", "rails": "rails", "ruby_style": "ruby", "schema_design": "database", "security": "security", "testing": "testing", "wordpress": "wordpress"}

func legacyString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func legacyTimestamp(value any) (any, error) {
	if value == nil || value == "" {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("invalid legacy timestamp")
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, err
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

//nolint:gocyclo // Legacy normalization checks every accepted field before planning the import.
func readLegacy(root string, scope map[string]any) (legacyDocument, error) {
	result := legacyDocument{root: root, rules: []map[string]any{}}
	if err := validateScope(scope); err != nil {
		return result, err
	}
	if err := checkDirectoryChain(root, filepath.Join(root, "lessons")); err != nil {
		return result, err
	}
	data, err := regularFile(filepath.Join(root, "lessons", "lessons.md"))
	if err != nil {
		return result, err
	}
	result.digest = digest(string(data))
	fences := legacyFence.FindAllSubmatch(data, -1)
	if len(fences) != 1 {
		return result, fmt.Errorf("legacy lessons need exactly one YAML fence")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(fences[0][1]))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return result, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return result, fmt.Errorf("legacy lessons need one YAML document")
	}
	var timestamps func(*yaml.Node) error
	timestamps = func(n *yaml.Node) error {
		if n.Tag == "!!timestamp" {
			var parsed time.Time
			if err := n.Decode(&parsed); err != nil {
				return err
			}
			n.Tag = "!!str"
			n.Value = parsed.UTC().Format(time.RFC3339)
		}
		for _, child := range n.Content {
			if err := timestamps(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := timestamps(&node); err != nil {
		return result, err
	}
	if err := checkYAMLNode(&node, 0); err != nil {
		return result, err
	}
	var document map[string]any
	if err := node.Decode(&document); err != nil {
		return result, err
	}
	rules, err := mapArray(document["rules"])
	if err != nil {
		return result, err
	}
	for _, r := range rules {
		directive := normalize(legacyString(r["description"]))
		privacy := sanitize(directive)
		if !privacy.Accepted || privacy.Excerpt != directive {
			return result, fmt.Errorf("legacy directive is not privacy-safe")
		}
		source := legacyString(r["source"])
		if source != "seed" && source != "feedback" {
			return result, fmt.Errorf("invalid legacy source")
		}
		examples := []string{}
		if value, exists := r["raw_examples"]; exists {
			values, err := stringArray(value)
			if err != nil {
				return result, err
			}
			for _, example := range values {
				privacy := sanitize(example)
				if privacy.Accepted {
					examples = append(examples, privacy.Excerpt)
				} else {
					result.rejected++
				}
			}
		}
		first, err := legacyTimestamp(r["first_seen"])
		if err != nil {
			return result, err
		}
		last, err := legacyTimestamp(r["last_seen"])
		if err != nil {
			return result, err
		}
		persona := legacyString(r["persona_file"])
		if persona != "" && (!safeRelative(persona) || !strings.HasSuffix(persona, ".md")) {
			return result, fmt.Errorf("unsafe legacy persona path")
		}
		for _, key := range []string{"confidence", "usage_count"} {
			if value := r[key]; value != nil {
				var numeric float64
				switch n := value.(type) {
				case int:
					numeric = float64(n)
				case uint64:
					numeric = float64(n)
				case float64:
					numeric = n
				default:
					return result, fmt.Errorf("invalid legacy count")
				}
				if math.IsNaN(numeric) || math.IsInf(numeric, 0) || numeric < 0 {
					return result, fmt.Errorf("invalid legacy count")
				}
			}
		}
		surface := categorySurfaces[legacyString(r["category"])]
		if surface == "" {
			surface = "architecture"
		}
		result.rules = append(result.rules, map[string]any{"legacy_id": legacyString(r["id"]), "directive": directive, "source": source, "scope": cloneMap(scope), "surfaces": []string{surface}, "persona_file": nullableString(persona), "safe_examples": examples, "first_seen": first, "last_seen": last, "legacy_confidence": r["confidence"], "legacy_usage_count": r["usage_count"]})
	}
	return result, nil
}

//nolint:gocyclo // Legacy normalization checks every accepted field before planning the import.
func (s *Store) planMigration(projectRoot, repo string) (migrationPlan, error) {
	plan := migrationPlan{seeds: []rule{}, observations: []map[string]any{}, personaPaths: []string{}}
	if !repositoryKeyPattern.MatchString(repo) {
		return plan, fmt.Errorf("migration needs a canonical repository key")
	}
	var err error
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return plan, err
	}
	plan.global, err = readLegacy(s.config.Root, map[string]any{"kind": "global", "repo_key": nil})
	if err != nil {
		return plan, err
	}
	plan.project, err = readLegacy(projectRoot, map[string]any{"kind": "repository", "repo_key": repo})
	if err != nil {
		return plan, err
	}
	plan.digests = []string{plan.global.digest, plan.project.digest}
	sort.Strings(plan.digests)
	globalSeeds := map[string]bool{}
	globalExamples := map[string]bool{}
	for _, r := range plan.global.rules {
		if r["source"] == "seed" {
			globalSeeds[strings.ToLower(textField(r["directive"]))] = true
		} else {
			for _, example := range stringValues(r["safe_examples"]) {
				globalExamples[sanitize(example).Fingerprint] = true
			}
		}
		if path := textFieldPointer(r["persona_file"]); path != "" {
			plan.personaPaths = append(plan.personaPaths, path)
		}
	}
	seedSeen, observationSeen := map[string]bool{}, map[string]bool{}
	all := append(append([]map[string]any{}, plan.global.rules...), plan.project.rules...)
	for _, r := range all {
		if r["source"] == "seed" {
			scope := record(r["scope"])
			directive := textField(r["directive"])
			key := strings.ToLower(directive)
			if globalSeeds[key] {
				scope = map[string]any{"kind": "global", "repo_key": nil}
			}
			encoded, _ := jsonScope(scope)
			identity := key + "\x00" + encoded
			if seedSeen[identity] {
				continue
			}
			seedSeen[identity] = true
			plan.seeds = append(plan.seeds, rule{"id": "legacy_seed_" + digest(identity)[:24], "directive": directive, "scope": cloneMap(scope), "surfaces": stringValues(r["surfaces"]), "origin": "legacy_seed", "status": "candidate", "evidence": map[string]any{"observation_ids": []string{}, "explicit_count": 0, "correction_count": 0, "distinct_sessions": 0, "distinct_repositories": 0}, "created_at": r["first_seen"], "updated_at": r["last_seen"]})
			if r["last_seen"] == nil {
				plan.seeds[len(plan.seeds)-1]["updated_at"] = r["first_seen"]
			}
			continue
		}
		for _, example := range stringValues(r["safe_examples"]) {
			privacy := sanitize(example)
			scope := record(r["scope"])
			if globalExamples[privacy.Fingerprint] {
				scope = map[string]any{"kind": "global", "repo_key": nil}
			}
			stamp := textField(r["last_seen"])
			if stamp == "" {
				stamp = textField(r["first_seen"])
			}
			if stamp == "" {
				stamp = s.config.Clock().UTC().Format(time.RFC3339)
			}
			encoded, _ := jsonScope(scope)
			identity := privacy.Fingerprint + "\x00" + encoded + "\x00" + stamp
			if observationSeen[identity] {
				continue
			}
			observationSeen[identity] = true
			repo := textField(scope["repo_key"])
			turn := digest(stamp)
			signals, detected, _ := analyzeSignals(example)
			signals.DurableLanguage = true
			entry := observation{ID: "obs_" + digest(strings.Join([]string{"migration", privacy.Fingerprint, "", turn, repo}, "\x00")), CapturedAt: stamp, Source: "migration", RepoKey: nullableString(repo), SessionKey: nil, TurnKey: &turn, PromptFingerprint: privacy.Fingerprint, SafeExcerpt: &privacy.Excerpt, Signals: signals, SuggestedSurfaces: detected}
			data, err := canonicalJSON(entry)
			if err != nil {
				return plan, err
			}
			var value map[string]any
			if err := strictjson.Decode(data, &value); err != nil {
				return plan, err
			}
			plan.observations = append(plan.observations, value)
		}
	}
	sort.Slice(plan.seeds, func(i, j int) bool {
		a, b := plan.seeds[i], plan.seeds[j]
		ak := textField(record(a["scope"])["kind"]) + "\x00" + textField(a["directive"]) + "\x00" + textField(a["id"])
		bk := textField(record(b["scope"])["kind"]) + "\x00" + textField(b["directive"]) + "\x00" + textField(b["id"])
		return ak < bk
	})
	sort.Slice(plan.observations, func(i, j int) bool {
		a, b := plan.observations[i], plan.observations[j]
		return textField(a["captured_at"])+"\x00"+textField(a["id"]) < textField(b["captured_at"])+"\x00"+textField(b["id"])
	})
	ranges := func(key string) map[string]any {
		values := []float64{}
		for _, r := range all {
			if r[key] != nil {
				switch n := r[key].(type) {
				case int:
					values = append(values, float64(n))
				case uint64:
					values = append(values, float64(n))
				case float64:
					values = append(values, n)
				}
			}
		}
		result := map[string]any{"min": nil, "max": nil, "count": len(values)}
		if len(values) > 0 {
			sort.Float64s(values)
			result["min"] = values[0]
			result["max"] = values[len(values)-1]
		}
		return result
	}
	plan.report = map[string]any{"schema_version": schemaVersion, "migration": "legacy_v1", "input_digests": plan.digests, "legacy_rules": map[string]any{"global": len(plan.global.rules), "project": len(plan.project.rules)}, "seeds": map[string]any{"candidate": len(plan.seeds)}, "observations": map[string]any{"accepted": len(plan.observations), "rejected_sensitive": plan.global.rejected + plan.project.rejected}, "legacy_confidence": ranges("legacy_confidence"), "legacy_usage_count": ranges("legacy_usage_count")}
	plan.personaPaths = sortedUnique(plan.personaPaths)
	return plan, nil
}

func textFieldPointer(value any) string {
	if p, ok := value.(*string); ok && p != nil {
		return *p
	}
	return textField(value)
}
func jsonScope(scope map[string]any) (string, error) { // Scope field order is part of legacy identity.
	data, err := json.Marshal(struct {
		Kind    any `json:"kind"`
		RepoKey any `json:"repo_key"`
	}{scope["kind"], scope["repo_key"]})
	return string(data), err
}
