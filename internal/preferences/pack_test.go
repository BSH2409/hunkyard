package preferences

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writePack lays out a team pack: SKILL.md listing one reference file per
// surface, and pack.json with the content digest.
func writePack(t *testing.T, dir, name, version string, bySurface map[string][]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "reference"), 0o755); err != nil {
		t.Fatal(err)
	}
	var refs strings.Builder
	for surface, bullets := range bySurface {
		refs.WriteString("- [" + surface + "](reference/" + surface + ".md)\n")
		var body strings.Builder
		body.WriteString("# " + surface + "\n\n")
		for _, b := range bullets {
			body.WriteString("- " + b + "\n")
		}
		if err := os.WriteFile(filepath.Join(dir, "reference", surface+".md"), []byte(body.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	skill := "---\nname: " + name + "\ndescription: team conventions\n---\n\n# Conventions\n\n## References\n\n" + refs.String()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := readPackDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(packManifest{SchemaVersion: 1, Name: name, Version: version, CreatedAt: "2026-11-01T00:00:00Z", Digest: content.digest})
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
}

func packRules(s *Store, name string) []rule {
	current, _ := s.loadCurrent()
	var out []rule
	if current == nil {
		return out
	}
	for _, r := range current.Rules.rules {
		if r["pack"] == name {
			out = append(out, r)
		}
	}
	return out
}

func TestImportPackCreatesTeamRules(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids.", "Validate request bodies at the handler."}, "testing": {"Prefer table tests in Go."}})
	result, err := s.importPack(dir, packTarget{scope: "global"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "imported" || result.Pack != "shared-backend" || result.Added != 3 {
		t.Fatalf("result = %+v", result)
	}
	rules := packRules(s, "shared-backend")
	if len(rules) != 3 || rules[0]["origin"] != "team_pack" || rules[0]["status"] != "active" {
		t.Fatalf("rules = %+v", rules)
	}
	if got := selectPersona(rules, "add an api endpoint", ""); !strings.Contains(got.Text, "Return 404 for unknown ids.") {
		t.Fatalf("team rule not selected:\n%s", got.Text)
	}
	listed := s.listPacks()
	if len(listed) != 1 || listed[0].Name != "shared-backend" || listed[0].Rules != 3 || listed[0].Version != "1.0.0" {
		t.Fatalf("list = %+v", listed)
	}
	again, err := s.importPack(dir, packTarget{scope: "global"}, false)
	if err != nil || again.Status != "unchanged" {
		t.Fatalf("re-import = %+v %v", again, err)
	}
}

func TestImportPackNewVersionReplacesRules(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids.", "Old rule to drop."}})
	if _, err := s.importPack(dir, packTarget{scope: "global"}, false); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	writePack(t, dir, "backend", "1.1.0", map[string][]string{"api": {"Return 404 for unknown ids.", "Log request ids."}})
	result, err := s.importPack(dir, packTarget{scope: "global"}, false)
	if err != nil || result.Status != "updated" || result.Added != 1 || result.Removed != 1 || result.Kept != 1 {
		t.Fatalf("update = %+v %v", result, err)
	}
	for _, r := range packRules(s, "shared-backend") {
		if r["directive"] == "Old rule to drop." {
			t.Fatal("dropped directive survived the update")
		}
	}
}

func TestImportPackRejectsTamperedOrEscapingPacks(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids."}})
	if err := os.WriteFile(filepath.Join(dir, "reference", "api.md"), []byte("- Changed after export.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.importPack(dir, packTarget{scope: "global"}, false); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered pack accepted: %v", err)
	}
	escaping := t.TempDir()
	if err := os.WriteFile(filepath.Join(escaping, "SKILL.md"), []byte("## References\n\n- [x](../secrets.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.importPack(escaping, packTarget{scope: "repository", repoKey: "orion-0123456789ab", repository: true}, false); err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("escaping reference accepted: %v", err)
	}
	if current, _ := s.loadCurrent(); current != nil {
		t.Fatal("a rejected pack created a generation")
	}
}

func TestImportPackSkipsWhatItCannotUse(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Prefer table tests in Go.", "testing")
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{
		"testing": {"Prefer table tests in Go.", "Set password=hunter2 in fixtures."},
		"notreal": {"Ignored surface."},
	})
	result, err := s.importPack(dir, packTarget{scope: "global"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 0 || result.SkippedPersonal != 1 || result.SkippedInvalid != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestImportPackDryRunChangesNothing(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids."}})
	result, err := s.importPack(dir, packTarget{scope: "global"}, true)
	if err != nil || result.Status != "dry_run" || result.Added != 1 {
		t.Fatalf("dry run = %+v %v", result, err)
	}
	if current, _ := s.loadCurrent(); current != nil || len(s.listPacks()) != 0 {
		t.Fatal("dry run wrote to the store")
	}
}

func TestRepositoryPackImport(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "ignored-name", "1", map[string][]string{"database": {"Add an index for every foreign key."}})
	if err := os.Remove(filepath.Join(dir, "pack.json")); err != nil {
		t.Fatal(err)
	}
	result, err := s.importPack(dir, packTarget{scope: "repository", repoKey: "orion-0123456789ab", repository: true}, false)
	if err != nil || result.Pack != "repo-orion-0123456789ab" {
		t.Fatalf("repo import = %+v %v", result, err)
	}
	rules := packRules(s, "repo-orion-0123456789ab")
	if len(rules) != 1 || record(rules[0]["scope"])["repo_key"] != "orion-0123456789ab" {
		t.Fatalf("repo rules = %+v", rules)
	}
}

func TestRemovePack(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids."}})
	if _, err := s.importPack(dir, packTarget{scope: "global"}, false); err != nil {
		t.Fatal(err)
	}
	captureRule(t, s, "Name handlers by resource.", "api")
	result, err := s.removePack("shared-backend")
	if err != nil || result.Removed != 1 {
		t.Fatalf("remove = %+v %v", result, err)
	}
	if len(packRules(s, "shared-backend")) != 0 || len(s.listPacks()) != 0 {
		t.Fatal("pack rules or registry entry survived")
	}
	if _, err := s.removePack("shared-backend"); err == nil {
		t.Fatal("removing an unknown pack succeeded")
	}
	if _, err := s.removePack("code-like-him"); err == nil {
		t.Fatal("the source pack was removable")
	}
}

func TestSessionStartSyncsTheRepositoryPack(t *testing.T) {
	s := testStore(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	key, err := RepositoryKey(repo)
	if err != nil || key == "" {
		t.Fatalf("repository key: %q %v", key, err)
	}
	packDir := filepath.Join(repo, filepath.FromSlash(RepositoryPackDir))
	writePack(t, packDir, "ignored", "1", map[string][]string{"database": {"Add an index for every foreign key."}})
	sessionStartResponse(t, s, repo)
	if got := packRules(s, "repo-"+key); len(got) != 1 {
		t.Fatalf("pack not imported at session start: %+v", got)
	}
	sessionStartResponse(t, s, repo)
	if got := packRules(s, "repo-"+key); len(got) != 1 {
		t.Fatalf("second session start changed the pack: %+v", got)
	}
	if err := os.RemoveAll(packDir); err != nil {
		t.Fatal(err)
	}
	sessionStartResponse(t, s, repo)
	if got := packRules(s, "repo-"+key); len(got) != 0 {
		t.Fatalf("deleted pack still applies: %+v", got)
	}
}

func TestRemovedRepositoryPackStaysDeclinedUntilItChanges(t *testing.T) {
	s := testStore(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	key, _ := RepositoryKey(repo)
	packDir := filepath.Join(repo, filepath.FromSlash(RepositoryPackDir))
	writePack(t, packDir, "ignored", "1", map[string][]string{"database": {"Add an index for every foreign key."}})
	sessionStartResponse(t, s, repo)
	if _, err := s.removePack("repo-" + key); err != nil {
		t.Fatal(err)
	}
	sessionStartResponse(t, s, repo)
	if got := packRules(s, "repo-"+key); len(got) != 0 {
		t.Fatalf("a declined repository pack came back at session start: %+v", got)
	}
	if listed := s.listPacks(); len(listed) != 1 || !listed[0].Declined {
		t.Fatalf("list does not show the decline: %+v", listed)
	}
	if err := os.RemoveAll(packDir); err != nil {
		t.Fatal(err)
	}
	writePack(t, packDir, "ignored", "2", map[string][]string{"database": {"Add an index for every foreign key.", "Name migrations by intent."}})
	sessionStartResponse(t, s, repo)
	if got := packRules(s, "repo-"+key); len(got) != 2 {
		t.Fatalf("a changed pack was not offered again: %+v", got)
	}
}
