package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceBatchSourceBinding(t *testing.T) {
	s := testStore(t)
	root := sourcePackFixture(t, s)
	if err := os.WriteFile(filepath.Join(root, "unreferenced.md"), []byte("not supplied to the host"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil || batch.source == nil || len(batch.source.files) != 4 || len(batch.observations) != 1 {
		t.Fatal("batch contains extra source files or lacks evidence", err)
	}
	document := batch.document()
	delete(document, "input_digest")
	encoded, err := canonicalJSON(document)
	if err != nil || digest(string(encoded)) != batch.digest {
		t.Fatal("batch digest does not bind canonical input", err)
	}
	if result, err := s.maintain("due", ignoredProposal(t, batch)); err != nil || result["status"] != "success" {
		t.Fatal("source import failed", result, err)
	}
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	next, _, err := s.buildMaintenanceBatch(false)
	if err != nil || len(next.observations) != 0 || next.source != nil {
		t.Fatal("processed evidence or unchanged sources were restaged", err)
	}
	name := filepath.Join(root, "SKILL.md")
	data, err := regularFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, append(data, []byte("\nNew source guidance.\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if audit := s.audit(false); audit.Status != "failed" || !strings.Contains(strings.Join(audit.Errors, ","), "source_pack_stale") {
		t.Fatal("audit accepted changed source bytes", audit)
	}
	var out, errOut bytes.Buffer
	if exit := Run([]string{"audit", "--root", s.config.Root, "--json"}, strings.NewReader(""), &out, &errOut); exit != 5 {
		t.Fatal("CLI audit accepted stale source metadata", exit, out.String())
	}
	result, err := s.maintain("force", nil)
	if err != nil || result["status"] != "needs_proposal" || len(record(result["source_pack_files"])) != 4 {
		t.Fatal("changed allowlisted source snapshot was not exported", result, err)
	}
	request, err := json.Marshal(result)
	if err != nil || strings.Contains(string(request), "not supplied to the host") || strings.Contains(string(request), "unreferenced.md") {
		t.Fatal("unreferenced source was exported", err)
	}
}

func TestMaintenanceValidationQuarantineAndLock(t *testing.T) {
	s := testStore(t)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	name := s.dataPath("observations", "pending.jsonl")
	valid, err := regularFile(name)
	if err != nil {
		t.Fatal(err)
	}
	malformed := append(append([]byte{}, valid...), []byte("\nnot-json\n\n")...)
	if err := atomicReplace(name, malformed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.maintain("dry_run", nil); err == nil {
		t.Fatal("dry run accepted malformed pending evidence")
	}
	after, err := regularFile(name)
	if err != nil || !bytes.Equal(after, malformed) {
		t.Fatal("dry run mutated pending evidence", err)
	}
	if _, err := os.Stat(s.dataPath("observations", "quarantine")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote quarantine")
	}
	result, err := s.maintain("force", nil)
	if err != nil || result["status"] != "needs_proposal" {
		t.Fatal("force could not repair malformed evidence", result, err)
	}
	after, err = regularFile(name)
	if err != nil || !bytes.Equal(after, valid) {
		t.Fatal("repair altered valid evidence", err)
	}
	entries, err := os.ReadDir(s.dataPath("observations", "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatal("malformed line not quarantined exactly once", err)
	}
	quarantined, err := regularFile(s.dataPath("observations", "quarantine", entries[0].Name()))
	if err != nil || string(quarantined) != "not-json\n" {
		t.Fatal("quarantine changed raw invalid record", err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.maintain("due", []byte(`{"unsafe-detail":"not a proposal"}`)); err == nil {
		t.Fatal("invalid proposal accepted")
	}
	if _, err := os.Stat(s.dataPath("state.json")); !os.IsNotExist(err) {
		t.Fatal("validation failure stamped success")
	}
	if err := withFileLock(s.dataPath(".maintenance.lock"), false, func() error {
		result, err := s.maintain("due", nil)
		if err != nil || result["status"] != "already_running" {
			t.Fatal("nonblocking maintenance lock differs", result, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if result, err := s.maintain("due", ignoredProposal(t, batch)); err != nil || result["status"] != "success" {
		t.Fatal("validation retry failed", result, err)
	}
}

func TestMaintenanceMirrorFailureAndMigrationMetadata(t *testing.T) {
	s := testStore(t)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	migration := map[string]any{"legacy_v1": map[string]any{"fixture": "keep"}}
	state := map[string]any{"schema_version": 1, "migrations": migration}
	encoded, err := canonicalJSON(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicReplace(s.dataPath("state.json"), encoded); err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureDirectory(filepath.Join(s.config.Root, "persona")); err != nil {
		t.Fatal(err)
	}
	manual := filepath.Join(s.config.Root, "persona", "core.md")
	if err := os.WriteFile(manual, []byte("manual guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.maintain("due", ignoredProposal(t, batch)); err == nil {
		t.Fatal("mirror failure stamped success")
	}
	state = readJSONFile(s.dataPath("state.json"))
	if state["last_success_local_date"] != nil || !sameValue(state["migrations"], migration) {
		t.Fatal("failed maintenance changed completion or migration state")
	}
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		t.Fatal("mirror failure lost published generation", err)
	}
	if err := os.Remove(manual); err != nil {
		t.Fatal(err)
	}
	if result, err := s.maintain("due", nil); err != nil || result["status"] != "success" || result["generation_id"] != current.ID {
		t.Fatal("mirror retry reapplied proposal", result, err)
	}
	state = readJSONFile(s.dataPath("state.json"))
	if !sameValue(state["migrations"], migration) || state["last_success_local_date"] != "2026-08-19" {
		t.Fatal("daily success lost migration metadata")
	}
	if result, err := s.maintain("force", nil); err != nil || result["status"] != "needs_proposal" {
		t.Fatal("force did not bypass daily stamp", result, err)
	}
}

func TestMaintenanceInterruptedPublicationPreservesConcurrentCapture(t *testing.T) {
	for _, stage := range []string{"maintenance_published", "maintenance_archived"} {
		t.Run(stage, func(t *testing.T) {
			s := testStore(t)
			sourceFixture(t, s)
			input := captureInput()
			if _, err := s.Capture(input); err != nil {
				t.Fatal(err)
			}
			batch, _, err := s.buildMaintenanceBatch(false)
			if err != nil {
				t.Fatal(err)
			}
			s.fault = func(point string) error {
				if point == stage {
					return errors.New("injected interruption")
				}
				return nil
			}
			if _, err := s.maintain("due", ignoredProposal(t, batch)); err == nil {
				t.Fatal("interruption did not fail")
			}
			published, err := s.loadCurrent()
			if err != nil || published == nil {
				t.Fatal("publication lost", err)
			}
			s.fault = nil
			input.TurnID = "concurrent-turn"
			if _, err := s.Capture(input); err != nil {
				t.Fatal(err)
			}
			result, err := s.maintain("due", nil)
			if err != nil || result["status"] != "success" || result["generation_id"] != published.ID {
				t.Fatal("recovery reapplied proposal", result, err)
			}
			if s.pendingCount() != 1 {
				t.Fatal("recovery lost concurrent capture")
			}
			archive, err := os.ReadFile(s.dataPath("observations", "archive", "2026-08.jsonl"))
			if err != nil || strings.Count(string(archive), "\n") != 1 {
				t.Fatal("archive duplicated evidence", err)
			}
		})
	}
}

func TestGenerationRemainsReadableAsCandidatesAge(t *testing.T) {
	s := testStore(t)
	catalog := testCatalog(t, s, ruleFixture("repository"))
	files, manifest := renderPersona(catalog.rules)
	if _, err := s.commitGeneration(generation{Rules: catalog.ruleSet(), ProcessedIDs: []string{}, SourcePacks: map[string]any{}, PersonaFiles: files, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	later := s.config.Clock().Add(181 * 24 * time.Hour)
	s.config.Clock = func() time.Time { return later }
	if _, err := s.loadCurrent(); err != nil {
		t.Fatal("aging made a valid generation unreadable", err)
	}
}

func TestMaintenanceRetentionPreservesOnePriorGeneration(t *testing.T) {
	s := testStore(t)
	for i := range 3 {
		next := emptyGeneration()
		next.Manifest["fixture"] = i
		if _, err := s.commitGeneration(next); err != nil {
			t.Fatal(err)
		}
	}
	current, err := s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.dataPath("generations"))
	if err != nil {
		t.Fatal(err)
	}
	old := s.config.Clock().Add(-100 * 24 * time.Hour)
	for _, entry := range entries {
		if err := os.Chtimes(s.dataPath("generations", entry.Name()), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.pruneRetention(current.ID); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(s.dataPath("generations"))
	if err != nil || len(entries) != 2 {
		t.Fatal("retention discarded every prior generation", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, s.dataPath("logs")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	if err := s.pruneRetention(current.ID); err == nil {
		t.Fatal("retention followed a symlink root")
	}
}

func TestMaintenanceWithoutAnOptionalSourcePack(t *testing.T) {
	s := testStore(t)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal("personal maintenance requires an unrelated source skill", err)
	}
	if batch.source != nil || batch.document()["source_pack_digest"] != nil {
		t.Fatal("missing source baseline was invented")
	}
	result, err := s.maintain("due", ignoredProposal(t, batch))
	if err != nil || result["status"] != "success" {
		t.Fatal("personal-only maintenance failed", result, err)
	}
}

func sourceFixture(t *testing.T, s *Store) {
	t.Helper()
	root := s.dataPath("sources", "code-like-him")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# Fixture\n## References\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func ignoredProposal(t *testing.T, batch maintenanceBatch) []byte {
	t.Helper()
	actions := []any{}
	summary := map[string]any{}
	for _, name := range summaryFields {
		summary[name] = 0
	}
	add := func(kind, reason string, ids []string, citation any) {
		actions = append(actions, map[string]any{"type": kind, "reason_code": reason, "observation_ids": ids, "rule_id": nil, "target_rule_ids": []string{}, "source_ref": citation, "proposed_rule": nil})
		summary[summaryActions[kind]] = summary[summaryActions[kind]].(int) + 1
	}
	for _, o := range batch.observations {
		add("ignore_observation", "too_vague", []string{textField(o["id"])}, nil)
		summary["processed"] = summary["processed"].(int) + 1
	}
	if batch.source != nil {
		for _, file := range batch.source.files {
			add("refresh_source_pack", "source_no_actionable_rule", []string{}, map[string]any{"file": file.Path, "digest": file.Digest})
		}
	}
	data, err := canonicalJSON(map[string]any{"schema_version": schemaVersion, "input_digest": batch.digest, "actions": actions, "summary": summary})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMaintenanceLocalProposalAndRecovery(t *testing.T) {
	s := testStore(t)
	sourceFixture(t, s)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.dataPath("observations", "pending.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	proposal := ignoredProposal(t, batch)
	dry, err := s.maintain("dry_run", proposal)
	if err != nil || dry["status"] != "dry_run" {
		t.Fatal("dry run rejected", dry, err)
	}
	if current, err := s.loadCurrent(); err != nil || current != nil {
		t.Fatal("dry run published", err)
	}
	after, err := os.ReadFile(s.dataPath("observations", "pending.jsonl"))
	if err != nil || string(before) != string(after) {
		t.Fatal("dry run changed inbox", err)
	}
	first, err := s.maintain("due", proposal)
	if err != nil || first["status"] != "success" {
		t.Fatal("local proposal failed", first, err)
	}
	if s.pendingCount() != 0 {
		t.Fatal("processed inbox not compacted")
	}
	second, err := s.maintain("due", nil)
	if err != nil || second["status"] != "already_complete" {
		t.Fatal("daily replay did not stop", second, err)
	}
	archive, err := os.ReadFile(s.dataPath("observations", "archive", "2026-08.jsonl"))
	if err != nil || string(archive) != string(before) {
		t.Fatal("archive lost evidence", err)
	}
	if err := os.Remove(s.dataPath("state.json")); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.maintain("due", nil)
	if err != nil || recovered["status"] != "success" {
		t.Fatal("publication recovery failed", recovered, err)
	}
	current, err := s.loadCurrent()
	if err != nil || current.ID != first["generation_id"] {
		t.Fatal("recovery reapplied actions", err)
	}
	if err := os.RemoveAll(s.dataPath("sources", "code-like-him")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.buildMaintenanceBatch(false); err == nil {
		t.Fatal("a missing recorded source pack was silently forgotten")
	}
}

func TestMaintenanceRecoversPublicationAcrossMidnight(t *testing.T) {
	s := testStore(t)
	sourceFixture(t, s)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	s.fault = func(stage string) error {
		if stage == "maintenance_published" {
			return errors.New("interrupted")
		}
		return nil
	}
	if _, err := s.maintain("due", ignoredProposal(t, batch)); err == nil {
		t.Fatal("missing injected interruption")
	}
	previous, err := s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	later := s.config.Clock().Add(24 * time.Hour)
	s.config.Clock = func() time.Time { return later }
	s.fault = nil
	result, err := s.maintain("due", nil)
	if err != nil || result["status"] != "needs_proposal" {
		t.Fatal("today's batch unavailable", result, err)
	}
	state := readJSONFile(s.dataPath("state.json"))
	if state["last_generation"] != previous.ID || state["last_success_local_date"] != "2026-08-19" || s.pendingCount() != 0 {
		t.Fatal("yesterday's publication was not finalized", state)
	}
	archive, err := os.ReadFile(s.dataPath("observations", "archive", "2026-08.jsonl"))
	if err != nil || strings.Count(string(archive), "\n") != 1 {
		t.Fatal("missing recovered archive", err)
	}
	if record(result["batch"])["local_date"] != "2026-08-20" {
		t.Fatal("recovery skipped today's work", result)
	}
}
