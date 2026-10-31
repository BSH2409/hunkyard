package preferences

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirrorInterruptionPreservesPublicationAndRollback(t *testing.T) {
	s := testStore(t)
	first := emptyGeneration()
	first.Manifest["generation_id"] = "g-first"
	if _, err := s.commitGeneration(first); err != nil {
		t.Fatal(err)
	}
	sourcePackFixture(t, s)
	snapshot, err := s.snapshotSourcePack()
	if err != nil {
		t.Fatal(err)
	}
	original := syncDirectory
	t.Cleanup(func() { syncDirectory = original })
	root := filepath.Join(s.config.Root, "persona")
	syncs := 0
	syncDirectory = func(name string) error {
		if name == root {
			syncs++
			if syncs == 2 {
				return errors.New("injected mid-mirror failure")
			}
		}
		return original(name)
	}
	next := emptyGeneration()
	next.Manifest["generation_id"] = "g-next"
	next.PersonaFiles["ruby.md"] = generatedHeader + "\n- New guidance.\n"
	next.SourcePacks = snapshot.metadata
	if _, err := s.commitGeneration(next); err != nil || s.mirrorError == nil {
		t.Fatal("mid-mirror failure lost published generation or repair status", err)
	}
	current, err := s.loadCurrent()
	if err != nil || current.ID != "g-next" {
		t.Fatal("mirror interruption changed current", err)
	}
	data, err := regularFile(s.dataPath("source-packs.yml"))
	if err != nil || !strings.Contains(string(data), snapshot.digest) {
		t.Fatal("persona failure prevented source mirror refresh", err)
	}
	staging, err := filepath.Glob(s.dataPath(".persona-mirror-*"))
	if err != nil || len(staging) != 0 {
		t.Fatal("interruption leaked mirror staging", err)
	}
	previousError := s.mirrorError
	s.fault = func(stage string) error {
		if stage == "before_swap" {
			return errors.New("injected pre-swap failure")
		}
		return nil
	}
	next.Manifest["generation_id"] = "g-failed"
	if _, err := s.commitGeneration(next); err == nil || !errors.Is(s.mirrorError, previousError) {
		t.Fatal("failed pre-swap commit erased prior repair state")
	}
	s.fault = nil
	syncDirectory = func(name string) error {
		if name == root {
			return errors.New("injected rollback mirror failure")
		}
		return original(name)
	}
	if err := s.rollback("g-first"); err != nil || s.mirrorError == nil {
		t.Fatal("rollback mirror failure lost selected generation", err)
	}
	current, err = s.loadCurrent()
	if err != nil || current.ID != "g-first" {
		t.Fatal("rollback did not preserve selected generation", err)
	}
	syncDirectory = original
	if err := s.repairMirrors(); err != nil || s.mirrorError != nil {
		t.Fatal("interrupted mirror could not be repaired", err)
	}
}

func TestPersonaMirrorSyncsRenamedAndRemovedParents(t *testing.T) {
	s := testStore(t)
	first := emptyGeneration()
	first.PersonaFiles["old/ruby.md"] = generatedHeader + "\n- Old guidance.\n"
	if _, err := s.commitGeneration(first); err != nil {
		t.Fatal(err)
	}
	original := syncDirectory
	t.Cleanup(func() { syncDirectory = original })
	synced := map[string]int{}
	syncDirectory = func(name string) error { synced[name]++; return original(name) }
	next := emptyGeneration()
	next.PersonaFiles["new/testing.md"] = generatedHeader + "\n- Test guidance.\n"
	if _, err := s.commitGeneration(next); err != nil || s.mirrorError != nil {
		t.Fatal("mirror update failed", err)
	}
	root := filepath.Join(s.config.Root, "persona")
	for _, dir := range []string{root, filepath.Join(root, "old"), filepath.Join(root, "new")} {
		if synced[dir] == 0 {
			t.Errorf("mirror parent was not synced: %s", dir)
		}
	}
}

func TestGenerationSourceCitationsAndRollback(t *testing.T) {
	s := testStore(t)
	root := sourcePackFixture(t, s)
	snapshot, err := s.snapshotSourcePack()
	if err != nil {
		t.Fatal(err)
	}
	r := ruleFixture("global")
	r["origin"], r["source_pack"], r["load_tier"] = "source_pack", "code-like-him", "core"
	r["source_ref"] = map[string]any{"file": "SKILL.md", "digest": snapshot.files["SKILL.md"].Digest}
	c := testCatalog(t, s, r)
	files, manifest := renderPersona(c.rules)
	next := generation{Rules: c.ruleSet(), ProcessedIDs: []string{}, SourcePacks: snapshot.metadata, PersonaFiles: files, Manifest: manifest}
	id, err := s.commitGeneration(next)
	if err != nil {
		t.Fatal("exact source citation rejected", err)
	}
	current, err := s.loadCurrent()
	if err != nil || current.Rules.rules[0]["status"] != "active" {
		t.Fatal("source core default needs invented personal evidence", err)
	}
	data, err := regularFile(s.dataPath("source-packs.yml"))
	if err != nil || !strings.HasPrefix(string(data), sourceMirrorHeader) || strings.Contains(string(data), root) || strings.Contains(string(data), "Keep contracts") {
		t.Fatal("source mirror disclosed source prose or paths", err)
	}
	for _, kind := range []string{"missing", "stale", "unknown_file", "wrong_pack"} {
		t.Run(kind, func(t *testing.T) {
			next := generation{Rules: c.ruleSet(), ProcessedIDs: []string{}, SourcePacks: cloneMap(snapshot.metadata), PersonaFiles: files, Manifest: map[string]any{"fixture": kind}}
			switch kind {
			case "missing":
				next.SourcePacks = map[string]any{}
			case "stale":
				record(next.Rules.rules[0]["source_ref"])["digest"] = strings.Repeat("f", 64)
			case "unknown_file":
				record(next.Rules.rules[0]["source_ref"])["file"] = "reference/unknown.md"
			case "wrong_pack":
				next.Rules.rules[0]["source_pack"] = "unknown"
			}
			if _, err := s.commitGeneration(next); err == nil {
				t.Fatal("invalid transaction citation accepted")
			}
			current, err := s.loadCurrent()
			if err != nil || current.ID != id {
				t.Fatal("invalid citation changed current", err)
			}
		})
	}
	empty := emptyGeneration()
	empty.Rules = testCatalog(t, s).ruleSet()
	if _, err := s.commitGeneration(empty); err != nil {
		t.Fatal(err)
	}
	if err := s.rollback(id); err != nil || s.mirrorError != nil {
		t.Fatal("source rollback failed", err)
	}
	after, err := regularFile(s.dataPath("source-packs.yml"))
	if err != nil || string(after) != string(data) {
		t.Fatal("rollback did not restore source mirror", err)
	}
	if _, err := c.upsertExplicit(observationFixture("personal", "session-one", "alpha-a1b2c3d4e5f6"), "Prefer personal service boundaries.", map[string]any{"kind": "global", "repo_key": nil}, []string{"services"}); err != nil {
		t.Fatal(err)
	}
	if c.orderedActive()[0]["origin"] != "feedback" {
		t.Fatal("personal preference did not outrank source defaults")
	}
}

func TestGenerationRejectsResignedInvalidArtifacts(t *testing.T) {
	for _, kind := range []string{"malformed_yaml", "alias_yaml", "tagged_yaml", "wrong_yaml_version", "wrong_manifest_version", "malformed_manifest", "manifest_path_type", "unsorted_paths", "duplicate_paths", "missing_persona", "persona_digest", "symlink_persona", "symlink_generation", "parent_collision"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			next := emptyGeneration()
			next.PersonaFiles["ruby.md"] = generatedHeader + "\n- Ruby guidance.\n"
			if kind == "parent_collision" {
				next.PersonaFiles["ruby.md/nested.md"] = generatedHeader
				if _, err := s.commitGeneration(next); err == nil {
					t.Fatal("completed candidate with file/parent collision published")
				}
				if current, err := s.loadCurrent(); err != nil || current != nil {
					t.Fatal("failed candidate changed current", err)
				}
				return
			}
			id, err := s.commitGeneration(next)
			if err != nil {
				t.Fatal(err)
			}
			dir := s.dataPath("generations", id)
			manifest := readJSONFile(filepath.Join(dir, "manifest.json"))
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.Chmod(path, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			updateManifest := true
			switch kind {
			case "malformed_yaml", "alias_yaml", "tagged_yaml", "wrong_yaml_version":
				text := map[string]string{"malformed_yaml": "rules: [", "alias_yaml": "schema_version: 1\nrules: &rules [*rules]\n", "tagged_yaml": "schema_version: 1\nrules: !ruby/object []\n", "wrong_yaml_version": "schema_version: 2\nrules: []\n"}[kind]
				write(filepath.Join(dir, "rules.yml"), []byte(text))
				record(manifest["files"])["rules.yml"] = digest(text)
			case "wrong_manifest_version":
				manifest["schema_version"] = 2
			case "malformed_manifest":
				write(filepath.Join(dir, "manifest.json"), []byte("not JSON"))
				updateManifest = false
			case "manifest_path_type":
				manifest["persona_files"] = "core.md"
			case "unsorted_paths":
				manifest["persona_files"] = []string{"ruby.md", "core.md"}
			case "duplicate_paths":
				manifest["persona_files"] = []string{"core.md", "core.md", "ruby.md"}
			case "missing_persona":
				manifest["persona_files"] = []string{"core.md", "missing.md", "ruby.md"}
			case "persona_digest":
				write(filepath.Join(dir, "persona", "ruby.md"), []byte("tampered"))
			case "symlink_persona":
				path := filepath.Join(dir, "persona", "ruby.md")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), path); err != nil {
					t.Skip("symlink privilege unavailable")
				}
			case "symlink_generation":
				outside := filepath.Join(t.TempDir(), "generation")
				if err := os.Rename(dir, outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, dir); err != nil {
					t.Skip("symlink privilege unavailable")
				}
				updateManifest = false
			}
			if updateManifest {
				data, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				write(filepath.Join(dir, "manifest.json"), data)
			}
			if _, err := s.loadCurrent(); err == nil {
				t.Fatal("invalid generation accepted after checksums were updated")
			}
		})
	}
}

func TestGenerationAuthorityBoundary(t *testing.T) {
	s := testStore(t)
	c := testCatalog(t, s, ruleFixture("repository"))
	next := emptyGeneration()
	next.Rules = c.ruleSet()
	// Even authorized input cannot persist caller-supplied derived counters.
	next.Rules.rules[0]["status"] = "core"
	next.Rules.rules[0]["evidence"] = map[string]any{"explicit_count": 999, "observation_count": 999, "repository_count": 999, "session_count": 999}
	first, err := s.commitGeneration(next)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.loadCurrent()
	if err != nil || current.Rules.rules[0]["status"] != "candidate" {
		t.Fatal("forged derived counters persisted", err)
	}
	if count, ok := integer(record(current.Rules.rules[0]["evidence"])["observation_count"]); !ok || count != 0 {
		t.Fatal("forged evidence persisted")
	}
	if _, err := s.catalogFromGeneration(&generation{Rules: validatedRuleSet{rules: []rule{ruleFixture("global")}}}); err == nil {
		t.Fatal("untrusted catalog seed accepted")
	}
	for _, kind := range []string{"raw_changed", "foreign_issuer", "unknown_rule_field", "unknown_pack_field"} {
		t.Run(kind, func(t *testing.T) {
			next := emptyGeneration()
			next.Rules = current.Rules
			next.Rules.rules = cloneRules(current.Rules.rules)
			switch kind {
			case "raw_changed":
				next.Rules.issuer = nil
				next.Rules.rules[0]["directive"] = "Prefer forged guidance."
			case "foreign_issuer":
				next.Rules.issuer = testStore(t)
				next.Rules.rules[0]["directive"] = "Prefer forged guidance."
			case "unknown_rule_field":
				next.Rules.rules[0]["model_note"] = "not canonical"
			case "unknown_pack_field":
				next.SourcePacks = map[string]any{"code-like-him": map[string]any{"digest": strings.Repeat("a", 64), "files": map[string]any{"SKILL.md": strings.Repeat("b", 64)}, "refreshed_at": "2026-08-19T12:00:00Z", "content": "must not persist"}}
			}
			if _, err := s.commitGeneration(next); err == nil {
				t.Fatal("forged generation accepted")
			}
			after, err := s.loadCurrent()
			if err != nil || after.ID != first {
				t.Fatal("rejected commit changed current", err)
			}
		})
	}
	next = emptyGeneration()
	next.Rules = validatedRuleSet{rules: cloneRules(current.Rules.rules)}
	next.Manifest["fixture"] = "raw unchanged round trip"
	if _, err := s.commitGeneration(next); err != nil {
		t.Fatal("unchanged validated rules cannot round trip", err)
	}
	current, err = s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.catalogFromGeneration(current)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.confirm("rule-1", observationFixture("obs-confirm", "session-one", "alpha-a1b2c3d4e5f6"))
	if err != nil {
		t.Fatal(err)
	}
	issued := c.ruleSet()
	r["directive"] = "Caller mutation"
	next = emptyGeneration()
	next.Rules = issued
	if _, err := s.commitGeneration(next); err != nil {
		t.Fatal("validated catalog action could not commit", err)
	}
}

func TestGenerationMirrorIndependenceAndRepair(t *testing.T) {
	for _, kind := range []string{"source", "persona", "both", "unsigned_source"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			if _, err := s.commitGeneration(emptyGeneration()); err != nil {
				t.Fatal(err)
			}
			persona := filepath.Join(s.config.Root, "persona", "core.md")
			source := s.dataPath("source-packs.yml")
			if kind == "source" || kind == "both" || kind == "unsigned_source" {
				content := "manual source\n"
				if kind == "unsigned_source" {
					content = "schema_version: 1\npacks: {}\n"
				}
				if err := os.WriteFile(source, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "persona" || kind == "both" {
				if err := os.WriteFile(persona, []byte("manual persona\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			next := emptyGeneration()
			next.PersonaFiles["core.md"] += "- New guidance.\n"
			next.Manifest["fixture"] = kind
			id, err := s.commitGeneration(next)
			if err != nil || s.mirrorError == nil {
				t.Fatal("mirror failure lost publication or repair status", err)
			}
			current, err := s.loadCurrent()
			if err != nil || current.ID != id {
				t.Fatal("mirror failure rolled back publication", err)
			}
			data, err := regularFile(persona)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "source" || kind == "unsigned_source" {
				if string(data) != next.PersonaFiles["core.md"] {
					t.Fatal("source failure prevented persona refresh")
				}
			} else if string(data) != "manual persona\n" {
				t.Fatal("manual persona overwritten")
			}
			if kind == "both" && (!strings.Contains(s.mirrorError.Error(), "persona") || !strings.Contains(s.mirrorError.Error(), "source-pack")) {
				t.Fatal("simultaneous mirror errors not aggregated")
			}
			if err := os.Remove(persona); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			if err := s.repairMirrors(); err != nil || s.mirrorError != nil {
				t.Fatal("mirror repair failed", err)
			}
			if data, err := regularFile(persona); err != nil || string(data) != next.PersonaFiles["core.md"] {
				t.Fatal("repair restored wrong persona", err)
			}
		})
	}
}

func TestPersonaMirrorConfinementAndManagedRemoval(t *testing.T) {
	for _, kind := range []string{"symlink_file", "symlink_parent", "symlink_root", "file_parent", "file_root"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			if _, err := s.commitGeneration(emptyGeneration()); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(s.config.Root, "persona")
			outside := t.TempDir()
			path := filepath.Join(root, "nested")
			next := emptyGeneration()
			next.PersonaFiles["nested/ruby.md"] = generatedHeader + "\n- Ruby guidance.\n"
			next.Manifest["fixture"] = kind
			if kind == "symlink_file" {
				path = filepath.Join(root, "core.md")
				outside = filepath.Join(outside, "outside.md")
				if err := os.WriteFile(outside, []byte("keep outside"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasSuffix(kind, "root") {
				path = root
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(kind, "symlink") {
				if err := os.Symlink(outside, path); err != nil {
					t.Skip("symlink privilege unavailable")
				}
			} else if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.commitGeneration(next); err != nil || s.mirrorError == nil {
				t.Fatal("unsafe mirror did not request repair", err)
			}
			if kind == "symlink_file" {
				if data, err := regularFile(outside); err != nil || string(data) != "keep outside" {
					t.Fatal("mirror changed symlink target", err)
				}
			} else if strings.HasPrefix(kind, "symlink") {
				entries, err := os.ReadDir(outside)
				if err != nil || len(entries) != 0 {
					t.Fatal("mirror escaped root", err)
				}
			}
		})
	}
	s := testStore(t)
	first := emptyGeneration()
	first.PersonaFiles["nested/ruby.md"] = generatedHeader + "\n- Old.\n"
	if _, err := s.commitGeneration(first); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(s.config.Root, "persona", "notes.md")
	if err := os.WriteFile(notes, []byte("hand-written\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.commitGeneration(emptyGeneration()); err != nil || s.mirrorError != nil {
		t.Fatal("managed removal failed", err)
	}
	if _, err := os.Stat(filepath.Join(s.config.Root, "persona", "nested", "ruby.md")); !os.IsNotExist(err) {
		t.Fatal("obsolete managed file retained")
	}
	if data, err := regularFile(notes); err != nil || string(data) != "hand-written\n" {
		t.Fatal("unknown manual file changed", err)
	}
}

func TestCurrentGenerationLinkTarget(t *testing.T) {
	id := "gen_" + strings.Repeat("a", 16)
	if !generationIDPattern.MatchString(id) {
		t.Skip("fixture id does not match the generation pattern")
	}
	tests := []struct {
		target string
		ok     bool
	}{
		{"generations/" + id, true},
		{`generations\` + id, true}, // os.Readlink on Windows
		{"generations/../" + id, false},
		{"/abs/generations/" + id, false},
		{"other/" + id, false},
		{"generations/" + id + "/x", false},
	}
	for _, tt := range tests {
		got, ok := currentGenerationID(tt.target)
		if ok != tt.ok || ok && got != id {
			t.Errorf("currentGenerationID(%q) = %q, %v; want ok=%v", tt.target, got, ok, tt.ok)
		}
	}
}

func TestOpenRefusesUnsupportedPlatform(t *testing.T) {
	previous := SupportedPlatform
	SupportedPlatform = false
	t.Cleanup(func() { SupportedPlatform = previous })
	if _, err := Open(Config{Root: t.TempDir()}); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Open on an unsupported platform: %v", err)
	}
}
