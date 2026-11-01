package preferences

import (
	"bytes"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

type auditResult struct {
	Status     string           `json:"status"`
	Errors     []string         `json:"errors"`
	Counts     map[string]any   `json:"counts"`
	Duplicates []map[string]any `json:"duplicates"`
}

func (s *Store) pendingCount() int {
	data, err := regularFile(s.dataPath("observations", "pending.jsonl"))
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			count++
		}
	}
	return count
}

//nolint:gocyclo // The audit reports every independent integrity failure in one read-only pass.
func (s *Store) audit(duplicates bool) auditResult {
	result := auditResult{Status: "failed", Errors: []string{}, Counts: map[string]any{}, Duplicates: []map[string]any{}}
	current, err := s.loadCurrent()
	if err != nil {
		result.Errors = append(result.Errors, "current_generation_invalid")
		return result
	}
	if current == nil {
		result.Errors = append(result.Errors, "no_current_generation")
		return result
	}
	add := func(code string) { result.Errors = append(result.Errors, code) }
	if err := validateRuleGraph(current.Rules.rules); err != nil {
		add("rule_graph_invalid")
	}
	files, _ := renderPersona(current.Rules.rules)
	if !sameValue(files, current.PersonaFiles) {
		add("persona_generation_stale")
	}
	for name, expected := range current.PersonaFiles {
		target := filepath.Join(s.config.Root, "persona", filepath.FromSlash(name))
		data, err := regularFile(target)
		if err != nil || checkDirectoryChain(s.config.Root, filepath.Dir(target)) != nil || string(data) != expected {
			add("persona_mirror_mismatch")
		}
		if path.Base(name) == "core.md" && estimatedTokens(expected) > 600 {
			add("core_budget_exceeded")
		}
	}
	data, err := regularFile(s.dataPath("source-packs.yml"))
	var mirror struct {
		SchemaVersion int            `yaml:"schema_version"`
		Packs         map[string]any `yaml:"packs"`
	}
	if err != nil || !bytes.HasPrefix(data, []byte(sourceMirrorHeader)) || decodeYAML(bytes.TrimPrefix(data, []byte(sourceMirrorHeader)), &mirror) != nil || mirror.SchemaVersion != schemaVersion || !sameValue(mirror.Packs, current.SourcePacks) {
		add("source_mirror_mismatch")
	}
	if current.SourcePacks["code-like-him"] != nil {
		source, err := s.snapshotSourcePack()
		if err != nil {
			add("source_pack_stale")
		} else {
			recorded := record(current.SourcePacks["code-like-him"])
			expected := record(source.metadata["code-like-him"])
			if recorded["digest"] != expected["digest"] || !sameValue(recorded["files"], expected["files"]) {
				add("source_pack_stale")
			}
			for _, r := range current.Rules.rules {
				if r["origin"] == "source_pack" && source.validateCitation(record(r["source_ref"])) != nil {
					add("source_pack_stale")
				}
			}
		}
	}
	data, err = regularFile(s.dataPath("observations", "pending.jsonl"))
	if err != nil {
		add("pending_observations_invalid")
	} else {
		seen := map[string]bool{}
		for _, line := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var entry map[string]any
			if strictjson.Decode(line, &entry) != nil || textField(entry["id"]) == "" || seen[textField(entry["id"])] {
				add("pending_observations_invalid")
				continue
			}
			seen[textField(entry["id"])] = true
			if excerpt, ok := entry["safe_excerpt"].(string); ok {
				privacy := sanitize(excerpt)
				if !privacy.Accepted || privacy.Excerpt != excerpt {
					add("pending_observations_invalid")
				}
			}
		}
	}
	groups := map[string][]string{}
	for _, r := range current.Rules.rules {
		status := textField(r["status"])
		count, _ := result.Counts[status].(int)
		result.Counts[status] = count + 1
		if duplicates {
			surfaces := sortedUnique(stringValues(r["surfaces"]))
			key, _ := canonicalJSON([]any{directiveKey(r), r["scope"], surfaces})
			groups[string(key)] = append(groups[string(key)], textField(r["id"]))
		}
	}
	for _, ids := range groups {
		if len(ids) > 1 {
			sort.Strings(ids)
			result.Duplicates = append(result.Duplicates, map[string]any{"ids": ids})
		}
	}
	sort.Slice(result.Duplicates, func(i, j int) bool {
		return strings.Join(stringValues(result.Duplicates[i]["ids"]), "\x00") < strings.Join(stringValues(result.Duplicates[j]["ids"]), "\x00")
	})
	result.Counts["processed"] = len(current.ProcessedIDs)
	result.Counts["pending"] = s.pendingCount()
	result.Errors = sortedUnique(result.Errors)
	if len(result.Errors) == 0 {
		result.Status = "success"
	}
	return result
}

func readJSONFile(name string) map[string]any {
	data, err := regularFile(name)
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	if strictjson.Decode(data, &result) != nil || result == nil {
		return map[string]any{}
	}
	return result
}

func (s *Store) status() (map[string]any, error) {
	current, err := s.loadCurrent()
	if err != nil {
		return nil, err
	}
	state := readJSONFile(s.dataPath("state.json"))
	last := state["last_success_local_date"]
	due := "due"
	if last == s.config.Clock().Local().Format("2006-01-02") {
		due = "complete_today"
	}
	cron, cronErr := readCrontab()
	result := map[string]any{"status": "no_generation", "pending_count": s.pendingCount(), "last_success_local_date": last, "next_due": due, "hook_installed": s.hookInstalled(), "cron_installed": cronErr == nil && strings.Count(cron, cronBegin) == 1 && strings.Count(cron, cronEnd) == 1, "hook_trust": "unknown", "hook_health": nil}
	if data, err := regularFile(s.dataPath("hook-health.json")); err == nil {
		var value any
		if strictjson.Decode(data, &value) == nil {
			result["hook_health"] = value
		}
	}
	if current == nil {
		return result, nil
	}
	counts := map[string]int{}
	for _, name := range []string{"active", "core", "candidate", "dormant", "conflicted", "superseded"} {
		counts[name] = 0
	}
	for _, r := range current.Rules.rules {
		counts[textField(r["status"])]++
	}
	result["status"] = "ok"
	result["generation"] = current.ID
	result["rule_counts"] = counts
	result["source_pack_digest"] = nil
	result["source_pack_changed"] = nil
	if source, err := s.snapshotSourcePack(); err == nil {
		result["source_pack_digest"] = source.digest
		result["source_pack_changed"] = source.digest != record(current.SourcePacks["code-like-him"])["digest"]
	}
	return result, nil
}
