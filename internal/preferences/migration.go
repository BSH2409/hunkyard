package preferences

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

//nolint:gocyclo // Migration keeps backup, validated publication, audit, and completion in one transaction.
func (s *Store) migrationBatch(plan migrationPlan) (maintenanceBatch, *generation, error) {
	batch := maintenanceBatch{localDate: s.config.Clock().Local().Format("2006-01-02"), observations: []map[string]any{}}
	current, err := s.loadCurrent()
	if err != nil {
		return batch, nil, err
	}
	catalog, err := s.catalogFromGeneration(current)
	if err != nil {
		return batch, nil, err
	}
	for _, seed := range plan.seeds {
		directive := textField(seed["directive"])
		scope := record(seed["scope"])
		surfaces := sortedUnique(stringValues(seed["surfaces"]))
		r := newRule(directive, scope, surfaces, "legacy_seed", s.config.Clock())
		id := textField(r["id"])
		if slices.ContainsFunc(catalog.rules, func(r rule) bool { return r["id"] == id }) {
			continue
		}
		for _, key := range []string{"created_at", "updated_at"} {
			if seed[key] != nil {
				r[key] = seed[key]
			}
		}
		created, err := time.Parse(time.RFC3339Nano, textField(r["created_at"]))
		if err != nil {
			return batch, nil, err
		}
		updated, err := time.Parse(time.RFC3339Nano, textField(r["updated_at"]))
		if err != nil || updated.Before(created) {
			return batch, nil, fmt.Errorf("legacy timestamps are reversed")
		}
		if err := recomputeRule(r, s.config.Clock()); err != nil {
			return batch, nil, err
		}
		if _, err := catalog.replaceRule(r); err != nil {
			return batch, nil, err
		}
	}
	batch.catalog = catalog
	batch.rules = cloneRules(catalog.rules)
	for _, observation := range plan.observations {
		if current == nil || !slices.Contains(current.ProcessedIDs, textField(observation["id"])) {
			batch.observations = append(batch.observations, cloneMap(observation))
		}
	}
	source, err := s.sourcePackForGeneration(current)
	if err != nil {
		return batch, nil, err
	}
	if source != nil {
		batch.sourceDigest = source.digest
	}
	if current != nil {
		batch.generationID = &current.ID
	}
	if source != nil && (current == nil || record(current.SourcePacks["code-like-him"])["digest"] != source.digest) {
		batch.source = source
	}
	document := batch.document()
	delete(document, "input_digest")
	document["migration_input_digests"] = plan.digests
	data, err := canonicalJSON(document)
	if err != nil {
		return batch, nil, err
	}
	batch.digest = digest(string(data))
	return batch, current, nil
}

func (s *Store) migrationState() (map[string]any, error) {
	data, err := regularFile(s.dataPath("state.json"))
	if os.IsNotExist(err) || err == nil && strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state map[string]any
	if err := strictjson.Decode(data, &state); err != nil {
		return nil, err
	}
	version, ok := integer(state["schema_version"])
	if !ok || version != schemaVersion {
		return nil, fmt.Errorf("invalid migration state")
	}
	if value, exists := state["migrations"]; exists && record(value) == nil {
		return nil, fmt.Errorf("invalid migration state")
	}
	return state, nil
}

func validateMigrationMarker(entry map[string]any) error {
	if err := exactFields(entry, strings.Fields("backup completed_at generation input_digests"), nil); err != nil {
		return err
	}
	if !generationIDPattern.MatchString(textField(entry["generation"])) || strings.Contains(textField(entry["generation"]), "..") {
		return fmt.Errorf("invalid migrated generation")
	}
	name := textField(entry["backup"])
	if len(name) != 26 || !strings.HasSuffix(name, "-legacy-v1") {
		return fmt.Errorf("invalid migration backup name")
	}
	if _, err := time.Parse("20060102T150405Z", strings.TrimSuffix(name, "-legacy-v1")); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, textField(entry["completed_at"])); err != nil {
		return err
	}
	ids, err := stringArray(entry["input_digests"])
	if err != nil || len(ids) != 2 {
		return fmt.Errorf("invalid migration input digests")
	}
	for _, id := range ids {
		if !sha256Pattern.MatchString(id) {
			return fmt.Errorf("invalid migration digest")
		}
	}
	return nil
}

//nolint:gocyclo // Migration keeps backup, validated publication, audit, and completion in one transaction.
func (s *Store) migrate(projectRoot, repo string, dry bool, proposal []byte) (map[string]any, error) {
	result := map[string]any{"status": "failed", "generation_id": nil}
	execute := func() error {
		state, err := s.migrationState()
		if err != nil {
			return err
		}
		if migrations := record(state["migrations"]); migrations != nil && migrations["legacy_v1"] != nil {
			entry := record(migrations["legacy_v1"])
			if err := validateMigrationMarker(entry); err != nil {
				return err
			}
			dailyIncomplete := slices.ContainsFunc(strings.Fields("last_success_local_date last_success_at last_generation maintenance_runner"), func(key string) bool {
				_, exists := state[key]
				return !exists
			})
			if !dry && dailyIncomplete {
				if err := s.stampMigration(state, entry); err != nil {
					return err
				}
			}
			result["status"] = "already_migrated"
			return nil
		}
		plan, err := s.planMigration(projectRoot, repo)
		if err != nil {
			return err
		}
		result["report"] = plan.report
		batch, current, err := s.migrationBatch(plan)
		if err != nil {
			return err
		}
		if len(proposal) == 0 {
			result["status"] = "needs_proposal"
			result["batch"] = batch.document()
			result["migration_input_digests"] = plan.digests
			files := map[string]any{}
			if batch.source != nil {
				for name, file := range batch.source.files {
					files[name] = map[string]any{"digest": file.Digest, "content": file.Content}
				}
			}
			result["source_pack_files"] = files
			return nil
		}
		var backup string
		var approved map[string]string
		if !dry {
			backup, err = s.backupLegacy(plan)
			if err != nil {
				return err
			}
			result["backup"] = backup
			approved, err = verifyLegacyBackup(backup, plan)
			if err != nil {
				return err
			}
		}
		checked, err := validateProposal(proposal, batch)
		if err != nil {
			return err
		}
		application, err := applyProposal(checked, batch)
		if err != nil {
			return err
		}
		if dry {
			result["status"] = "dry_run"
			return nil
		}
		// Re-read legacy inputs after validation so the verified backup remains authoritative.
		fresh, err := s.planMigration(projectRoot, repo)
		if err != nil {
			return err
		}
		if !sameValue(fresh.digests, plan.digests) {
			return fmt.Errorf("legacy inputs changed during migration")
		}
		files, manifest := renderPersona(application.rules.rules)
		manifest["migration"] = map[string]any{"name": "legacy_v1", "input_digests": plan.digests, "batch_input_digest": batch.digest, "backup": filepath.Base(backup)}
		packs := application.sourcePacks
		ids := application.processedIDs
		if current != nil {
			ids = append(ids, current.ProcessedIDs...)
			if packs == nil {
				packs = current.SourcePacks
			}
		}
		if packs == nil {
			packs = map[string]any{}
		}
		s.legacyPersona = approved
		defer func() { s.legacyPersona = nil }()
		id, err := s.commitLocked(generation{Rules: application.rules, ProcessedIDs: ids, SourcePacks: packs, PersonaFiles: files, Manifest: manifest})
		if err != nil {
			return err
		}
		result["generation_id"] = id
		if s.mirrorError != nil {
			return s.mirrorError
		}
		if err := s.interrupt("migration_published"); err != nil {
			return err
		}
		if err := withFileLock(s.dataPath("observations", ".pending.lock"), false, func() error {
			if _, err := os.Lstat(s.dataPath("observations", "pending.jsonl")); os.IsNotExist(err) {
				return writeNewFile(s.dataPath("observations", "pending.jsonl"), nil)
			} else {
				return err
			}
		}); err != nil {
			return err
		}
		if s.audit(false).Status != "success" {
			return fmt.Errorf("migration audit failed")
		}
		if err := ensureDirectory(s.dataPath("reports")); err != nil {
			return err
		}
		report := cloneMap(plan.report)
		report["generation"] = id
		report["backup"] = filepath.Base(backup)
		report["completed_at"] = s.config.Clock().UTC().Format(time.RFC3339)
		data, err := canonicalJSON(report)
		if err != nil {
			return err
		}
		if err := atomicReplace(s.dataPath("reports", "migration-legacy-v1.json"), append(data, '\n')); err != nil {
			return err
		}
		entry := map[string]any{"completed_at": s.config.Clock().UTC().Format(time.RFC3339), "input_digests": plan.digests, "generation": id, "backup": filepath.Base(backup)}
		migrations := record(state["migrations"])
		if migrations == nil {
			migrations = map[string]any{}
		}
		migrations["legacy_v1"] = entry
		state["migrations"] = migrations
		if err := s.stampMigration(state, entry); err != nil {
			return err
		}
		result["status"] = "success"
		return nil
	}
	if dry {
		return result, execute()
	}
	err := withFileLock(s.dataPath(".maintenance.lock"), true, func() error { return s.withGenerationLock(execute) })
	return result, err
}

func (s *Store) stampMigration(state, entry map[string]any) error {
	completed, err := time.Parse(time.RFC3339Nano, textField(entry["completed_at"]))
	if err != nil {
		return err
	}
	state["schema_version"] = schemaVersion
	state["last_success_local_date"] = completed.Local().Format("2006-01-02")
	state["last_success_at"] = completed.UTC().Format(time.RFC3339)
	state["last_generation"] = entry["generation"]
	state["maintenance_runner"] = "local_proposal"
	data, err := canonicalJSON(state)
	if err != nil {
		return err
	}
	return atomicReplace(s.dataPath("state.json"), append(data, '\n'))
}
