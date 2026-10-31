package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourcePackFixture(t *testing.T, s *Store) string {
	t.Helper()
	root := s.dataPath("sources", "code-like-him")
	copyFixtureTree(t, "testdata/contracts/source-pack", root)
	return root
}

func TestSourcePackReferenceParity(t *testing.T) {
	cases := []string{
		"test_snapshot_imports_only_sorted_allowlisted_reference_files", "test_snapshot_digest_is_deterministic_and_changes_with_a_referenced_file", "test_unreferenced_metadata_does_not_change_the_snapshot", "test_reference_section_ends_at_the_next_level_two_heading",
		"test_absolute_reference_is_rejected", "test_parent_traversal_reference_is_rejected", "test_missing_reference_is_rejected", "test_symlink_that_escapes_the_source_root_is_rejected",
		"test_citation_requires_an_allowlisted_path_and_current_digest", "test_citation_becomes_stale_when_the_source_file_changes", "test_metadata_contains_only_digest_manifest_and_refresh_time",
		"test_rule_validation_accepts_cross_cutting_core_and_surface_specific_rules", "test_rule_validation_rejects_wrong_pack_stale_citation_and_non_active_import", "test_rule_validation_requires_surface_tier_for_single_surface_guidance",
		"test_rule_validation_accepts_aligned_negative_guidance_about_the_discouraged_habit", "test_rule_validation_rejects_positive_advocacy_for_the_discouraged_habit", "test_rule_validation_uses_the_nearest_positive_controller_for_a_discouraged_phrase", "test_rule_validation_honors_an_immediate_contrastive_negation",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			root := sourcePackFixture(t, s)
			snapshot, err := s.snapshotSourcePack()
			if err != nil {
				t.Fatal(err)
			}
			write := func(name, text string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			citation := func(name string) map[string]any {
				return map[string]any{"file": name, "digest": snapshot.files[name].Digest}
			}
			rule := func(directive, tier string, surfaces []string) map[string]any {
				return map[string]any{"origin": "source_pack", "source_pack": "code-like-him", "source_ref": citation("SKILL.md"), "status": "active", "directive": directive, "load_tier": tier, "surfaces": surfaces, "scope": map[string]any{"kind": "global", "repo_key": nil}}
			}
			switch name {
			case cases[0], cases[10]:
				var expected map[string]any
				readContract(t, "source-pack-metadata.json", &expected)
				if !sameValue(snapshot.metadata, expected) {
					t.Fatal("source digest manifest differs from reference")
				}
				if len(snapshot.files) != 4 {
					t.Fatal("unlinked files included")
				}
				for name, file := range snapshot.files {
					data, err := os.ReadFile(filepath.Join(root, name))
					if err != nil || file.Content != string(data) || !sha256Pattern.MatchString(file.Digest) {
						t.Fatal("source bytes differ", err)
					}
				}
				encoded, _ := canonicalJSON(snapshot.metadata)
				if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "Keep contracts") {
					t.Fatal("source prose or paths leaked into metadata")
				}
			case cases[1], cases[2], cases[3]:
				again, err := s.snapshotSourcePack()
				if err != nil || again.digest != snapshot.digest {
					t.Fatal("non-deterministic source digest", err)
				}
				if name == cases[1] {
					write("reference/testing-posture.md", "Changed guidance\n")
				} else {
					write("_meta/confidence-report.md", "Not imported\n")
				}
				again, err = s.snapshotSourcePack()
				if err != nil {
					t.Fatal(err)
				}
				if (again.digest != snapshot.digest) != (name == cases[1]) {
					t.Fatal("digest included wrong files")
				}
			case cases[4], cases[5], cases[6]:
				path := map[string]string{cases[4]: "https://example.invalid/reference.md", cases[5]: "../outside.md", cases[6]: "reference/missing.md"}[name]
				write("SKILL.md", "## References\n[Invalid]("+path+")\n")
				if _, err := s.snapshotSourcePack(); err == nil {
					t.Fatal("unsafe or missing source accepted")
				}
			case cases[7]:
				outside := filepath.Join(t.TempDir(), "outside.md")
				if err := os.WriteFile(outside, []byte("Outside\n"), 0600); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(root, "reference/testing-posture.md")
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, target); err != nil {
					t.Skip("symlink privilege unavailable")
				}
				if _, err := s.snapshotSourcePack(); err == nil {
					t.Fatal("symlink escaped source root")
				}
			case cases[8]:
				if err := snapshot.validateCitation(citation("reference/testing-posture.md")); err != nil {
					t.Fatal(err)
				}
				for _, ref := range []map[string]any{{"file": "reference/testing-posture.md", "digest": strings.Repeat("0", 64)}, {"file": "reference/missing.md", "digest": snapshot.files["SKILL.md"].Digest}, {"file": "../outside.md", "digest": snapshot.files["SKILL.md"].Digest}} {
					if err := snapshot.validateCitation(ref); err == nil {
						t.Fatal("forged citation accepted")
					}
				}
			case cases[9]:
				write("reference/testing-posture.md", "New guidance\n")
				if err := snapshot.validateCitation(citation("reference/testing-posture.md")); err == nil {
					t.Fatal("stale source citation accepted")
				}
			case cases[11]:
				for _, r := range []map[string]any{rule("Keep contracts explicit at system boundaries.", "core", []string{"architecture", "communication"}), rule("Keep Rails controllers HTTP-focused.", "surface", []string{"rails"})} {
					if err := snapshot.validateProposedRule(r); err != nil {
						t.Fatal(err)
					}
				}
			case cases[12]:
				for _, mutate := range []func(map[string]any){func(r map[string]any) { r["source_pack"] = "other" }, func(r map[string]any) {
					r["source_ref"] = map[string]any{"file": "SKILL.md", "digest": strings.Repeat("0", 64)}
				}, func(r map[string]any) { r["status"] = "candidate" }} {
					r := rule("Keep contracts explicit.", "core", []string{"architecture", "communication"})
					mutate(r)
					if err := snapshot.validateProposedRule(r); err == nil {
						t.Fatal("invalid imported rule accepted")
					}
				}
			case cases[13]:
				if err := snapshot.validateProposedRule(rule("Keep Rails controllers HTTP-focused.", "core", []string{"rails"})); err == nil {
					t.Fatal("single-surface core accepted")
				}
			default:
				directives := map[string][]string{cases[14]: {"Do not use vague commit messages.", "Avoid vague commit messages.", "Never write vague commit messages.", "Vague commit messages should not be used."}, cases[15]: {"Prefer vague commit messages.", "Use uninformative commit messages.", "Write vague commit messages.", "Favor uninformative commit messages.", "Choose vague commit messages.", "Vague commit messages should be used.", "Uninformative commit messages are preferred.", "Avoid delays. Vague commit messages should be used."}, cases[16]: {"Avoid delays and use vague commit messages."}, cases[17]: {"Use detailed, not vague commit messages."}}[name]
				for _, directive := range directives {
					err := snapshot.validateProposedRule(rule(directive, "surface", []string{"communication"}))
					accepted := name == cases[14] || name == cases[17]
					if (err == nil) != accepted {
						t.Fatal("discouraged habit polarity differs", err)
					}
				}
			}
		})
	}
}

func TestSourceSnapshotAllowlistAndConfinement(t *testing.T) {
	s := testStore(t)
	root := s.dataPath("sources", "code-like-him")
	if err := os.MkdirAll(filepath.Join(root, "reference"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("SKILL.md", "# Source fixture\n[Ignored link](https://example.invalid)\n## References\n- [Naming](reference/naming.md)\n## Other\n[Not imported](outside.md)\n")
	write("reference/naming.md", "Prefer clear domain names.\n")
	write("outside.md", "Not allowlisted.\n")
	snapshot, err := s.snapshotSourcePack()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.files) != 2 || snapshot.files["outside.md"].Path != "" {
		t.Fatal("source snapshot included unlinked files")
	}
	if err := snapshot.revalidate(); err != nil {
		t.Fatal(err)
	}
	write("reference/naming.md", "Changed source instructions.\n")
	if err := snapshot.revalidate(); err == nil {
		t.Fatal("stale captured source authority accepted")
	}
	outside := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(outside, []byte("Outside source root"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "reference/naming.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "reference/naming.md")); err != nil {
		t.Skip("symlink privileges unavailable")
	}
	if _, err := s.snapshotSourcePack(); err == nil {
		t.Fatal("source reference escaped its trusted root")
	}
}
