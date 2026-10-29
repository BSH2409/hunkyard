package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
)

// These tests cover the destination-routing rules for integrations.
// They protect against regressions in integrationMap entries: missing
// globalDest fields, wrong globalDestKind, or wrong destination paths.

func TestDestFor_ProjectMode(t *testing.T) {
	// Project install must always use the dest field, regardless of whether a
	// globalDest is set.
	for name, files := range integrationMap {
		for i, f := range files {
			got := destFor(f, false, "/home/me", name)
			if got != f.dest {
				t.Errorf("%s[%d] project: got %q, want %q", name, i, got, f.dest)
			}
		}
	}
}

func TestDestFor_GlobalMode(t *testing.T) {
	// In global mode, integrations with no globalDest fall through to the raw
	// relative dest (which lands under $HOME because cwd == $HOME during a
	// global install). Integrations with a globalDest get the redirected
	// absolute path. The two routes have meaningfully different semantics, so
	// the test exercises both.
	home := "/home/me"
	cases := []struct {
		tool    string
		fileIdx int
		want    string
	}{
		// No globalDest → raw relative dest, written cwd-relative (cwd == $HOME).
		{"claude-code", 0, ".claude/skills/hunkyard/SKILL.md"},
		{"claude-code", 1, ".claude/skills/hunkyard-cli/SKILL.md"},
		{"codex", 0, ".agents/skills/hunkyard/SKILL.md"},
		{"codex", 1, ".agents/skills/hunkyard-cli/SKILL.md"},
		// Codex plugin redirects to ~/.codex/plugins/hunkyard/.
		{"codex-plugin", 0, filepath.Join(home, ".codex/plugins/hunkyard/.codex-plugin/plugin.json")},
		{"codex-plugin", 1, filepath.Join(home, ".codex/plugins/hunkyard/skills/hunkyard/SKILL.md")},
	}
	for _, tc := range cases {
		f := integrationMap[tc.tool][tc.fileIdx]
		got := destFor(f, true, home, tc.tool)
		if got != tc.want {
			t.Errorf("%s[%d] global: got %q, want %q", tc.tool, tc.fileIdx, got, tc.want)
		}
	}
}

func TestIntegrationMap_SnapshotGlobalRouting(t *testing.T) {
	// Snapshot test: verifies each tool's globalDest configuration matches
	// what the integration validation findings established. Update this test
	// when intentionally changing routing.
	type want struct {
		globalDest string
		kind       globalDestKind
	}
	expected := map[string][]want{
		"claude-code": {{"", globalDestNone}, {"", globalDestNone}, {"", globalDestNone}, {"", globalDestNone}},
		"codex":       {{"", globalDestNone}, {"", globalDestNone}, {"", globalDestNone}, {"", globalDestNone}},
		"codex-plugin": {
			{".codex/plugins/hunkyard/.codex-plugin/plugin.json", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/hunkyard/SKILL.md", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/hunkyard-cli/SKILL.md", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/hunkyard-story/SKILL.md", globalDestRelHome},
			{".codex/plugins/hunkyard/hooks/hooks.json", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/hunkyard/references/review-workflow.md", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/learn-preferences/SKILL.md", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/learn-preferences/references/local-maintenance.md", globalDestRelHome},
			{".codex/plugins/hunkyard/skills/learn-preferences/schemas/maintenance-proposal.json", globalDestRelHome},
		},
	}
	for tool, files := range expected {
		got := integrationMap[tool]
		if len(got) != len(files) {
			t.Errorf("%s: got %d files, want %d", tool, len(got), len(files))
			continue
		}
		for i, w := range files {
			if got[i].globalDest != w.globalDest || got[i].globalDestKind != w.kind {
				t.Errorf("%s[%d]: got (%q, kind=%d), want (%q, kind=%d)",
					tool, i, got[i].globalDest, got[i].globalDestKind, w.globalDest, w.kind)
			}
		}
	}
}

func TestInstallOneFile_WritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "subdir", "out.md")
	f := integration{source: "integrations/claude-code/skills/hunkyard/SKILL.md", dest: dest}

	// First install: file written.
	installOneFile(f, dest, false)
	if _, err := os.ReadFile(dest); err != nil {
		t.Fatalf("expected file at %s: %v", dest, err)
	}

	// Second install without --force: should skip without erroring.
	// Modify the file to verify it's not overwritten.
	if err := os.WriteFile(dest, []byte("hand-edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	installOneFile(f, dest, false)
	got, _ := os.ReadFile(dest)
	if string(got) != "hand-edited" {
		t.Errorf("non-force should skip; file was overwritten: %q", got)
	}

	// Force install: file overwritten with embedded content.
	installOneFile(f, dest, true)
	got, _ = os.ReadFile(dest)
	if string(got) == "hand-edited" {
		t.Errorf("force should overwrite; file still has hand-edited content")
	}
}

func TestCleanupLegacyIntegrationFiles_RemovesOnlyKnownContent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	known := []byte("known Hunkyard-managed content")
	modified := []byte("user-modified content")
	old := legacyIntegrationFiles
	t.Cleanup(func() { legacyIntegrationFiles = old })
	legacyIntegrationFiles = map[string][]legacyIntegrationFile{
		"test-known": {
			{dest: "old/hunkyard.md", hash: computeFileHash(known)},
		},
		"test-modified": {
			{dest: "modified/hunkyard.md", hash: computeFileHash(known)},
		},
		"test-missing": {
			{dest: "missing/hunkyard.md", hash: computeFileHash(known)},
		},
		"test-global": {
			{
				dest:           "project/hunkyard.md",
				globalDest:     ".agents/legacy/hunkyard.md",
				globalDestKind: globalDestRelHome,
				hash:           computeFileHash(known),
			},
		},
		"test-global-empty": {
			{
				dest: "project-only/hunkyard.md",
				hash: computeFileHash(known),
			},
		},
	}

	if err := os.MkdirAll("old", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("old/hunkyard.md", known, 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupLegacyIntegrationFiles("test-known", false, dir)
	if _, err := os.Stat("old/hunkyard.md"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("known legacy file should be removed, stat error = %v", err)
	}

	if err := os.MkdirAll("modified", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("modified/hunkyard.md", modified, 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupLegacyIntegrationFiles("test-modified", false, dir)
	got, err := os.ReadFile("modified/hunkyard.md")
	if err != nil {
		t.Fatalf("modified legacy file should be preserved: %v", err)
	}
	if string(got) != string(modified) {
		t.Fatalf("modified legacy file changed: got %q", got)
	}

	cleanupLegacyIntegrationFiles("test-missing", false, dir) // missing dest is a no-op

	home := t.TempDir()
	globalPath := filepath.Join(home, ".agents", "legacy", "hunkyard.md")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, known, 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupLegacyIntegrationFiles("test-global", true, home)
	if _, err := os.Stat(globalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("known global legacy file should be removed, stat error = %v", err)
	}

	// Entries without a globalDest are project-only; global cleanup must skip them.
	if err := os.MkdirAll("project-only", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("project-only/hunkyard.md", known, 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupLegacyIntegrationFiles("test-global-empty", true, home)
	if _, err := os.Stat("project-only/hunkyard.md"); err != nil {
		t.Fatalf("project-only legacy file must remain during global cleanup: %v", err)
	}
}

func TestInstallIntegration_CodexRemovesObsoleteImplicitInvocationPolicy(t *testing.T) {
	const knownYAML = "policy:\n  allow_implicit_invocation: false\n"
	knownHash := "a1499d95abd8447558c535fe5554adcc3c9b988a0a39264a6283d430effe1e94"
	if got := computeFileHash([]byte(knownYAML)); got != knownHash {
		t.Fatalf("fixture hash = %s, want %s (update test if shipped openai.yaml bytes changed)", got, knownHash)
	}

	t.Run("project codex", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "project")
		home := filepath.Join(root, "home")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.SetHome(t, home)
		t.Chdir(dir)

		legacyPath := ".agents/skills/hunkyard/agents/openai.yaml"
		if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyPath, []byte(knownYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := installIntegration("codex", false); err != nil {
			t.Fatalf("install codex: %v", err)
		}
		if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("project openai.yaml should be removed, stat error = %v", err)
		}
	})

	t.Run("global codex", func(t *testing.T) {
		home := t.TempDir()
		testutil.SetHome(t, home)
		t.Chdir(home)

		legacyPath := filepath.Join(home, ".agents/skills/hunkyard/agents/openai.yaml")
		if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacyPath, []byte(knownYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := installIntegration("codex", false); err != nil {
			t.Fatalf("install codex: %v", err)
		}
		if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("global openai.yaml should be removed, stat error = %v", err)
		}
	})

	t.Run("global codex-plugin", func(t *testing.T) {
		home := t.TempDir()
		testutil.SetHome(t, home)
		t.Chdir(home)

		loosePath := filepath.Join(home, ".agents/skills/hunkyard/agents/openai.yaml")
		pluginPath := filepath.Join(home, ".codex/plugins/hunkyard/skills/hunkyard/agents/openai.yaml")
		for _, path := range []string{loosePath, pluginPath} {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(knownYAML), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := installIntegration("codex-plugin", false); err != nil {
			t.Fatalf("install codex-plugin: %v", err)
		}
		for _, path := range []string{loosePath, pluginPath} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s should be removed, stat error = %v", path, err)
			}
		}
	})
}

func TestInstallIntegration_CodexPluginEndToEnd(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.SetHome(t, home)
	t.Chdir(dir)

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}

	for _, path := range []string{
		".agents/skills/hunkyard/SKILL.md",
		".agents/skills/hunkyard-cli/SKILL.md",
		"plugins/hunkyard/.codex-plugin/plugin.json",
		"plugins/hunkyard/skills/hunkyard/SKILL.md",
		"plugins/hunkyard/skills/hunkyard-cli/SKILL.md",
		"plugins/hunkyard/hooks/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}

	hookPath := filepath.Join(dir, "plugins/hunkyard/hooks/hooks.json")
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hookData), "plan-hook") {
		t.Fatalf("local plugin must not intercept plan exit: %s", hookData)
	}

	marketplacePath := filepath.Join(dir, ".agents/plugins/marketplace.json")
	if got := countHunkyardMarketplaceEntries(t, marketplacePath, "./plugins/hunkyard"); got != 1 {
		t.Fatalf("expected one Hunkyard marketplace entry after first install, got %d", got)
	}
	assertHunkyardMarketplacePathExists(t, marketplacePath, dir)
	assertCodexPluginEnabled(t, filepath.Join(home, ".codex", "config.toml"), "hunkyard@local")
	for _, path := range []string{
		".codex/plugins/cache/local/hunkyard/local/.codex-plugin/plugin.json",
		".codex/plugins/cache/local/hunkyard/local/skills/hunkyard/SKILL.md",
		".codex/plugins/cache/local/hunkyard/local/hooks/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(home, path)); err != nil {
			t.Fatalf("expected cache file %s to be written: %v", path, err)
		}
	}

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("second installIntegration: %v", err)
	}
	if got := countHunkyardMarketplaceEntries(t, marketplacePath, "./plugins/hunkyard"); got != 1 {
		t.Fatalf("expected idempotent marketplace registration, got %d entries", got)
	}
}

func TestInstallIntegration_CodexPluginGlobalEndToEnd(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Chdir(home)

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}

	for _, path := range []string{
		".agents/skills/hunkyard/SKILL.md",
		".agents/skills/hunkyard-cli/SKILL.md",
		".codex/plugins/hunkyard/.codex-plugin/plugin.json",
		".codex/plugins/hunkyard/skills/hunkyard/SKILL.md",
		".codex/plugins/hunkyard/skills/hunkyard-cli/SKILL.md",
		".codex/plugins/hunkyard/hooks/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(home, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}

	marketplacePath := filepath.Join(home, ".agents/plugins/marketplace.json")
	if got := countHunkyardMarketplaceEntries(t, marketplacePath, "./.codex/plugins/hunkyard"); got != 1 {
		t.Fatalf("expected one Hunkyard marketplace entry after global install, got %d", got)
	}
	assertHunkyardMarketplacePathExists(t, marketplacePath, home)
	assertCodexPluginEnabled(t, filepath.Join(home, ".codex", "config.toml"), "hunkyard@local")
	if _, err := os.Stat(filepath.Join(home, ".codex/plugins/cache/local/hunkyard/local/.codex-plugin/plugin.json")); err != nil {
		t.Fatalf("expected global cache to be written: %v", err)
	}
}

func TestInstallIntegration_CodexPluginDoesNotActivateExistingProjectFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(dir, "plugins/hunkyard/hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins/hunkyard/.codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.SetHome(t, home)
	t.Chdir(dir)

	staleHookPath := filepath.Join(dir, "plugins/hunkyard/hooks/hooks.json")
	if err := os.WriteFile(staleHookPath, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"stale-command"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins/hunkyard/.codex-plugin/plugin.json"), []byte(`{"name":"hunkyard","hooks":"./hooks/hooks.json"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}

	embeddedHook, err := integrationsFS.ReadFile("integrations/codex/plugin/hunkyard/hooks/hooks.json")
	if err != nil {
		t.Fatalf("reading embedded hook: %v", err)
	}
	for _, path := range []string{
		staleHookPath,
		filepath.Join(home, ".codex/plugins/cache/local/hunkyard/local/hooks/hooks.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(data) != string(embeddedHook) {
			t.Fatalf("%s did not use embedded Hunkyard hook:\n%s", path, data)
		}
	}
}

func TestInstallCodexPluginMarketplaceForceOverwritesInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", true); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	if got := countHunkyardMarketplaceEntries(t, path, "./plugins/hunkyard"); got != 1 {
		t.Fatalf("expected one Hunkyard marketplace entry, got %d", got)
	}
}

func TestInstallCodexPluginMarketplaceForceOverwritesMalformedPlugins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"plugins":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", true); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	if got := countHunkyardMarketplaceEntries(t, path, "./plugins/hunkyard"); got != 1 {
		t.Fatalf("expected one Hunkyard marketplace entry, got %d", got)
	}
}

func TestInstallCodexPluginMarketplaceInvalidJSONReturnsErrorWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err == nil {
		t.Fatal("expected invalid marketplace JSON to return an error")
	}
}

func TestInstallCodexPluginMarketplaceMalformedPluginsReturnsErrorWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"plugins":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err == nil {
		t.Fatal("expected malformed plugins field to return an error")
	}
}

func TestInstallCodexPluginMarketplaceRejectsWhitespaceNameWithoutForce(t *testing.T) {
	for _, name := range []string{"local ", "   "} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "marketplace.json")
			existing := fmt.Sprintf(`{
  "name": %q,
  "plugins": [
    {"name": "hunkyard", "source": {"source": "local", "path": "./plugins/hunkyard"}}
  ]
}`, name)
			if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err == nil {
				t.Fatal("expected whitespace marketplace name to be rejected without force")
			}
		})
	}
}

func TestInstallCodexPluginMarketplaceRepairsStaleSourcePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	stale := `{
  "name": "local",
  "plugins": [
    {
      "name": "hunkyard",
      "source": {"source": "local", "path": "./plugins/hunkyard"},
      "policy": {"installation": "INSTALLED_BY_DEFAULT", "authentication": "ON_INSTALL"},
      "category": "Developer Tools"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./.codex/plugins/hunkyard", false); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	if got := countHunkyardMarketplaceEntries(t, path, "./.codex/plugins/hunkyard"); got != 1 {
		t.Fatalf("expected stale Hunkyard marketplace entry to be replaced, got %d", got)
	}
}

func TestInstallCodexPluginMarketplaceRepairsStalePolicyAndDedupes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	stale := `{
  "name": "local",
  "plugins": [
    {
      "name": "hunkyard",
      "source": {"source": "local", "path": "./plugins/hunkyard"},
      "policy": {"installation": "AVAILABLE", "authentication": "ON_USE"},
      "category": "Old Category"
    },
    {
      "name": "hunkyard",
      "source": {"source": "local", "path": "./plugins/hunkyard"},
      "policy": {"installation": "INSTALLED_BY_DEFAULT", "authentication": "ON_INSTALL"},
      "category": "Developer Tools"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	if got := countHunkyardMarketplaceEntries(t, path, "./plugins/hunkyard"); got != 1 {
		t.Fatalf("expected one repaired Hunkyard marketplace entry, got %d", got)
	}
}

func TestInstallCodexPluginMarketplacePreservesOtherTypedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	existing := `{
  "name": "local",
  "sentinel": {"keep": true},
  "interface": {"displayName": "My Plugins", "theme": "dark"},
  "plugins": [
    {
      "name": "other",
      "source": {"source": "local", "path": "./other", "kind": "dev"},
      "policy": {"authentication": "ON_USE", "extra": "kept"},
      "category": "Other",
      "summary": "keep me"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var marketplace codexPluginMarketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatal(err)
	}
	if _, ok := marketplace.Extra["sentinel"]; !ok {
		t.Fatalf("expected top-level sentinel to be preserved: %s", data)
	}
	if _, ok := marketplace.Interface.Extra["theme"]; !ok {
		t.Fatalf("expected interface theme to be preserved: %s", data)
	}
	var other codexMarketplacePlugin
	for _, plugin := range marketplace.Plugins {
		if plugin.Name == "other" {
			other = plugin
			break
		}
	}
	if other.Name == "" {
		t.Fatalf("expected other plugin to be preserved: %s", data)
	}
	if len(other.Extra["summary"]) == 0 {
		t.Fatalf("expected plugin summary to be preserved: %s", data)
	}
	if len(other.Source.Extra["kind"]) == 0 {
		t.Fatalf("expected source kind to be preserved: %s", data)
	}
	if len(other.Policy.Extra["extra"]) == 0 {
		t.Fatalf("expected policy extra to be preserved: %s", data)
	}
}

func TestInstallCodexPluginMarketplacePreservesShorthandSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	existing := `{
  "name": "local",
  "plugins": [
    {"name": "other", "source": "./plugins/other", "category": "Other"}
  ]
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodexPluginMarketplace(path, "./plugins/hunkyard", false); err != nil {
		t.Fatalf("installCodexPluginMarketplace: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var marketplace struct {
		Plugins []struct {
			Name   string          `json:"name"`
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatal(err)
	}
	for _, plugin := range marketplace.Plugins {
		if plugin.Name == "other" && string(plugin.Source) == `"./plugins/other"` {
			return
		}
	}
	t.Fatalf("expected shorthand source to be preserved: %s", data)
}

func TestUpsertCodexPluginConfig(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "creates plugin table",
			want: "[features]\nplugins = true\nhooks = true\nplugin_hooks = true\n\n[plugins.\"hunkyard@local\"]\nenabled = true\n",
		},
		{
			name: "enables existing plugin table",
			raw:  "[plugins.\"hunkyard@local\"]\nenabled = false\nsource = \"keep\"\n",
			want: "[plugins.\"hunkyard@local\"]\nenabled = true\nsource = \"keep\"\n\n[features]\nplugins = true\nhooks = true\nplugin_hooks = true\n",
		},
		{
			name: "preserves surrounding config",
			raw:  "model = \"gpt-5.1\"\n\n[features]\nhooks = false\n\n[plugins.\"other@local\"]\nenabled = true\n",
			want: "model = \"gpt-5.1\"\n\n[features]\nhooks = true\nplugins = true\nplugin_hooks = true\n\n[plugins.\"other@local\"]\nenabled = true\n\n[plugins.\"hunkyard@local\"]\nenabled = true\n",
		},
		{
			name: "updates commented table headers",
			raw:  "[features] # managed by user\nhooks = false\n\n[plugins.\"hunkyard@local\"] # existing plugin\nenabled = false\n",
			want: "[features] # managed by user\nhooks = true\nplugins = true\nplugin_hooks = true\n\n[plugins.\"hunkyard@local\"] # existing plugin\nenabled = true\n",
		},
		{
			name: "stops before array table headers",
			raw:  "[features]\nhooks = false\n\n[[hooks.PreToolUse]]\nmatcher = \"shell\"\n\n[plugins.\"hunkyard@local\"]\nenabled = false\n",
			want: "[features]\nhooks = true\nplugins = true\nplugin_hooks = true\n\n[[hooks.PreToolUse]]\nmatcher = \"shell\"\n\n[plugins.\"hunkyard@local\"]\nenabled = true\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := upsertCodexPluginConfig(tt.raw, "hunkyard@local"); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestInstallCodexPluginConfigPreservesModeAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "managed-config.toml")
	link := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, []byte("model = \"gpt-5.1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := installCodexPluginConfig(link, "hunkyard@local"); err != nil {
		t.Fatalf("installCodexPluginConfig: %v", err)
	}
	if info, err := os.Lstat(link); err != nil {
		t.Fatal(err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected config path symlink to be preserved")
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[plugins.\"hunkyard@local\"]") {
		t.Fatalf("expected target config to be updated, got:\n%s", data)
	}
}

func TestInstallCodexPluginConfigDefaultsNewFileTo0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := installCodexPluginConfig(path, "hunkyard@local"); err != nil {
		t.Fatalf("installCodexPluginConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestCodexPluginCachePathComponentsAreValidated(t *testing.T) {
	for _, value := range []string{"", "..", ".", "bad/name", `bad\name`, "bad.name", "bad name", " ", " local", "local ", "ümlaut"} {
		if _, err := validCodexPluginSegment(value, "test"); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
	for _, value := range []string{"local", "my_market-1"} {
		if got, err := validCodexPluginSegment(value, "test"); err != nil || got != value {
			t.Fatalf("valid component = %q, %v; want %q, nil", got, err, value)
		}
	}
}

func TestCodexPluginManifestVersionRejectsWhitespace(t *testing.T) {
	if _, err := codexPluginManifestVersionFromBytes([]byte(`{"version":"   "}`)); err == nil {
		t.Fatal("expected whitespace version to be rejected")
	}
	if got, err := codexPluginManifestVersionFromBytes([]byte(`{}`)); err != nil || got != "local" {
		t.Fatalf("version = %q, %v; want local, nil", got, err)
	}
}

func countHunkyardMarketplaceEntries(t *testing.T, path, wantSourcePath string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected marketplace at %s: %v", path, err)
	}
	var marketplace codexPluginMarketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatalf("marketplace is not valid JSON: %v", err)
	}
	count := 0
	for _, plugin := range marketplace.Plugins {
		if plugin.Name != "hunkyard" {
			continue
		}
		count++
		if plugin.Source.Source != "local" || plugin.Source.Path != wantSourcePath {
			t.Fatalf("unexpected Hunkyard plugin source: %+v", plugin.Source)
		}
		if plugin.Policy.Installation != "INSTALLED_BY_DEFAULT" {
			t.Fatalf("unexpected Hunkyard plugin policy: %+v", plugin.Policy)
		}
		if plugin.Policy.Authentication != "ON_INSTALL" {
			t.Fatalf("unexpected Hunkyard plugin auth policy: %+v", plugin.Policy)
		}
		if plugin.Category != "Developer Tools" {
			t.Fatalf("unexpected Hunkyard plugin category: %+v", plugin.Category)
		}
	}
	return count
}

func assertHunkyardMarketplacePathExists(t *testing.T, marketplacePath, marketplaceRoot string) {
	t.Helper()

	data, err := os.ReadFile(marketplacePath)
	if err != nil {
		t.Fatal(err)
	}
	var marketplace codexPluginMarketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatal(err)
	}
	for _, plugin := range marketplace.Plugins {
		if plugin.Name != "hunkyard" {
			continue
		}
		relPath := plugin.Source.Path
		pluginRoot := filepath.Join(marketplaceRoot, relPath)
		manifestPath := filepath.Join(pluginRoot, ".codex-plugin", "plugin.json")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("marketplace path %q should resolve to installed plugin manifest %s: %v", relPath, manifestPath, err)
		}
		return
	}
	t.Fatal("Hunkyard marketplace entry not found")
}

func assertCodexPluginEnabled(t *testing.T, path, pluginKey string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected Codex config at %s: %v", path, err)
	}
	want := "[plugins.\"" + pluginKey + "\"]\nenabled = true"
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected Codex config to enable %s, got:\n%s", pluginKey, data)
	}
	if !codexPluginConfigReadyRaw(string(data), pluginKey) {
		t.Fatalf("expected Codex config to enable plugins, hooks, plugin_hooks, and %s, got:\n%s", pluginKey, data)
	}
}

func TestPrintUniqueHints_Dedups(t *testing.T) {
	// printUniqueHints prints to stdout; we just verify it doesn't panic on
	// duplicates and empty input. Output ordering and dedup logic are simple
	// enough that visual inspection during integration use covers the rest.
	printUniqueHints(nil)
	printUniqueHints([]string{"a", "b", "a", "c", "b"})
}
