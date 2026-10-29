package preferences

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

func (b maintenanceBatch) document() map[string]any {
	var sourceDigest any
	if b.sourceDigest != "" {
		sourceDigest = b.sourceDigest
	}
	return map[string]any{"schema_version": schemaVersion, "local_date": b.localDate, "generation_id": b.generationID, "observations": b.observations, "rules": b.rules, "source_pack_digest": sourceDigest, "input_digest": b.digest}
}

//nolint:gocyclo // Maintenance phases preserve the shared lock and publication recovery boundary.
func (s *Store) pending(repair bool) ([]map[string]any, error) {
	if err := checkDirectoryChain(s.config.Root, s.dataPath("observations")); err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	name := s.dataPath("observations", "pending.jsonl")
	data, err := regularFile(name)
	if os.IsNotExist(err) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	observations := []map[string]any{}
	var retained bytes.Buffer
	changed := false
	seen := map[string]bool{}
	for i, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		err := strictjson.Decode(line, &entry)
		if err == nil && textField(entry["id"]) == "" {
			err = fmt.Errorf("pending observation needs an id")
		}
		if err == nil && entry["context"] != nil {
			err = validateContext(entry["context"])
		}
		if err == nil && entry["safe_excerpt"] != nil {
			excerpt, ok := entry["safe_excerpt"].(string)
			privacy := sanitize(excerpt)
			if !ok || !privacy.Accepted || privacy.Excerpt != excerpt {
				err = fmt.Errorf("pending observation is not privacy-safe")
			}
		}
		if err != nil {
			if !repair {
				return nil, fmt.Errorf("invalid pending observation at line %d", i+1)
			}
			if err := ensureDirectory(s.dataPath("observations", "quarantine")); err != nil {
				return nil, err
			}
			if err := writeNewFile(s.dataPath("observations", "quarantine", fmt.Sprintf("%s-line-%d-%s.jsonl", s.config.Clock().UTC().Format("20060102T150405Z"), i+1, randomID())), append(append([]byte{}, line...), '\n')); err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		if seen[textField(entry["id"])] {
			return nil, fmt.Errorf("duplicate pending observation")
		}
		seen[textField(entry["id"])] = true
		observations = append(observations, entry)
		retained.Write(line)
		retained.WriteByte('\n')
	}
	if changed {
		if err := atomicReplace(name, retained.Bytes()); err != nil {
			return nil, err
		}
	}
	return observations, nil
}

//nolint:gocyclo // The canonical batch binds pending evidence, current rules, and an optional source snapshot.
func (s *Store) buildMaintenanceBatch(repair bool) (maintenanceBatch, *generation, error) {
	batch := maintenanceBatch{localDate: s.config.Clock().Local().Format("2006-01-02"), observations: []map[string]any{}}
	current, err := s.loadCurrent()
	if err != nil {
		return batch, nil, err
	}
	catalog, err := s.catalogFromGeneration(current)
	if err != nil {
		return batch, nil, err
	}
	batch.catalog = catalog
	batch.rules = cloneRules(catalog.rules)
	read := func() error {
		entries, err := s.pending(repair)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if current == nil || !slices.Contains(current.ProcessedIDs, textField(entry["id"])) {
				batch.observations = append(batch.observations, entry)
			}
		}
		return nil
	}
	if repair {
		err = withFileLock(s.dataPath("observations", ".pending.lock"), false, read)
	} else {
		err = read()
	}
	if err != nil {
		return batch, nil, err
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
	encoded, err := canonicalJSON(document)
	if err != nil {
		return batch, nil, err
	}
	batch.digest = digest(string(encoded))
	return batch, current, nil
}

//nolint:gocyclo // Maintenance phases preserve the shared lock and publication recovery boundary.
func (s *Store) maintain(mode string, proposal []byte) (map[string]any, error) {
	result := map[string]any{"status": "failed", "generation_id": nil, "input_digest": nil}
	if !slices.Contains([]string{"dry_run", "due", "force"}, mode) {
		return result, fmt.Errorf("invalid maintenance mode")
	}
	execute := func() error {
		state := readJSONFile(s.dataPath("state.json"))
		today := s.config.Clock().Local().Format("2006-01-02")
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		if mode != "dry_run" && current != nil {
			previous := record(current.Manifest["maintenance"])
			if textField(previous["local_date"]) != "" && previous["input_digest"] != state["last_maintenance_input_digest"] {
				if err := s.finishMaintenance(current, textField(previous["local_date"])); err != nil {
					return err
				}
				state = readJSONFile(s.dataPath("state.json"))
				if previous["local_date"] == today && len(proposal) == 0 {
					result["status"] = "success"
					result["generation_id"] = current.ID
					result["input_digest"] = previous["input_digest"]
					return nil
				}
			}
		}
		if mode == "due" && state["last_success_local_date"] == today {
			result["status"] = "already_complete"
			return nil
		}
		batch, current, err := s.buildMaintenanceBatch(mode != "dry_run")
		if err != nil {
			return err
		}
		result["input_digest"] = batch.digest
		if len(proposal) == 0 {
			result["status"] = "needs_proposal"
			result["batch"] = batch.document()
			files := map[string]any{}
			if batch.source != nil {
				for name, file := range batch.source.files {
					files[name] = map[string]any{"digest": file.Digest, "content": file.Content}
				}
			}
			result["source_pack_files"] = files
			if mode != "dry_run" {
				if err := ensureDirectory(s.dataPath("proposals")); err != nil {
					return err
				}
				name := s.dataPath("proposals", batch.digest+".json")
				encoded, err := canonicalJSON(result)
				if err != nil {
					return err
				}
				if err := atomicReplace(name, append(encoded, '\n')); err != nil {
					return err
				}
				result["proposal_request"] = name
			}
			return nil
		}
		validated, err := validateProposal(proposal, batch)
		if err != nil {
			return err
		}
		application, err := applyProposal(validated, batch)
		if err != nil {
			return err
		}
		if mode == "dry_run" {
			result["status"] = "dry_run"
			return nil
		}
		files, manifest := renderPersona(application.rules.rules)
		manifest["maintenance"] = map[string]any{"input_digest": batch.digest, "local_date": batch.localDate, "run_id": randomID(), "runner": "local_proposal", "action_counts": application.counts}
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
		id, err := s.commitLocked(generation{Rules: application.rules, ProcessedIDs: ids, SourcePacks: packs, PersonaFiles: files, Manifest: manifest})
		if err != nil {
			return err
		}
		if err := s.interrupt("maintenance_published"); err != nil {
			return err
		}
		published, err := s.loadCurrent()
		if err != nil {
			return err
		}
		if err := s.finishMaintenance(published, batch.localDate); err != nil {
			return err
		}
		result["status"] = "success"
		result["generation_id"] = id
		return nil
	}
	if mode == "dry_run" {
		err := execute()
		return result, err
	}
	err := withFileLock(s.dataPath(".maintenance.lock"), true, func() error { return s.withGenerationLock(execute) })
	if lockBusy(err) {
		result["status"] = "already_running"
		return result, nil
	}
	return result, err
}

func (s *Store) finishMaintenance(current *generation, localDate string) error {
	if err := s.refreshMirrors(current.PersonaFiles, current.SourcePacks, current); err != nil {
		return err
	}
	if err := s.compactPending(current.ProcessedIDs, localDate); err != nil {
		return err
	}
	if result := s.audit(false); result.Status != "success" {
		return fmt.Errorf("maintenance audit failed")
	}
	maintenance := record(current.Manifest["maintenance"])
	report := map[string]any{"schema_version": schemaVersion, "generation_id": current.ID, "input_digest": maintenance["input_digest"], "action_counts": maintenance["action_counts"], "runner": "local_proposal"}
	if err := ensureDirectory(s.dataPath("reports")); err != nil {
		return err
	}
	encoded, err := canonicalJSON(report)
	if err != nil {
		return err
	}
	if err := atomicReplace(s.dataPath("reports", current.ID+".json"), append(encoded, '\n')); err != nil {
		return err
	}
	if err := s.pruneRetention(current.ID); err != nil {
		return err
	}
	state := readJSONFile(s.dataPath("state.json"))
	state["schema_version"] = schemaVersion
	state["last_success_local_date"] = localDate
	state["last_success_at"] = s.config.Clock().UTC().Format(time.RFC3339)
	state["last_generation"] = current.ID
	state["last_maintenance_input_digest"] = maintenance["input_digest"]
	state["maintenance_runner"] = "local_proposal"
	encoded, err = canonicalJSON(state)
	if err != nil {
		return err
	}
	return atomicReplace(s.dataPath("state.json"), append(encoded, '\n'))
}

func (s *Store) compactPending(ids []string, localDate string) error {
	return withFileLock(s.dataPath("observations", ".pending.lock"), false, func() error {
		entries, err := s.pending(false)
		if err != nil {
			return err
		}
		if err := ensureDirectory(s.dataPath("observations", "archive")); err != nil {
			return err
		}
		archivePath := s.dataPath("observations", "archive", localDate[:7]+".jsonl")
		archived, err := regularFile(archivePath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		known := map[string]bool{}
		for _, line := range bytes.Split(archived, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var entry map[string]any
			if strictjson.Decode(line, &entry) != nil || textField(entry["id"]) == "" {
				return fmt.Errorf("invalid archive")
			}
			known[textField(entry["id"])] = true
		}
		var keep bytes.Buffer
		output := bytes.NewBuffer(append([]byte{}, archived...))
		for _, entry := range entries {
			encoded, err := canonicalJSON(entry)
			if err != nil {
				return err
			}
			if slices.Contains(ids, textField(entry["id"])) {
				if !known[textField(entry["id"])] {
					output.Write(encoded)
					output.WriteByte('\n')
					known[textField(entry["id"])] = true
				}
			} else {
				keep.Write(encoded)
				keep.WriteByte('\n')
			}
		}
		if err := atomicReplace(archivePath, output.Bytes()); err != nil {
			return err
		}
		if err := s.interrupt("maintenance_archived"); err != nil {
			return err
		}
		return atomicReplace(s.dataPath("observations", "pending.jsonl"), keep.Bytes())
	})
}

//nolint:gocyclo // Maintenance phases preserve the shared lock and publication recovery boundary.
func (s *Store) pruneRetention(currentID string) error {
	for _, dir := range []string{"logs", "runs", "generations"} {
		root := s.dataPath(dir)
		if err := checkDirectoryChain(s.config.Root, root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		days := 30
		if dir == "generations" {
			days = 14
		}
		prior := ""
		var priorTime time.Time
		if dir == "generations" {
			for _, entry := range entries {
				if !entry.IsDir() || entry.Name() == currentID || !generationIDPattern.MatchString(entry.Name()) {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if prior == "" || info.ModTime().After(priorTime) {
					prior = entry.Name()
					priorTime = info.ModTime()
				}
			}
		}
		for _, entry := range entries {
			if entry.Name() == currentID || entry.Name() == prior || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if dir == "generations" && (!entry.IsDir() || !generationIDPattern.MatchString(entry.Name())) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(s.config.Clock().Add(-time.Duration(days) * 24 * time.Hour)) {
				if dir == "generations" {
					if _, err := s.loadGeneration(filepath.Join(root, entry.Name()), entry.Name()); err != nil {
						return err
					}
				}
				if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
					return err
				}
			}
		}
		if err := syncDirectory(root); err != nil {
			return err
		}
	}
	return nil
}
