package main

//go:generate go run gen_integration_hashes.go

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/localonly"
)

// computeFileHash returns the hex-encoded SHA256 hash of data.
func computeFileHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// latestCacheDir returns the lexicographically last subdirectory name
// inside dir, or "" if dir doesn't exist or has no subdirectories.
// Version directories sort correctly by string comparison (e.g. "1.0.1" > "1.0.0").
func latestCacheDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() > latest {
			latest = entry.Name()
		}
	}
	return latest
}

// location describes where a stale file was found, determining the update advice.
const (
	locationProject     = "project"     // ./  (hunkyard install)
	locationHome        = "home"        // ~/  (hunkyard install from home)
	locationMarketplace = "marketplace" // ~/.claude/plugins/marketplaces/hunkyard/
	locationCache       = "cache"       // ~/.claude/plugins/cache/hunkyard/
)

type staleFile struct {
	agent    string // e.g. "claude-code"
	file     string // source file name
	dest     string // absolute path where the stale file was found
	location string // one of the location* constants
	hash     string // expected (bundled) content hash for this integration source
}

// toolDirFromDest extracts the tool config directory from a dest path
// (e.g. ".claude/skills/hunkyard/SKILL.md" → ".claude").
func toolDirFromDest(dest string) string {
	return strings.SplitN(dest, "/", 2)[0]
}

// marketplaceUpdateHint returns tool-specific advice for updating a marketplace plugin.
var marketplaceUpdateHints = map[string]string{
	".claude": "claude plugin marketplace update hunkyard\nclaude plugin update hunkyard@hunkyard",
}

// updateHint returns location-specific advice for how to fix this stale file.
func (s staleFile) updateHint() string {
	switch s.location {
	case locationProject:
		return fmt.Sprintf("Run: hunkyard install %s --force", s.agent)
	case locationHome:
		return fmt.Sprintf("Run: cd ~ && hunkyard install %s --force", s.agent)
	case locationMarketplace, locationCache:
		if s.agent == "codex-plugin" {
			return "Run: hunkyard install codex-plugin --force"
		}
		// Find the tool dir from the integration's dest path
		if files, ok := integrationMap[s.agent]; ok && len(files) > 0 {
			toolDir := toolDirFromDest(files[0].dest)
			if hint, ok := marketplaceUpdateHints[toolDir]; ok {
				return hint
			}
		}
		return "Update the hunkyard plugin in your editor settings"
	default:
		return fmt.Sprintf("Run: hunkyard install %s --force", s.agent)
	}
}

// integrationStatus describes a detected integration and whether it is current.
type integrationStatus struct {
	Agent    string `json:"agent"`
	Status   string `json:"status"`   // "current" or "stale"
	Location string `json:"location"` // "project", "home", "marketplace", "cache"
	Hint     string `json:"hint"`     // update hint (stale only)
	Hash     string `json:"hash,omitempty"`
}

// detectInstalledIntegrations scans all candidate paths for each agent
// and returns the status of every agent that has at least one file installed.
// Unlike checkInstalledIntegrations (which only returns stale files),
// this reports both current and stale agents.
func detectInstalledIntegrations(projectDir, homeDir string) []integrationStatus {
	var results []integrationStatus
	seen := make(map[string]bool)

	agents := make([]string, 0, len(integrationMap))
	for agent := range integrationMap {
		if localonly.Enabled() && agent != "codex" && agent != "claude-code" {
			continue
		}
		agents = append(agents, agent)
	}
	sort.Strings(agents)

	for _, agent := range agents {
		if seen[agent] {
			continue
		}
		files := integrationMap[agent]
		for _, f := range files {
			expectedHash, ok := integrationHashes[f.source]
			if !ok {
				continue
			}

			candidates := buildCandidates(f, agent, projectDir, homeDir)

			for _, c := range candidates {
				installed, err := os.ReadFile(c.path)
				if err != nil {
					continue
				}
				status := "current"
				hint := ""
				if computeFileHash(installed) != expectedHash {
					status = "stale"
					sf := staleFile{agent: agent, file: filepath.Base(f.dest), dest: c.path, location: c.location, hash: expectedHash}
					hint = sf.updateHint()
				}
				results = append(results, integrationStatus{
					Agent:    agent,
					Status:   status,
					Location: c.location,
					Hint:     hint,
					Hash:     expectedHash,
				})
				seen[agent] = true
				break // first found file per agent is enough
			}
			if seen[agent] {
				break // found this agent, move to next
			}
		}
	}
	return results
}

// candidate is a path + location pair for integration file lookup.
type candidate struct {
	path     string
	location string
}

// buildCandidates returns the list of candidate paths to check for an integration file.
func buildCandidates(f integration, agent, projectDir, homeDir string) []candidate {
	homePath := filepath.Join(homeDir, f.dest)
	if configPath, ok := claudeConfigDirPath(f.dest); ok {
		homePath = configPath
	}
	if f.globalDest != "" {
		if resolved, err := resolveGlobalDest(f.globalDestKind, f.globalDest, homeDir); err == nil {
			homePath = resolved
		}
	}

	candidates := []candidate{
		{filepath.Join(projectDir, f.dest), locationProject},
		{homePath, locationHome},
	}

	if agent == "codex-plugin" {
		candidates = append(candidates, codexPluginCacheCandidates(f, projectDir, homeDir)...)
		return candidates
	}

	toolDir := toolDirFromDest(f.dest)
	marketplacePath := filepath.Join(homeDir, toolDir, "plugins", "marketplaces", "hunkyard", f.source)
	candidates = append(candidates, candidate{marketplacePath, locationMarketplace})

	agentPrefix := fmt.Sprintf("integrations/%s/", agent)
	if strings.HasPrefix(f.source, agentPrefix) {
		relPath := strings.TrimPrefix(f.source, agentPrefix)
		cacheBase := filepath.Join(homeDir, toolDir, "plugins", "cache", "hunkyard", "hunkyard")
		if latest := latestCacheDir(cacheBase); latest != "" {
			cachePath := filepath.Join(cacheBase, latest, relPath)
			candidates = append(candidates, candidate{cachePath, locationCache})
		}
	}

	return candidates
}

func codexPluginCacheCandidates(f integration, projectDir, homeDir string) []candidate {
	const sourcePrefix = "integrations/codex/plugin/hunkyard/"
	relPath, ok := strings.CutPrefix(f.source, sourcePrefix)
	if !ok {
		return nil
	}

	var candidates []candidate
	for _, marketplaceName := range codexPluginMarketplaceNames(projectDir, homeDir) {
		cacheBase := filepath.Join(codexHome(homeDir), "plugins", "cache", marketplaceName, "hunkyard")
		latest := latestCacheDir(cacheBase)
		if latest == "" {
			continue
		}
		candidates = append(candidates, candidate{filepath.Join(cacheBase, latest, relPath), locationCache})
	}
	return candidates
}

func codexPluginMarketplaceNames(projectDir, homeDir string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}

	if name, ok := readCodexPluginMarketplaceName(filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), "./plugins/hunkyard"); ok {
		add(name)
	}
	if name, ok := readCodexPluginMarketplaceName(filepath.Join(homeDir, ".agents", "plugins", "marketplace.json"), "./.codex/plugins/hunkyard"); ok {
		add(name)
	}
	add("local")
	return names
}

func readCodexPluginMarketplaceName(path, sourcePath string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var marketplace codexPluginMarketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return "", false
	}
	for _, plugin := range marketplace.Plugins {
		if plugin.Name == "hunkyard" && plugin.Source.matchesLocalPath(sourcePath) {
			if marketplace.Name == "" {
				return "local", true
			}
			name, err := validCodexPluginSegment(marketplace.Name, "Codex marketplace name")
			if err != nil {
				return "", false
			}
			return name, true
		}
	}
	return "", false
}

// checkInstalledIntegrations scans known integration destinations for files
// that exist but differ from the precomputed hash in integrationHashes.
// Checks project-local, home dir, marketplace source, and marketplace cache
// files. For Codex plugins it also validates the marketplace/config/cache
// records Codex needs before a copied plugin can load.
func checkInstalledIntegrations(projectDir, homeDir string) []staleFile {
	var results []staleFile

	// Sort agents for deterministic output order
	agents := make([]string, 0, len(integrationMap))
	for agent := range integrationMap {
		agents = append(agents, agent)
	}
	sort.Strings(agents)

	for _, agent := range agents {
		files := integrationMap[agent]
		for _, f := range files {
			expectedHash, ok := integrationHashes[f.source]
			if !ok {
				continue
			}

			candidates := buildCandidates(f, agent, projectDir, homeDir)

			for _, c := range candidates {
				installed, err := os.ReadFile(c.path)
				if err != nil {
					continue
				}
				if computeFileHash(installed) != expectedHash {
					results = append(results, staleFile{
						agent:    agent,
						file:     filepath.Base(f.dest),
						dest:     c.path,
						location: c.location,
						hash:     expectedHash,
					})
				}
			}
		}
	}
	results = append(results, checkCodexPluginInstallCompleteness(projectDir, homeDir)...)
	return results
}

func checkCodexPluginInstallCompleteness(projectDir, homeDir string) []staleFile {
	type installRoot struct {
		root            string
		marketplacePath string
		sourcePath      string
		location        string
	}
	roots := []installRoot{
		{
			root:            filepath.Join(projectDir, "plugins", "hunkyard"),
			marketplacePath: filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"),
			sourcePath:      "./plugins/hunkyard",
			location:        locationProject,
		},
		{
			root:            filepath.Join(homeDir, ".codex", "plugins", "hunkyard"),
			marketplacePath: filepath.Join(homeDir, ".agents", "plugins", "marketplace.json"),
			sourcePath:      "./.codex/plugins/hunkyard",
			location:        locationHome,
		},
	}

	var results []staleFile
	for _, root := range roots {
		if _, err := os.Stat(filepath.Join(root.root, ".codex-plugin", "plugin.json")); err != nil {
			continue
		}

		marketplaceName, ok := readCodexPluginMarketplaceName(root.marketplacePath, root.sourcePath)
		if !ok {
			results = append(results, staleFile{
				agent:    "codex-plugin",
				file:     "marketplace.json",
				dest:     root.marketplacePath,
				location: root.location,
			})
			marketplaceName = "local"
		}

		pluginKey := "hunkyard@" + marketplaceName
		configPath := filepath.Join(codexHome(homeDir), "config.toml")
		if !codexPluginConfigReady(configPath, pluginKey) {
			results = append(results, staleFile{
				agent:    "codex-plugin",
				file:     "config.toml",
				dest:     configPath,
				location: root.location,
			})
		}

		version, err := codexPluginManifestVersion(root.root)
		if err != nil {
			continue
		}
		cacheManifest := filepath.Join(codexHome(homeDir), "plugins", "cache", marketplaceName, "hunkyard", version, ".codex-plugin", "plugin.json")
		if _, err := os.Stat(cacheManifest); err != nil {
			results = append(results, staleFile{
				agent:    "codex-plugin",
				file:     "plugin.json",
				dest:     cacheManifest,
				location: locationCache,
			})
		}
	}
	return results
}

func codexPluginConfigReady(path, pluginKey string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return codexPluginConfigReadyRaw(string(data), pluginKey)
}

func codexPluginConfigReadyRaw(raw, pluginKey string) bool {
	return tomlBoolEnabledRaw(raw, "features", "plugins") &&
		tomlBoolEnabledRaw(raw, "features", "hooks") &&
		tomlBoolEnabledRaw(raw, "features", "plugin_hooks") &&
		tomlBoolEnabledRaw(raw, fmt.Sprintf("plugins.%q", pluginKey), "enabled")
}

func tomlBoolEnabledRaw(raw, table, key string) bool {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !tomlTableMatches(line, table) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if _, ok := tomlTableName(lines[j]); ok {
				return false
			}
			trimmed := strings.TrimSpace(lines[j])
			if !tomlKeyIs(lines[j], key) {
				continue
			}
			_, value, ok := strings.Cut(trimmed, "=")
			value, _, _ = strings.Cut(value, "#")
			return ok && strings.TrimSpace(value) == "true"
		}
		return false
	}
	return false
}

// printStaleWarnings prints location-specific warnings for stale integrations
// to stderr. Returns the number of unique warnings printed.
func printStaleWarnings(stale []staleFile) int {
	if len(stale) == 0 {
		return 0
	}

	// Deduplicate by agent+location — one warning per unique combo
	type key struct{ agent, location string }
	seen := make(map[key]bool)
	for _, s := range stale {
		k := key{s.agent, s.location}
		if seen[k] {
			continue
		}
		seen[k] = true
		fmt.Fprintf(os.Stderr, "Note: %s integration outdated (%s). %s\n", s.agent, s.dest, strings.ReplaceAll(s.updateHint(), "|", ": "))
	}
	return len(seen)
}

// agentProbe describes how to detect whether an AI coding tool is present on
// the system. Checked in order: CLI binary on PATH, then config directory in
// $HOME.
type agentProbe struct {
	agent    string   // integration name (key in integrationMap)
	bins     []string // CLI binary names to look for on PATH
	homeDirs []string // directories relative to $HOME whose existence signals the agent
}

var agentProbes = []agentProbe{
	{"claude-code", []string{"claude"}, []string{".claude"}},
	{"codex", []string{"codex"}, nil},
}

// detectPresentAgents returns the names of AI coding tools that appear to be
// installed on the system (binary on PATH or config dir in $HOME).
func detectPresentAgents(homeDir string) []string {
	var present []string
	for _, p := range agentProbes {
		if agentProbePresent(p, homeDir) {
			present = append(present, p.agent)
		}
	}
	return present
}

func agentProbePresent(p agentProbe, homeDir string) bool {
	for _, bin := range p.bins {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	for _, dir := range p.homeDirs {
		if fi, err := os.Stat(filepath.Join(homeDir, dir)); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

// installedAgents returns the set of agents that have at least one hunkyard
// integration file installed (project-local, home, marketplace, or cache).
func installedAgents(projectDir, homeDir string) map[string]bool {
	installed := make(map[string]bool)
	for _, s := range detectInstalledIntegrations(projectDir, homeDir) {
		installed[s.Agent] = true
	}
	return installed
}

// checkMissingIntegrations returns agents that are present on the system but
// have no hunkyard integration installed (project-local or home).
func checkMissingIntegrations(projectDir, homeDir string) []string {
	present := detectPresentAgents(homeDir)
	if len(present) == 0 {
		return nil
	}

	installed := installedAgents(projectDir, homeDir)

	var missing []string
	for _, agent := range present {
		if !installed[agent] {
			missing = append(missing, agent)
		}
	}
	return missing
}

// printMissingHints prints a suggestion for each detected-but-not-installed
// agent. Returns the number of hints printed.
func printMissingHints(missing []string) int {
	if len(missing) == 0 {
		return 0
	}
	if len(missing) == 1 {
		fmt.Fprintf(os.Stderr, "Tip: %s detected but hunkyard integration not installed.\n", missing[0])
		fmt.Fprintf(os.Stderr, "     Run: hunkyard install %s\n", missing[0])
	} else {
		fmt.Fprintf(os.Stderr, "Tip: detected AI tools without hunkyard integration: %s\n", strings.Join(missing, ", "))
		fmt.Fprintf(os.Stderr, "     Run: hunkyard install all  (or hunkyard install <agent> for a specific one)\n")
	}
	fmt.Fprintf(os.Stderr, "     Disable: HUNKYARD_NO_INTEGRATION_CHECK=1\n")
	return len(missing)
}

// runCheck implements the "hunkyard check" subcommand.
func runCheck() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "hunkyard %s — checking installed integrations...\n\n", version)

	stale := checkInstalledIntegrations(cwd, home)
	missing := checkMissingIntegrations(cwd, home)
	forgeCLIs := checkForgeCLIs()

	if len(stale) == 0 && len(missing) == 0 {
		fmt.Fprintln(os.Stderr, "All installed integrations are up to date.")
	} else if len(stale) > 0 {
		// Deduplicate by hint — show each unique update action only once
		seenHints := make(map[string]bool)
		for _, s := range stale {
			hint := s.updateHint()
			if seenHints[hint] {
				continue
			}
			seenHints[hint] = true
			fmt.Fprintf(os.Stderr, "  outdated: %s\n", s.dest)
			termHint := strings.ReplaceAll(hint, "|", ": ")
			fmt.Fprintf(os.Stderr, "    → %s\n", termHint)
		}
	}

	if len(missing) > 0 {
		if len(stale) > 0 {
			fmt.Fprintln(os.Stderr)
		}
		printMissingHints(missing)
	}

	fmt.Fprintln(os.Stderr)
	printForgeCLIHints(forgeCLIs)
}

type forgeCLIStatus struct {
	name          string
	binary        string
	loginHint     string
	authenticated bool
	detail        string
}

var forgeCLIProbes = []struct {
	name      string
	binary    string
	authArgs  []string
	loginHint string
}{
	{name: "GitHub", binary: "gh", authArgs: []string{"auth", "status"}, loginHint: "gh auth login"},
}

// checkForgeCLIs returns installed forge CLIs and their authentication state.
// Rendering stays separate so detection can be reused without terminal output.
func checkForgeCLIs() []forgeCLIStatus {
	if localonly.Enabled() {
		return nil
	}
	var present []forgeCLIStatus
	for _, probe := range forgeCLIProbes {
		if _, err := exec.LookPath(probe.binary); err != nil {
			continue
		}
		status := forgeCLIStatus{name: probe.name, binary: probe.binary, loginHint: probe.loginHint}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, probe.binary, probe.authArgs...).CombinedOutput()
		cancel()
		status.authenticated = err == nil
		if err != nil {
			status.detail = strings.TrimSpace(string(out))
			if status.detail != "" {
				status.detail = strings.Split(status.detail, "\n")[0]
			}
		}
		present = append(present, status)
	}
	return present
}

func printForgeCLIHints(present []forgeCLIStatus) {
	if localonly.Enabled() {
		fmt.Fprintln(os.Stderr, "Local review only; remote providers are disabled.")
		return
	}
	fmt.Fprintln(os.Stderr, "Remote review providers:")
	byBinary := make(map[string]forgeCLIStatus, len(present))
	for _, status := range present {
		byBinary[status.binary] = status
	}
	for _, probe := range forgeCLIProbes {
		status, ok := byBinary[probe.binary]
		if !ok {
			fmt.Fprintf(os.Stderr, "  unavailable: %s (%s is not installed)\n", probe.name, probe.binary)
			continue
		}
		if !status.authenticated {
			detail := ""
			if status.detail != "" {
				detail = ": " + status.detail
			}
			fmt.Fprintf(os.Stderr, "  needs login: %s (run %s)%s\n", status.name, status.loginHint, detail)
			continue
		}
		fmt.Fprintf(os.Stderr, "  ready: %s (%s)\n", status.name, status.binary)
	}
}
