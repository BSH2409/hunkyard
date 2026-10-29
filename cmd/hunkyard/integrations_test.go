package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// isolateAgentPATH points PATH at an empty temp dir and returns it, so agent
// detection does not find (and run `--version` on) whatever AI CLIs the
// developer has installed. Those probes take seconds each and make results
// machine-dependent. Tests that need a binary on PATH write a stub into the
// returned dir.
func isolateAgentPATH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	return dir
}

// writeStubBinary creates an executable named name in dir that prints out
// when run. On Windows only its presence matters to exec.LookPath.
func writeStubBinary(t *testing.T, dir, name, out string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\necho '" + out + "'\n"
	if runtime.GOOS == "windows" {
		path += ".bat"
		content = "@echo " + out + "\r\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestComputeFileHash(t *testing.T) {
	h1 := computeFileHash([]byte("hello world"))
	h2 := computeFileHash([]byte("hello world"))
	h3 := computeFileHash([]byte("different content"))

	if h1 != h2 {
		t.Errorf("same content should produce same hash: %q != %q", h1, h2)
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %q", len(h1), h1)
	}
}

func TestCheckInstalledIntegrations_StaleFile(t *testing.T) {
	isolateAgentPATH(t)
	dir := t.TempDir()

	// Write a file at the claude-code skill destination with different content
	ccDest := filepath.Join(dir, ".claude", "skills", "hunkyard")
	if err := os.MkdirAll(ccDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ccDest, "SKILL.md"), []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale := checkInstalledIntegrations(dir, dir)
	if len(stale) == 0 {
		t.Fatal("expected stale files, got none")
	}

	found := false
	for _, s := range stale {
		if s.agent == "claude-code" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-code in stale results")
	}
}

func TestCheckInstalledIntegrations_UpToDate(t *testing.T) {
	isolateAgentPATH(t)
	dir := t.TempDir()

	// Read the actual embedded content and write it to the destination
	// so it matches the precomputed hash
	embedded, err := integrationsFS.ReadFile("integrations/claude-code/skills/hunkyard/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	ccDest := filepath.Join(dir, ".claude", "skills", "hunkyard")
	if err := os.MkdirAll(ccDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ccDest, "SKILL.md"), embedded, 0o644); err != nil {
		t.Fatal(err)
	}

	stale := checkInstalledIntegrations(dir, dir)
	for _, s := range stale {
		if s.agent == "claude-code" && s.dest == filepath.Join(ccDest, "SKILL.md") {
			t.Error("file matches embedded content, should not be stale")
		}
	}
}

func TestCheckInstalledIntegrations_MissingFile(t *testing.T) {
	isolateAgentPATH(t)
	dir := t.TempDir()
	stale := checkInstalledIntegrations(dir, dir)
	if len(stale) != 0 {
		t.Errorf("expected no stale files for empty dir, got %d", len(stale))
	}
}

func TestCheckInstalledIntegrations_HomeDirStale(t *testing.T) {
	isolateAgentPATH(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Write stale file only in homeDir
	ccDest := filepath.Join(homeDir, ".claude", "skills", "hunkyard")
	if err := os.MkdirAll(ccDest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ccDest, "SKILL.md"), []byte("old version"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale := checkInstalledIntegrations(projectDir, homeDir)
	if len(stale) == 0 {
		t.Fatal("expected stale file in home dir, got none")
	}
	if stale[0].dest != filepath.Join(ccDest, "SKILL.md") {
		t.Errorf("expected home dir path, got %s", stale[0].dest)
	}
}

func TestCheckInstalledIntegrations_MarketplaceStale(t *testing.T) {
	isolateAgentPATH(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Write stale file at marketplace source path
	mpPath := filepath.Join(homeDir, ".claude", "plugins", "marketplaces", "hunkyard",
		"integrations", "claude-code", "skills", "hunkyard")
	if err := os.MkdirAll(mpPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mpPath, "SKILL.md"), []byte("old marketplace"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale := checkInstalledIntegrations(projectDir, homeDir)
	if len(stale) == 0 {
		t.Fatal("expected stale marketplace file, got none")
	}

	found := false
	for _, s := range stale {
		if s.location == locationMarketplace {
			found = true
			if !strings.Contains(s.updateHint(), "claude plugin update hunkyard@hunkyard") {
				t.Errorf("marketplace hint should suggest plugin update, got: %s", s.updateHint())
			}
		}
	}
	if !found {
		t.Error("expected marketplace location in stale results")
	}
}

func TestCheckInstalledIntegrations_CacheStale(t *testing.T) {
	isolateAgentPATH(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Write stale file at cache path with hash-named dir
	cachePath := filepath.Join(homeDir, ".claude", "plugins", "cache", "hunkyard", "hunkyard",
		"abc123def456", "skills", "hunkyard")
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "SKILL.md"), []byte("cached old"), 0o644); err != nil {
		t.Fatal(err)
	}

	stale := checkInstalledIntegrations(projectDir, homeDir)
	if len(stale) == 0 {
		t.Fatal("expected stale cache file, got none")
	}

	found := false
	for _, s := range stale {
		if s.location == locationCache {
			found = true
			if !strings.Contains(s.updateHint(), "claude plugin update hunkyard@hunkyard") {
				t.Errorf("cache hint should suggest plugin update, got: %s", s.updateHint())
			}
		}
	}
	if !found {
		t.Error("expected cache location in stale results")
	}
}

func TestLatestCacheDir(t *testing.T) {
	t.Run("picks lexicographically last dir", func(t *testing.T) {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, "1.0.0"), 0o755)
		os.Mkdir(filepath.Join(dir, "1.0.2"), 0o755)
		os.Mkdir(filepath.Join(dir, "1.0.1"), 0o755)
		if got := latestCacheDir(dir); got != "1.0.2" {
			t.Errorf("got %q, want 1.0.2", got)
		}
	})
	t.Run("ignores files", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "zzz"), nil, 0o644)
		os.Mkdir(filepath.Join(dir, "1.0.0"), 0o755)
		if got := latestCacheDir(dir); got != "1.0.0" {
			t.Errorf("got %q, want 1.0.0", got)
		}
	})
	t.Run("returns empty for nonexistent dir", func(t *testing.T) {
		if got := latestCacheDir("/no/such/path"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("returns empty for empty dir", func(t *testing.T) {
		if got := latestCacheDir(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestCheckInstalledIntegrations_CacheSkipsOldVersions(t *testing.T) {
	isolateAgentPATH(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create two version dirs: 1.0.0 (stale) and 1.0.1 (current)
	for _, ver := range []string{"1.0.0", "1.0.1"} {
		cachePath := filepath.Join(homeDir, ".claude", "plugins", "cache", "hunkyard", "hunkyard",
			ver, "skills", "hunkyard")
		if err := os.MkdirAll(cachePath, 0o755); err != nil {
			t.Fatal(err)
		}
		if ver == "1.0.0" {
			// Stale content in old version
			os.WriteFile(filepath.Join(cachePath, "SKILL.md"), []byte("old stale"), 0o644)
		} else {
			// Current content — use the real source file to get the correct hash
			data, err := integrationsFS.ReadFile("integrations/claude-code/skills/hunkyard/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(cachePath, "SKILL.md"), data, 0o644)
		}
	}

	stale := checkInstalledIntegrations(projectDir, homeDir)
	for _, s := range stale {
		if s.location == locationCache {
			t.Errorf("should not flag cache as stale when latest version matches, got: %s", s.dest)
		}
	}
}

func TestPrintStaleWarnings_NoStale(t *testing.T) {
	count := printStaleWarnings(nil)
	if count != 0 {
		t.Errorf("expected 0 warnings for nil slice, got %d", count)
	}
}

func TestPrintStaleWarnings_WithStale(t *testing.T) {
	stale := []staleFile{
		{agent: "claude-code", file: "SKILL.md", dest: "/tmp/test/.claude/skills/hunkyard/SKILL.md", location: locationProject},
	}
	count := printStaleWarnings(stale)
	if count == 0 {
		t.Error("expected at least 1 warning")
	}
}

func TestDetectInstalledIntegrations(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	// No integrations installed — should return empty
	result := detectInstalledIntegrations(projectDir, homeDir)
	if len(result) != 0 {
		t.Errorf("expected 0 integrations, got %d", len(result))
	}

	// Install a current integration file
	sourceFiles := integrationMap["claude-code"]
	if len(sourceFiles) == 0 {
		t.Fatal("no claude-code integration files defined")
	}
	sourceContent, err := integrationsFS.ReadFile(sourceFiles[0].source)
	if err != nil {
		t.Fatalf("reading embedded source: %v", err)
	}
	dest := filepath.Join(projectDir, sourceFiles[0].dest)
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, sourceContent, 0o644)

	result = detectInstalledIntegrations(projectDir, homeDir)
	if len(result) == 0 {
		t.Fatal("expected at least 1 integration, got 0")
	}
	found := false
	for _, r := range result {
		if r.Agent == "claude-code" && r.Status == "current" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected claude-code with status current, got %+v", result)
	}

	// Write a stale file
	os.WriteFile(dest, []byte("stale content"), 0o644)
	result = detectInstalledIntegrations(projectDir, homeDir)
	found = false
	for _, r := range result {
		if r.Agent == "claude-code" && r.Status == "stale" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected claude-code with status stale, got %+v", result)
	}
}

func TestDetectInstalledIntegrations_DedupsPerAgent(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	// Install same integration in both project and home — should only appear once
	sourceFiles := integrationMap["claude-code"]
	if len(sourceFiles) == 0 {
		t.Fatal("no claude-code integration files defined")
	}
	sourceContent, err := integrationsFS.ReadFile(sourceFiles[0].source)
	if err != nil {
		t.Fatalf("reading embedded source: %v", err)
	}
	for _, dir := range []string{projectDir, homeDir} {
		dest := filepath.Join(dir, sourceFiles[0].dest)
		os.MkdirAll(filepath.Dir(dest), 0o755)
		os.WriteFile(dest, sourceContent, 0o644)
	}

	result := detectInstalledIntegrations(projectDir, homeDir)
	count := 0
	for _, r := range result {
		if r.Agent == "claude-code" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 claude-code entry (deduped), got %d", count)
	}
}

func TestDetectInstalledIntegrations_CodexPluginRequiresPluginFile(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	for _, f := range integrationMap["codex"] {
		sourceContent, err := integrationsFS.ReadFile(f.source)
		if err != nil {
			t.Fatalf("reading embedded source: %v", err)
		}
		dest := filepath.Join(projectDir, f.dest)
		os.MkdirAll(filepath.Dir(dest), 0o755)
		os.WriteFile(dest, sourceContent, 0o644)
	}

	result := detectInstalledIntegrations(projectDir, homeDir)
	foundCodex := false
	for _, r := range result {
		switch r.Agent {
		case "codex":
			foundCodex = true
		case "codex-plugin":
			t.Fatalf("plain Codex skill install should not detect codex-plugin: %+v", result)
		}
	}
	if !foundCodex {
		t.Fatalf("expected Codex integration to be detected, got %+v", result)
	}
}

func TestDetectInstalledIntegrations_CodexPluginGlobalUsesGlobalDest(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	f := integrationMap["codex-plugin"][0]
	sourceContent, err := integrationsFS.ReadFile(f.source)
	if err != nil {
		t.Fatalf("reading embedded source: %v", err)
	}
	dest, err := resolveGlobalDest(f.globalDestKind, f.globalDest, homeDir)
	if err != nil {
		t.Fatalf("resolving global destination: %v", err)
	}
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, sourceContent, 0o644)

	result := detectInstalledIntegrations(projectDir, homeDir)
	for _, r := range result {
		if r.Agent != "codex-plugin" {
			continue
		}
		if r.Status != "current" || r.Location != locationHome {
			t.Fatalf("expected current global codex-plugin, got %+v", r)
		}
		return
	}
	t.Fatalf("expected codex-plugin to be detected, got %+v", result)
}

func TestCheckInstalledIntegrations_CodexPluginCacheStale(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	f := integrationMap["codex-plugin"][1]
	relPath, ok := strings.CutPrefix(f.source, "integrations/codex/plugin/hunkyard/")
	if !ok {
		t.Fatalf("unexpected codex-plugin source path %q", f.source)
	}
	cachePath := filepath.Join(homeDir, ".codex", "plugins", "cache", "local", "hunkyard", "local", relPath)
	os.MkdirAll(filepath.Dir(cachePath), 0o755)
	os.WriteFile(cachePath, []byte("old cached skill"), 0o644)

	stale := checkInstalledIntegrations(projectDir, homeDir)
	for _, s := range stale {
		if s.agent == "codex-plugin" && s.location == locationCache && s.dest == cachePath {
			return
		}
	}
	t.Fatalf("expected stale codex-plugin cache file %s, got %+v", cachePath, stale)
}

func TestCheckInstalledIntegrations_CodexPluginCacheUsesMarketplaceName(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(filepath.Join(projectDir, ".agents", "plugins"), 0o755)
	os.MkdirAll(homeDir, 0o755)

	marketplace := `{
  "name": "personal",
  "plugins": [
    {"name": "hunkyard", "source": {"source": "local", "path": "./plugins/hunkyard"}}
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), []byte(marketplace), 0o644); err != nil {
		t.Fatal(err)
	}

	f := integrationMap["codex-plugin"][2]
	relPath, ok := strings.CutPrefix(f.source, "integrations/codex/plugin/hunkyard/")
	if !ok {
		t.Fatalf("unexpected codex-plugin source path %q", f.source)
	}
	cachePath := filepath.Join(homeDir, ".codex", "plugins", "cache", "personal", "hunkyard", "local", relPath)
	os.MkdirAll(filepath.Dir(cachePath), 0o755)
	os.WriteFile(cachePath, []byte("old cached skill"), 0o644)

	stale := checkInstalledIntegrations(projectDir, homeDir)
	for _, s := range stale {
		if s.agent == "codex-plugin" && s.location == locationCache && s.dest == cachePath {
			return
		}
	}
	t.Fatalf("expected stale codex-plugin cache file %s, got %+v", cachePath, stale)
}

func TestCodexPluginMarketplaceNamesRejectsInvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(filepath.Join(projectDir, ".agents", "plugins"), 0o755)
	os.MkdirAll(homeDir, 0o755)

	marketplace := `{
  "name": "../outside",
  "plugins": [
    {"name": "hunkyard", "source": {"source": "local", "path": "./plugins/hunkyard"}}
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), []byte(marketplace), 0o644); err != nil {
		t.Fatal(err)
	}

	names := codexPluginMarketplaceNames(projectDir, homeDir)
	if len(names) != 1 || names[0] != "local" {
		t.Fatalf("marketplace names = %#v, want [local]", names)
	}
}

func TestCodexPluginMarketplaceNamesAcceptsShorthandSource(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(filepath.Join(projectDir, ".agents", "plugins"), 0o755)
	os.MkdirAll(homeDir, 0o755)

	marketplace := `{
  "name": "personal",
  "plugins": [
    {"name": "hunkyard", "source": "./plugins/hunkyard"}
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), []byte(marketplace), 0o644); err != nil {
		t.Fatal(err)
	}

	names := codexPluginMarketplaceNames(projectDir, homeDir)
	if len(names) != 2 || names[0] != "personal" || names[1] != "local" {
		t.Fatalf("marketplace names = %#v, want [personal local]", names)
	}
}

func TestCheckInstalledIntegrations_CodexPluginMissingMarketplaceConfigAndCache(t *testing.T) {
	isolateAgentPATH(t)
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	homeDir := filepath.Join(tmpDir, "home")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(homeDir, 0o755)

	manifest := integrationMap["codex-plugin"][0]
	sourceContent, err := integrationsFS.ReadFile(manifest.source)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(projectDir, "plugins", "hunkyard", ".codex-plugin", "plugin.json")
	os.MkdirAll(filepath.Dir(manifestPath), 0o755)
	os.WriteFile(manifestPath, sourceContent, 0o644)

	stale := checkInstalledIntegrations(projectDir, homeDir)
	want := []string{
		filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"),
		filepath.Join(homeDir, ".codex", "config.toml"),
		filepath.Join(homeDir, ".codex", "plugins", "cache", "local", "hunkyard", "local", ".codex-plugin", "plugin.json"),
	}
	for _, path := range want {
		found := false
		for _, s := range stale {
			if s.agent == "codex-plugin" && s.dest == path {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected codex-plugin completeness warning for %s, got %+v", path, stale)
		}
	}
}

func TestCodexPluginConfigReadyRaw(t *testing.T) {
	raw := strings.Join([]string{
		"model = \"gpt-5.1\"",
		"",
		"[features]",
		"plugins = true",
		"hooks = true",
		"plugin_hooks = true",
		"",
		"[plugins.\"hunkyard@local\"]",
		"enabled = true",
		"",
		"[plugins.\"other@local\"]",
		"enabled = false",
	}, "\n")
	if !codexPluginConfigReadyRaw(raw, "hunkyard@local") {
		t.Fatal("expected hunkyard plugin config to be ready")
	}
	if codexPluginConfigReadyRaw(raw, "other@local") {
		t.Fatal("did not expect disabled plugin config to be ready")
	}
	if codexPluginConfigReadyRaw(strings.Replace(raw, "plugin_hooks = true", "plugin_hooks = false", 1), "hunkyard@local") {
		t.Fatal("did not expect config without plugin_hooks to be ready")
	}
	if codexPluginConfigReadyRaw(strings.Replace(raw, "plugins = true", "plugins = false", 1), "hunkyard@local") {
		t.Fatal("did not expect config without plugins to be ready")
	}

	commentedHeaders := strings.ReplaceAll(raw, "[features]", "[features] # managed")
	commentedHeaders = strings.ReplaceAll(commentedHeaders, "[plugins.\"hunkyard@local\"]", "[plugins.\"hunkyard@local\"] # managed")
	if !codexPluginConfigReadyRaw(commentedHeaders, "hunkyard@local") {
		t.Fatal("expected commented table headers to be ready")
	}

	arrayTableRaw := strings.Join([]string{
		"[features]",
		"hooks = true",
		"",
		"[[hooks.PreToolUse]]",
		"plugins = true",
		"plugin_hooks = true",
		"",
		"[plugins.\"hunkyard@local\"]",
		"enabled = true",
	}, "\n")
	if codexPluginConfigReadyRaw(arrayTableRaw, "hunkyard@local") {
		t.Fatal("did not expect feature keys in array tables to count")
	}
}

func TestRunCheck_NoStale(t *testing.T) {
	// runCheck uses os.Getwd() and os.UserHomeDir(), so we just verify it doesn't panic
	// when called in a temp dir with no installed integrations
	isolateAgentPATH(t)
	t.Chdir(t.TempDir())

	out := captureStderr(t, runCheck)
	for _, want := range []string{
		"All installed integrations are up to date.",
		"unavailable: GitHub (gh is not installed)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("runCheck output missing %q:\n%s", want, out)
		}
	}
}

func TestCheckForgeCLIsReportsAuthState(t *testing.T) {
	bin := isolateAgentPATH(t)
	// gh is installed but not logged in: non-zero exit, two output lines.
	gh, body := filepath.Join(bin, "gh"), "#!/bin/sh\necho 'not logged in'\necho 'run gh auth login'\nexit 1\n"
	if runtime.GOOS == "windows" {
		gh, body = gh+".bat", "@echo not logged in\r\n@echo run gh auth login\r\n@exit /b 1\r\n"
	}
	if err := os.WriteFile(gh, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	got := checkForgeCLIs()
	if len(got) != 1 {
		t.Fatalf("checkForgeCLIs = %+v, want gh", got)
	}
	if got[0].binary != "gh" || got[0].authenticated || strings.TrimSpace(got[0].detail) != "not logged in" {
		t.Errorf("gh status = %+v, want unauthenticated with the first output line as detail", got[0])
	}

	writeStubBinary(t, bin, "gh", "Logged in to github.com")
	got = checkForgeCLIs()
	if len(got) != 1 || !got[0].authenticated || got[0].detail != "" {
		t.Errorf("gh status = %+v, want authenticated with no detail", got)
	}
}

func TestPrintForgeCLIHintsRendersDetectionSeparately(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	printForgeCLIHints([]forgeCLIStatus{{
		name: "GitHub", binary: "gh", loginHint: "gh auth login", detail: "not logged in",
	}})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = oldStderr
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"Remote review providers:",
		"needs login: GitHub (run gh auth login): not logged in",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("forge CLI hints missing %q:\n%s", want, text)
		}
	}
}

func TestDetectPresentAgents_BinaryOnPath(t *testing.T) {
	bin := isolateAgentPATH(t)
	homeDir := t.TempDir()
	writeStubBinary(t, bin, "claude", "1.0.0 (Claude Code)")
	writeStubBinary(t, bin, "codex", "codex-cli 1.0.0")

	got := detectPresentAgents(homeDir)
	want := []string{"claude-code", "codex"}
	if !slices.Equal(got, want) {
		t.Errorf("detectPresentAgents = %v, want %v", got, want)
	}
}

func TestDetectPresentAgents_ConfigDir(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()

	// Create a .claude directory to simulate claude-code presence
	os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755)

	agents := detectPresentAgents(homeDir)
	found := false
	for _, a := range agents {
		if a == "claude-code" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-code to be detected via .claude dir")
	}
}

func TestDetectPresentAgents_NoDuplicates(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()

	// Create multiple probe dirs for same agent — should only appear once
	os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755)

	agents := detectPresentAgents(homeDir)
	count := 0
	for _, a := range agents {
		if a == "claude-code" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected 1 claude-code entry, got %d", count)
	}
}

func TestCheckMissingIntegrations(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	// Create .claude dir to simulate agent presence
	os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755)

	// No integration installed — should report claude-code as missing
	missing := checkMissingIntegrations(projectDir, homeDir)
	found := false
	for _, m := range missing {
		if m == "claude-code" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-code in missing list when .claude dir exists but no integration installed")
	}
}

func TestCheckMissingIntegrations_Installed(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	// Create .claude dir
	os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755)

	// Install the integration file with correct content
	sourceFiles := integrationMap["claude-code"]
	sourceContent, _ := integrationsFS.ReadFile(sourceFiles[0].source)
	dest := filepath.Join(projectDir, sourceFiles[0].dest)
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, sourceContent, 0o644)

	missing := checkMissingIntegrations(projectDir, homeDir)
	for _, m := range missing {
		if m == "claude-code" {
			t.Error("claude-code should not be missing when integration is installed")
		}
	}
}

func TestCheckMissingIntegrations_NoAgentsPresent(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	// Empty home and PATH: no agents detected, so nothing missing.
	if missing := checkMissingIntegrations(projectDir, homeDir); len(missing) != 0 {
		t.Errorf("checkMissingIntegrations = %v, want none", missing)
	}
}

// TestHintMissingIntegrations_RealDetection runs session.HintMissingIntegrations
// through the hooks wire.go installs, against real files, which the session
// package's own tests stub out.
func TestHintMissingIntegrations_RealDetection(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
		want      []string
	}{
		{name: "prints when none installed", want: []string{"codex detected", "hunkyard install codex"}},
		{name: "silent when an integration is installed", installed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HUNKYARD_NO_INTEGRATION_CHECK", "")
			bin := isolateAgentPATH(t)
			homeDir := t.TempDir()
			projectDir := t.TempDir()
			testutil.SetHome(t, homeDir)
			t.Chdir(projectDir)

			// A codex binary on PATH makes codex a detected-but-missing agent.
			writeStubBinary(t, bin, "codex", "codex-cli 1.0.0")
			if tt.installed {
				src := integrationMap["claude-code"][0]
				content, err := integrationsFS.ReadFile(src.source)
				if err != nil {
					t.Fatal(err)
				}
				testutil.WriteFile(t, filepath.Join(projectDir, src.dest), string(content))
			}

			out := captureStderr(t, session.HintMissingIntegrations)
			if len(tt.want) == 0 && out != "" {
				t.Errorf("expected no hint, got:\n%s", out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("hint missing %q, got:\n%s", w, out)
				}
			}
		})
	}
}

func TestInstalledAgents(t *testing.T) {
	isolateAgentPATH(t)
	homeDir := t.TempDir()
	projectDir := t.TempDir()

	// Empty — no agents installed
	agents := installedAgents(projectDir, homeDir)
	if len(agents) != 0 {
		t.Errorf("expected 0 installed agents, got %d", len(agents))
	}

	// Install claude-code
	sourceFiles := integrationMap["claude-code"]
	sourceContent, _ := integrationsFS.ReadFile(sourceFiles[0].source)
	dest := filepath.Join(projectDir, sourceFiles[0].dest)
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.WriteFile(dest, sourceContent, 0o644)

	agents = installedAgents(projectDir, homeDir)
	if !agents["claude-code"] {
		t.Error("expected claude-code in installed agents")
	}
}

func TestPrintMissingHints(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if n := printMissingHints(nil); n != 0 {
			t.Errorf("expected 0, got %d", n)
		}
	})
	t.Run("single", func(t *testing.T) {
		if n := printMissingHints([]string{"claude-code"}); n != 1 {
			t.Errorf("expected 1, got %d", n)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		if n := printMissingHints([]string{"claude-code", "codex"}); n != 2 {
			t.Errorf("expected 2, got %d", n)
		}
	})
}
