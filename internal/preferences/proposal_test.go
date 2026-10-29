package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

type proposalCatalogFixture struct {
	Rules      []rule `json:"rules"`
	Authorized bool   `json:"authorized"`
	Clock      string `json:"clock"`
}
type proposalSourceFixture struct {
	Files    []struct{ Path, Digest, Content string } `json:"files"`
	Metadata map[string]any                           `json:"metadata"`
	Invalid  bool                                     `json:"invalid_authority"`
}
type proposalBatchFixture struct {
	Digest       string                  `json:"input_digest"`
	Observations []map[string]any        `json:"observations"`
	Rules        []rule                  `json:"rules"`
	Catalog      *proposalCatalogFixture `json:"catalog"`
	Coherent     bool                    `json:"catalogs_coherent"`
	Source       *proposalSourceFixture  `json:"source_pack"`
}
type proposalTrace struct {
	ErrorClass   string `json:"error_class"`
	Case, Kind   string
	Input        any
	Proposal     map[string]any
	Batch        *proposalBatchFixture
	Catalog      *proposalCatalogFixture
	Actions      []map[string]any
	Observations []map[string]any
	Accepted     bool
	Result       json.RawMessage
}

func proposalTraces(t *testing.T) []proposalTrace {
	t.Helper()
	data, err := os.ReadFile("testdata/contracts/proposals.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Reference string          `json:"reference"`
		Events    []proposalTrace `json:"events"`
	}
	if err := strictjson.Decode(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Events) != 264 {
		t.Fatal("incomplete synthetic reference traces")
	}
	return fixture.Events
}

func TestProposalParseReferenceParity(t *testing.T) {
	for i, trace := range proposalTraces(t) {
		if trace.Kind != "parse" {
			continue
		}
		t.Run(fmt.Sprintf("%s/%d", trace.Case, i), func(t *testing.T) {
			input, ok := trace.Input.(string)
			if !ok {
				input = "null"
			}
			proposal, err := parseProposal([]byte(input))
			if (err == nil) != trace.Accepted {
				t.Fatalf("accepted=%v, reference=%v: %v", err == nil, trace.Accepted, err)
			}
			if err == nil {
				actual, _ := canonicalJSON(proposal)
				var expected map[string]any
				if err := strictjson.Decode(trace.Result, &expected); err != nil {
					t.Fatal(err)
				}
				want, _ := canonicalJSON(expected)
				if string(actual) != string(want) {
					t.Fatal("proposal parse changed values")
				}
			}
		})
	}
}

func TestProposalReferenceParity(t *testing.T) {
	for i, trace := range proposalTraces(t) {
		if trace.Kind != "validate" && trace.Kind != "apply" {
			continue
		}
		t.Run(fmt.Sprintf("%s/%s/%d", trace.Case, trace.Kind, i), func(t *testing.T) {
			store := testStore(t)
			batch := maintenanceBatch{}
			if trace.Batch != nil {
				batch.digest = trace.Batch.Digest
				batch.observations = trace.Batch.Observations
				batch.rules = trace.Batch.Rules
				if fixture := trace.Batch.Catalog; fixture != nil {
					if stamp, err := time.Parse(time.RFC3339, fixture.Clock); err == nil {
						store.config.Clock = func() time.Time { return stamp }
					}
					if fixture.Authorized {
						batch.catalog = &catalog{store: store, rules: cloneRules(fixture.Rules)}
					} else {
						batch.catalog = &catalog{rules: cloneRules(fixture.Rules)}
					}
				}
				if source := trace.Batch.Source; source != nil {
					batch.source = &sourceSnapshot{issuer: store, metadata: source.Metadata, files: map[string]sourceFile{}}
					if source.Invalid {
						batch.source.issuer = nil
					}
					for _, file := range source.Files {
						batch.source.files[file.Path] = sourceFile{Path: file.Path, Digest: file.Digest, Content: file.Content}
					}
				}
			}
			input, _ := json.Marshal(trace.Proposal)
			validated, err := validateProposal(input, batch)
			if trace.Kind == "apply" && err == nil {
				before := cloneRules(batch.rules)
				if !trace.Batch.Coherent {
					validated.catalog = &catalog{store: store, rules: cloneRules(batch.rules)}
				}
				result, applyErr := applyProposal(validated, batch)
				err = applyErr
				if err == nil {
					var expected struct {
						Rules        []rule         `json:"rules"`
						ProcessedIDs []string       `json:"processed_ids"`
						SourcePacks  map[string]any `json:"source_packs"`
						Counts       map[string]any `json:"action_counts"`
					}
					if err := strictjson.Decode(trace.Result, &expected); err != nil {
						t.Fatal(err)
					}
					actualRules, _ := canonicalJSON(result.rules.rules)
					wanted, _ := canonicalJSON(expected.Rules)
					if string(actualRules) != string(wanted) {
						t.Fatalf("applied rules differ\nactual %s\nexpected %s", actualRules, wanted)
					}
					if !reflect.DeepEqual(result.processedIDs, expected.ProcessedIDs) {
						t.Fatal("processed observation IDs differ")
					}
					if !sameValue(result.sourcePacks, expected.SourcePacks) || !sameValue(result.counts, expected.Counts) {
						t.Fatal("source metadata or action counts differ")
					}
				} else if !reflect.DeepEqual(before, batch.rules) {
					t.Fatal("failed proposal changed authoritative input")
				}
			}
			if (err == nil) != trace.Accepted {
				t.Fatalf("accepted=%v, reference=%v: %v", err == nil, trace.Accepted, err)
			}
		})
	}
}

func TestCatalogActionReferenceParity(t *testing.T) {
	for i, trace := range proposalTraces(t) {
		if trace.Kind != "catalog" {
			continue
		}
		t.Run(fmt.Sprintf("%s/%d", trace.Case, i), func(t *testing.T) {
			store := testStore(t)
			var c *catalog
			if fixture := trace.Catalog; fixture != nil {
				if stamp, err := time.Parse(time.RFC3339, fixture.Clock); err == nil {
					store.config.Clock = func() time.Time { return stamp }
				}
				c = &catalog{rules: cloneRules(fixture.Rules)}
				if fixture.Authorized {
					c.store = store
				}
			}
			var before []rule
			if c != nil {
				before = cloneRules(c.rules)
			}
			err := c.applyActions(trace.Actions, trace.Observations)
			if (err == nil) != trace.Accepted {
				t.Fatalf("accepted=%v reference=%v: %v", err == nil, trace.Accepted, err)
			}
			if err != nil {
				if c != nil && !sameValue(before, c.rules) {
					t.Fatal("failed action mutated its authorized catalog")
				}
				return
			}
			var expected []rule
			if err := strictjson.Decode(trace.Result, &expected); err != nil {
				t.Fatal(err)
			}
			if !sameValue(c.rules, expected) {
				actual, _ := canonicalJSON(c.rules)
				want, _ := canonicalJSON(expected)
				t.Fatalf("catalog differs\nactual %s\nexpected %s", actual, want)
			}
		})
	}
}

func TestProposalSchemaReferenceContract(t *testing.T) {
	var schema map[string]any
	if err := strictjson.Decode(proposalSchema, &schema); err != nil {
		t.Fatal(err)
	}
	definitions := record(schema["$defs"])
	for _, name := range []string{"action", "proposed_rule", "scope", "source_ref", "summary"} {
		if record(definitions[name])["additionalProperties"] != false {
			t.Fatalf("%s schema permits unknown properties", name)
		}
	}
	actionProperties := record(record(definitions["action"])["properties"])
	if schema["additionalProperties"] != false || len(stringValues(record(actionProperties["type"])["enum"])) != 11 {
		t.Fatal("schema action contract changed")
	}
	if !sameValue(sortedUnique(stringValues(record(definitions["proposed_rule"])["required"])), sortedUnique(proposedFields)) {
		t.Fatal("schema proposed fields differ")
	}
	if record(record(schema["properties"])["schema_version"])["type"] != "integer" {
		t.Fatal("schema_version must remain an integer")
	}
	scope := record(definitions["scope"])
	properties := record(scope["properties"])
	if !sameValue(record(properties["kind"])["enum"], []string{"global", "repository"}) || !sameValue(record(properties["repo_key"])["type"], []string{"string", "null"}) {
		t.Fatal("scope schema changed")
	}
	// Both crossed pairs remain schema-valid; runtime scope validation enforces the pairing.
	for _, scope := range []map[string]any{{"kind": "global", "repo_key": nil}, {"kind": "repository", "repo_key": "alpha-a1b2c3d4e5f6"}} {
		if err := validateScope(scope); err != nil {
			t.Fatal(err)
		}
	}
	for _, scope := range []map[string]any{{"kind": "global", "repo_key": "alpha-a1b2c3d4e5f6"}, {"kind": "repository", "repo_key": nil}} {
		if validateScope(scope) == nil {
			t.Fatal("runtime accepted an incoherent scope pair")
		}
	}
	unsupported := map[string]bool{"allOf": true, "oneOf": true, "if": true, "then": true, "else": true, "not": true, "uniqueItems": true, "minLength": true, "maxLength": true}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if unsupported[key] {
					t.Fatalf("unsupported schema composition %s", key)
				}
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(schema)
}

func TestProposalAtomicityAndForgery(t *testing.T) {
	store := testStore(t)
	c := testCatalog(t, store, ruleFixture("global"))
	before := cloneRules(c.rules)
	actions := []map[string]any{{"action": "create", "rule": map[string]any{"id": "new", "directive": "Prefer clear names.", "scope": map[string]any{"kind": "global", "repo_key": nil}, "surfaces": []string{"naming"}}}, {"type": "invented_action"}}
	if err := c.applyActions(actions, []map[string]any{}); err == nil {
		t.Fatal("invalid final action accepted")
	}
	if !sameValue(before, c.rules) {
		t.Fatal("earlier action escaped a failed transaction")
	}
	evidence := observationFixture("obs-one", "session-one", "alpha-a1b2c3d4e5f6")
	batch := maintenanceBatch{digest: digest("locked Go fixture"), observations: []map[string]any{evidence}, rules: cloneRules(c.rules), catalog: c}
	summary := map[string]any{}
	for _, key := range summaryFields {
		summary[key] = 0
	}
	summary["processed"] = 1
	summary["ignored"] = 1
	document := map[string]any{"schema_version": 1, "input_digest": batch.digest, "actions": []any{map[string]any{"type": "ignore_observation", "observation_ids": []string{"obs-one"}, "rule_id": nil, "target_rule_ids": []string{}, "reason_code": "task_local", "source_ref": nil, "proposed_rule": nil}}, "summary": summary}
	encoded, _ := canonicalJSON(document)
	validated, err := validateProposal(encoded, batch)
	if err != nil {
		t.Fatal(err)
	}
	document["summary"].(map[string]any)["ignored"] = 0
	if _, err := applyProposal(validated, batch); err != nil {
		t.Fatalf("caller mutation changed a validated defensive copy: %v", err)
	}
	if _, err := applyProposal(validatedProposal{}, batch); err == nil {
		t.Fatal("zero validation authority accepted")
	}
	forged := batch
	forged.catalog = &catalog{store: store, rules: cloneRules(c.rules)}
	if _, err := applyProposal(validated, forged); err == nil {
		t.Fatal("replacement catalog authority accepted")
	}
	changed := batch
	changed.digest = digest("different locked batch")
	if _, err := applyProposal(validated, changed); err == nil {
		t.Fatal("stale batch digest accepted")
	}
	validated.document["summary"].(map[string]any)["ignored"] = 0
	if _, err := applyProposal(validated, batch); err == nil {
		t.Fatal("mutated proposal bypassed application validation")
	}
	if !sameValue(before, c.rules) {
		t.Fatal("proposal application mutated the authoritative input catalog")
	}
}

func TestProposalMalformedEnumCannotPanic(t *testing.T) {
	document := map[string]any{"schema_version": 1, "input_digest": digest("fixture"), "actions": []any{map[string]any{"type": "create_rule", "observation_ids": []string{"obs-one"}, "rule_id": "new", "target_rule_ids": []string{}, "reason_code": "durable_preference", "source_ref": nil, "proposed_rule": map[string]any{"id": "new", "directive": "Prefer clear names.", "scope": map[string]any{"kind": "global", "repo_key": nil}, "surfaces": []string{"naming"}, "load_tier": map[string]any{"unexpected": true}, "origin": "inferred", "source_pack": nil, "source_ref": nil, "supersedes": []string{}, "conflicts_with": []string{}}}}, "summary": map[string]any{}}
	encoded, _ := canonicalJSON(document)
	if _, err := parseProposal(encoded); err == nil {
		t.Fatal("object accepted as an optional enum")
	}
}
