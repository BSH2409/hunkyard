package preferences

import (
	"errors"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationFailureAndRetryBoundaries(t *testing.T) {
	for _, stage := range []string{"invalid_proposal", "before_swap", "migration_published"} {
		t.Run(stage, func(t *testing.T) {
			s := testStore(t)
			legacyFixture(t, s.config.Root)
			project := t.TempDir()
			legacyFixture(t, project)
			plan, err := s.planMigration(project, "example-a1b2c3d4e5f6")
			if err != nil {
				t.Fatal(err)
			}
			batch, _, err := s.migrationBatch(plan)
			if err != nil {
				t.Fatal(err)
			}
			proposal := ignoredProposal(t, batch)
			if stage == "invalid_proposal" {
				proposal = []byte(`{"sensitive-proposal-detail":true}`)
			} else {
				s.fault = func(point string) error {
					if point == stage {
						return errors.New("injected migration fault")
					}
					return nil
				}
			}
			failed, err := s.migrate(project, "example-a1b2c3d4e5f6", false, proposal)
			if err == nil || failed["status"] != "failed" {
				t.Fatal("migration failure was not surfaced", failed, err)
			}
			if _, err := verifyLegacyBackup(textField(failed["backup"]), plan); err != nil {
				t.Fatal("failure lost verified backup", err)
			}
			if _, err := os.Stat(s.dataPath("state.json")); !os.IsNotExist(err) {
				t.Fatal("failed migration stamped completion")
			}
			current, err := s.loadCurrent()
			if err != nil || (current != nil) != (stage == "migration_published") {
				t.Fatal("failed publication state differs", err)
			}
			s.fault = nil
			retry, _, err := s.migrationBatch(plan)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "migration_published" && (len(retry.observations) != 0 || retry.source != nil) {
				t.Fatal("retry reprocessed published migration evidence")
			}
			result, err := s.migrate(project, "example-a1b2c3d4e5f6", false, ignoredProposal(t, retry))
			if err != nil || result["status"] != "success" || result["backup"] != failed["backup"] {
				t.Fatal("migration retry failed or replaced verified backup", result, err)
			}
		})
	}
}

func TestMigrationCompletionValidationAndStampRepair(t *testing.T) {
	s := testStore(t)
	legacyFixture(t, s.config.Root)
	project := t.TempDir()
	legacyFixture(t, project)
	plan, err := s.planMigration(project, "example-a1b2c3d4e5f6")
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.migrationBatch(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.migrate(project, "example-a1b2c3d4e5f6", false, ignoredProposal(t, batch)); err != nil {
		t.Fatal(err)
	}
	state, err := s.migrationState()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"last_generation", "last_success_at", "last_success_local_date", "maintenance_runner"} {
		delete(state, key)
	}
	data, err := canonicalJSON(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicReplace(s.dataPath("state.json"), data); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(s.dataPath("generations"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.migrate(project, "example-a1b2c3d4e5f6", false, nil); err != nil || result["status"] != "already_migrated" {
		t.Fatal("completion stamp repair failed", err)
	}
	after, err := os.ReadDir(s.dataPath("generations"))
	if err != nil || len(before) != len(after) {
		t.Fatal("completion repair published a generation", err)
	}
	state, err = s.migrationState()
	if err != nil || state["last_success_local_date"] != "2026-08-19" || state["last_generation"] != record(record(state["migrations"])["legacy_v1"])["generation"] {
		t.Fatal("daily stamp not repaired", err)
	}
	state["migrations"] = map[string]any{"legacy_v1": map[string]any{}}
	data, err = canonicalJSON(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicReplace(s.dataPath("state.json"), data); err != nil {
		t.Fatal(err)
	}
	if _, err := s.migrate(project, "example-a1b2c3d4e5f6", false, nil); err == nil {
		t.Fatal("incomplete completion marker accepted")
	}
}

func TestMigrationReferenceParity(t *testing.T) {
	data, err := os.ReadFile("testdata/contracts/migration.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Reference string
		Events    []struct {
			Case, Kind, Text string
			Scope            map[string]any
			Accepted         bool
			GlobalText       string `json:"global_text"`
			ProjectText      string `json:"project_text"`
			RepoKey          string `json:"repo_key"`
			Clock            string
			Result           map[string]any
		}
	}
	if err := strictjson.Decode(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, event := range fixture.Events {
		t.Run(fmt.Sprintf("%s/%s/%d", event.Case, event.Kind, i), func(t *testing.T) {
			write := func(root, text string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "lessons"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "lessons", "lessons.md"), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if event.Kind == "read" {
				root := t.TempDir()
				write(root, event.Text)
				result, err := readLegacy(root, event.Scope)
				if (err == nil) != event.Accepted {
					t.Fatalf("legacy acceptance differs: %v", err)
				}
				if err == nil && !sameValue(map[string]any{"digest": result.digest, "rules": result.rules, "rejected": result.rejected}, event.Result) {
					t.Fatal("normalized legacy record differs")
				}
				return
			}
			s := testStore(t)
			clock, err := time.Parse(time.RFC3339, event.Clock)
			if err != nil {
				t.Fatal(err)
			}
			s.config.Clock = func() time.Time { return clock }
			project := t.TempDir()
			write(s.config.Root, event.GlobalText)
			write(project, event.ProjectText)
			plan, err := s.planMigration(project, event.RepoKey)
			if err != nil {
				t.Fatal(err)
			}
			actual := map[string]any{"seeds": plan.seeds, "observations": plan.observations, "report": plan.report, "input_digests": plan.digests, "legacy_global_persona_files": plan.personaPaths}
			if !sameValue(actual, event.Result) {
				t.Fatalf("migration plan differs: actual %v expected %v", actual, event.Result)
			}
		})
	}
}

func legacyFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "lessons"), 0700); err != nil {
		t.Fatal(err)
	}
	text := "# Legacy fixture\n\n```yaml\nrules:\n  - id: seed-1\n    description: Prefer explicit service contracts.\n    category: architecture\n    source: seed\n    confidence: 999\n    usage_count: 999\n  - id: feedback-1\n    description: Prefer idempotent retries.\n    category: testing\n    source: feedback\n    raw_examples:\n      - Always test retry behavior\n      - 'password=synthetic-value'\n```\n"
	if err := os.WriteFile(filepath.Join(root, "lessons", "lessons.md"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationLocalBackupAndIdempotency(t *testing.T) {
	s := testStore(t)
	sourceFixture(t, s)
	project := filepath.Join(t.TempDir(), "project")
	legacyFixture(t, s.config.Root)
	legacyFixture(t, project)
	plan, err := s.planMigration(project, "example-a1b2c3d4e5f6")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.seeds) != 1 || len(plan.observations) != 1 {
		t.Fatal("legacy duplicates inflated evidence")
	}
	batch, current, err := s.migrationBatch(plan)
	if err != nil || current != nil {
		t.Fatal(err)
	}
	proposal := ignoredProposal(t, batch)
	dry, err := s.migrate(project, "example-a1b2c3d4e5f6", true, proposal)
	if err != nil || dry["status"] != "dry_run" {
		t.Fatal("migration dry run failed", dry, err)
	}
	if _, err := os.Stat(s.dataPath("backups")); !os.IsNotExist(err) {
		t.Fatal("migration dry run wrote backup")
	}
	result, err := s.migrate(project, "example-a1b2c3d4e5f6", false, proposal)
	if err != nil || result["status"] != "success" {
		t.Fatal("migration failed", result, err)
	}
	current, err = s.loadCurrent()
	if err != nil || len(current.Rules.rules) != 1 || current.Rules.rules[0]["status"] != "candidate" {
		t.Fatal("legacy counters promoted a rule", err)
	}
	backup := textField(result["backup"])
	if _, err := os.Stat(filepath.Join(backup, "global", "lessons", "lessons.md")); err != nil {
		t.Fatal("verified backup missing", err)
	}
	replayed, err := s.migrate(project, "example-a1b2c3d4e5f6", false, nil)
	if err != nil || replayed["status"] != "already_migrated" {
		t.Fatal("migration replay failed", replayed, err)
	}
	// A completed migration must not roll back a later maintenance checkpoint.
	state, err := s.migrationState()
	if err != nil {
		t.Fatal(err)
	}
	state["last_success_local_date"] = "2026-08-20"
	state["last_success_at"] = "2026-08-20T12:00:00Z"
	state["last_generation"] = "later-generation"
	data, err := canonicalJSON(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicReplace(s.dataPath("state.json"), data); err != nil {
		t.Fatal(err)
	}
	if _, err := s.migrate(project, "example-a1b2c3d4e5f6", false, nil); err != nil {
		t.Fatal(err)
	}
	after, err := regularFile(s.dataPath("state.json"))
	if err != nil || string(after) != string(data) {
		t.Fatal("migration rewrote a newer maintenance checkpoint", err)
	}
}
