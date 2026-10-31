package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BSH2409/hunkyard/internal/prompt"
)

func runInstall(args []string) {
	target, force, global := parseInstallArgs(args)
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			err = os.Chdir(home)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if target == "" {
		printInstallUsage()
		os.Exit(1)
	}
	if err := runInstallTarget(target, force); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}

func parseInstallArgs(args []string) (target string, force, global bool) {
	for _, a := range args {
		switch {
		case a == "--global" || a == "-g":
			global = true
		case a == "--force" || a == "-f":
			force = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", a)
			os.Exit(1)
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "Error: only one target allowed (got %q and %q)\n", target, a)
				os.Exit(1)
			}
			target = a
		}
	}
	return target, force, global
}

func printInstallUsage() {
	fmt.Fprintln(os.Stderr, "Usage: hunkyard install [--global|-g] [--force|-f] <agent|all|preferences|prompts|story-prompts>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -g, --global Install for the user, independent of the current repository")
	fmt.Fprintln(os.Stderr, "  -f, --force  Overwrite existing integration files")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Available agents:")
	for _, a := range availableIntegrations() {
		fmt.Fprintf(os.Stderr, "  %s\n", a)
	}
	fmt.Fprintln(os.Stderr, "  all")
	fmt.Fprintln(os.Stderr, "  prompts")
	fmt.Fprintln(os.Stderr, "  story-prompts")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Install stock finish templates:")
	fmt.Fprintln(os.Stderr, "  cd ~ && hunkyard install prompts     # ~/.hunkyard/prompts/")
	fmt.Fprintln(os.Stderr, "  hunkyard install prompts             # .hunkyard/prompts/ in cwd (from repo root)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Install the stock on_story_generate template:")
	fmt.Fprintln(os.Stderr, "  cd ~ && hunkyard install story-prompts   # ~/.hunkyard/prompts/")
	fmt.Fprintln(os.Stderr, "  hunkyard install story-prompts           # .hunkyard/prompts/ in cwd (from repo root)")
}

func runInstallTarget(target string, force bool) error {
	if target == "claude" {
		target = "claude-code"
	}
	if target == "prompts" {
		return installPrompts(force)
	}
	if target == "story-prompts" {
		return installStoryPrompts(force)
	}
	if target == "all" {
		return installAllIntegrations(force)
	}
	return installIntegration(target, force)
}

func installAllIntegrations(force bool) error {
	var hadErr bool
	for _, name := range availableIntegrations() {
		if err := installIntegration(name, force); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed: %s: %v\n", name, err)
			hadErr = true
			continue
		}
	}
	if hadErr {
		return errors.New("one or more integrations failed to install")
	}
	return nil
}

func installPrompts(force bool) error {
	cwd := mustGetwd()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(cwd, ".hunkyard", "prompts")
	if isGlobalInstall(cwd, home) {
		dest = filepath.Join(home, ".hunkyard", "prompts")
	}
	fmt.Printf("Installing stock finish prompts to %s\n", dest)
	return prompt.InstallPrompts(dest, force)
}

func installStoryPrompts(force bool) error {
	cwd := mustGetwd()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(cwd, ".hunkyard", "prompts")
	if isGlobalInstall(cwd, home) {
		dest = filepath.Join(home, ".hunkyard", "prompts")
	}
	fmt.Printf("Installing stock story prompt to %s\n", dest)
	return prompt.InstallStoryPrompt(dest, force)
}

// globalDestKind selects how an integration's globalDest is interpreted.
type globalDestKind int

const (
	// globalDestNone means the integration has no separate global path —
	// use dest joined to home (the default for global installs).
	globalDestNone globalDestKind = iota
	// globalDestRelHome: globalDest is relative to $HOME.
	globalDestRelHome
)

type integration struct {
	source string // path inside integrations/ embed
	dest   string // destination relative to cwd
	hint   string // usage hint printed after install
	// globalDest, when set together with a non-zero globalDestKind, overrides
	// dest in global mode (cwd == $HOME). The kind determines how it's
	// resolved (see globalDestKind).
	globalDest     string
	globalDestKind globalDestKind
}

var integrationMap = map[string][]integration{
	"preferences": {
		{source: "integrations/preferences/SKILL.md", dest: ".agents/skills/learn-preferences/SKILL.md", hint: "The Go-backed learn-preferences skill is available in Codex and Claude"},
		{source: "integrations/preferences/references/local-maintenance.md", dest: ".agents/skills/learn-preferences/references/local-maintenance.md"},
		{source: "integrations/preferences/schemas/maintenance-proposal.json", dest: ".agents/skills/learn-preferences/schemas/maintenance-proposal.json"},
		{source: "integrations/preferences/SKILL.md", dest: ".claude/skills/learn-preferences/SKILL.md"},
		{source: "integrations/preferences/references/local-maintenance.md", dest: ".claude/skills/learn-preferences/references/local-maintenance.md"},
		{source: "integrations/preferences/schemas/maintenance-proposal.json", dest: ".claude/skills/learn-preferences/schemas/maintenance-proposal.json"},
	},
	"claude-code": {
		{source: "integrations/claude-code/skills/hunkyard/SKILL.md", dest: ".claude/skills/hunkyard/SKILL.md", hint: "Run /hunkyard in Claude Code to start a review loop"},
		{source: "integrations/claude-code/skills/hunkyard-cli/SKILL.md", dest: ".claude/skills/hunkyard-cli/SKILL.md", hint: "The hunkyard-cli skill is available to Claude Code agents when needed"},
		{source: "integrations/claude-code/skills/hunkyard-story/SKILL.md", dest: ".claude/skills/hunkyard-story/SKILL.md", hint: "Run /hunkyard-story in Claude Code to author a story and continue the review loop"},
		{source: "integrations/claude-code/skills/hunkyard/references/review-workflow.md", dest: ".claude/skills/hunkyard/references/review-workflow.md"},
	},
	"codex": {
		{source: "integrations/codex/skills/hunkyard/SKILL.md", dest: ".agents/skills/hunkyard/SKILL.md", hint: "Use $hunkyard in Codex to start a review loop"},
		{source: "integrations/codex/skills/hunkyard-cli/SKILL.md", dest: ".agents/skills/hunkyard-cli/SKILL.md", hint: "The hunkyard-cli skill is available to Codex agents when needed"},
		{source: "integrations/codex/skills/hunkyard-story/SKILL.md", dest: ".agents/skills/hunkyard-story/SKILL.md", hint: "Use $hunkyard-story in Codex to author a story and continue the review loop"},
		{source: "integrations/codex/skills/hunkyard/references/review-workflow.md", dest: ".agents/skills/hunkyard/references/review-workflow.md"},
	},
	"codex-plugin": {
		{source: "integrations/codex/plugin/hunkyard/.codex-plugin/plugin.json", dest: "plugins/hunkyard/.codex-plugin/plugin.json", globalDest: ".codex/plugins/hunkyard/.codex-plugin/plugin.json", globalDestKind: globalDestRelHome, hint: "The Hunkyard plugin is registered in the local Codex plugin marketplace"},
		{source: "integrations/codex/plugin/hunkyard/skills/hunkyard/SKILL.md", dest: "plugins/hunkyard/skills/hunkyard/SKILL.md", globalDest: ".codex/plugins/hunkyard/skills/hunkyard/SKILL.md", globalDestKind: globalDestRelHome, hint: "The plugin-packaged hunkyard skill is available to Codex as $hunkyard"},
		{source: "integrations/codex/plugin/hunkyard/skills/hunkyard-cli/SKILL.md", dest: "plugins/hunkyard/skills/hunkyard-cli/SKILL.md", globalDest: ".codex/plugins/hunkyard/skills/hunkyard-cli/SKILL.md", globalDestKind: globalDestRelHome, hint: "The plugin-packaged hunkyard-cli skill is available to Codex agents when needed"},
		{source: "integrations/codex/plugin/hunkyard/skills/hunkyard-story/SKILL.md", dest: "plugins/hunkyard/skills/hunkyard-story/SKILL.md", globalDest: ".codex/plugins/hunkyard/skills/hunkyard-story/SKILL.md", globalDestKind: globalDestRelHome, hint: "The plugin-packaged hunkyard-story skill is available to Codex as $hunkyard-story"},
		{source: "integrations/codex/plugin/hunkyard/hooks/hooks.json", dest: "plugins/hunkyard/hooks/hooks.json", globalDest: ".codex/plugins/hunkyard/hooks/hooks.json", globalDestKind: globalDestRelHome, hint: "The Hunkyard plugin uses explicit review commands; automatic plan hooks are disabled"},
		{source: "integrations/codex/plugin/hunkyard/skills/hunkyard/references/review-workflow.md", dest: "plugins/hunkyard/skills/hunkyard/references/review-workflow.md", globalDest: ".codex/plugins/hunkyard/skills/hunkyard/references/review-workflow.md", globalDestKind: globalDestRelHome},
		{source: "integrations/codex/plugin/hunkyard/skills/learn-preferences/SKILL.md", dest: "plugins/hunkyard/skills/learn-preferences/SKILL.md", globalDest: ".codex/plugins/hunkyard/skills/learn-preferences/SKILL.md", globalDestKind: globalDestRelHome},
		{source: "integrations/codex/plugin/hunkyard/skills/learn-preferences/references/local-maintenance.md", dest: "plugins/hunkyard/skills/learn-preferences/references/local-maintenance.md", globalDest: ".codex/plugins/hunkyard/skills/learn-preferences/references/local-maintenance.md", globalDestKind: globalDestRelHome},
		{source: "integrations/codex/plugin/hunkyard/skills/learn-preferences/schemas/maintenance-proposal.json", dest: "plugins/hunkyard/skills/learn-preferences/schemas/maintenance-proposal.json", globalDest: ".codex/plugins/hunkyard/skills/learn-preferences/schemas/maintenance-proposal.json", globalDestKind: globalDestRelHome},
	},
}

// legacyIntegrationFile describes an obsolete destination that could keep
// auto-invoking Hunkyard after an upgrade. Cleanup only removes byte-for-byte known
// Hunkyard-managed content; user-edited files are preserved with a warning.
type legacyIntegrationFile struct {
	dest           string
	globalDest     string
	globalDestKind globalDestKind
	hash           string
}

var legacyIntegrationFiles = map[string][]legacyIntegrationFile{
	"codex": {
		{
			// Hard allow_implicit_invocation:false gate — replaced by skill wording.
			dest:           ".agents/skills/hunkyard/agents/openai.yaml",
			globalDest:     ".agents/skills/hunkyard/agents/openai.yaml",
			globalDestKind: globalDestRelHome,
			hash:           "a1499d95abd8447558c535fe5554adcc3c9b988a0a39264a6283d430effe1e94",
		},
	},
	"codex-plugin": {
		{
			dest:           "plugins/hunkyard/skills/hunkyard/agents/openai.yaml",
			globalDest:     ".codex/plugins/hunkyard/skills/hunkyard/agents/openai.yaml",
			globalDestKind: globalDestRelHome,
			hash:           "a1499d95abd8447558c535fe5554adcc3c9b988a0a39264a6283d430effe1e94",
		},
		{
			// Loose skills installed alongside the plugin (project and ~/.agents).
			dest:           ".agents/skills/hunkyard/agents/openai.yaml",
			globalDest:     ".agents/skills/hunkyard/agents/openai.yaml",
			globalDestKind: globalDestRelHome,
			hash:           "a1499d95abd8447558c535fe5554adcc3c9b988a0a39264a6283d430effe1e94",
		},
	},
}

// availableIntegrations returns the sorted list of integration names that
// `hunkyard install <name>` accepts, derived from integrationMap keys.
func availableIntegrations() []string {
	if localonly.Enabled() {
		return []string{"claude-code", "codex"}
	}
	names := make([]string, 0, len(integrationMap))
	for name := range integrationMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isGlobalInstall reports whether the install should be treated as global
// (user-wide) rather than project-scoped. True when cwd == $HOME.
func isGlobalInstall(cwd, home string) bool {
	if cwd == "" || home == "" {
		return false
	}
	a, errA := filepath.Abs(cwd)
	b, errB := filepath.Abs(home)
	if errA != nil || errB != nil {
		return cwd == home
	}
	return a == b
}

// resolveGlobalDest expands an integration's globalDest into an absolute
// path according to its globalDestKind.
func resolveGlobalDest(kind globalDestKind, globalDest, home string) (string, error) {
	switch kind {
	case globalDestRelHome, globalDestNone:
		if filepath.IsAbs(globalDest) {
			return globalDest, nil
		}
		return filepath.Join(home, globalDest), nil
	default:
		return "", fmt.Errorf("unknown globalDestKind %d", kind)
	}
}

// installIntegration installs the named agent integration. It returns an
// error suitable for printing to stderr; callers decide whether to exit or
// continue (the `install all` loop continues past per-agent failures).
func installIntegration(name string, force bool) error {
	files, ok := integrationMap[name]
	if !ok {
		var b strings.Builder
		fmt.Fprintf(&b, "unknown agent: %s\n\nAvailable agents:\n", name)
		for _, a := range availableIntegrations() {
			fmt.Fprintf(&b, "  %s\n", a)
		}
		return errors.New(strings.TrimRight(b.String(), "\n"))
	}
	cwd := mustGetwd()
	home, _ := os.UserHomeDir()
	global := isGlobalInstall(cwd, home)

	var hints []string
	codexMarketplaceName := ""
	if name == "codex-plugin" {
		var err error
		codexMarketplaceName, err = installCodexPluginMarketplace(codexPluginMarketplacePath(global, home), codexPluginMarketplaceSourcePath(global), force)
		if err != nil {
			return err
		}
		for _, f := range integrationMap["codex"] {
			dest := destFor(f, global, home, "codex")
			installOneFile(f, dest, force)
			if f.hint != "" {
				hints = append(hints, f.hint)
			}
		}
	}
	for _, f := range files {
		dest := destFor(f, global, home, name)
		installOneFile(f, dest, force || name == "codex-plugin")
		if f.hint != "" {
			hints = append(hints, f.hint)
		}
	}
	cleanupLegacyIntegrationFiles(name, global, home)
	if name == "codex-plugin" {
		if err := installCodexPluginActivation(global, home, codexMarketplaceName); err != nil {
			return err
		}
		hints = append(hints, "The Hunkyard Codex plugin is enabled as hunkyard@"+codexMarketplaceName)
	}
	printUniqueHints(hints)
	fmt.Println()
	return nil
}

func cleanupLegacyIntegrationFiles(name string, global bool, home string) {
	for _, legacy := range legacyIntegrationFiles[name] {
		dest := legacy.dest
		if global {
			if legacy.globalDest == "" {
				continue
			}
			var err error
			dest, err = resolveGlobalDest(legacy.globalDestKind, legacy.globalDest, home)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not resolve obsolete %s integration path: %v\n", name, err)
				continue
			}
		}

		data, err := os.ReadFile(dest)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not inspect obsolete Hunkyard integration %s: %v\n", dest, err)
			continue
		}
		if computeFileHash(data) != legacy.hash {
			fmt.Fprintf(os.Stderr, "Warning: preserving modified obsolete Hunkyard integration %s; remove it manually to prevent automatic invocation\n", dest)
			continue
		}
		if err := os.Remove(dest); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove obsolete Hunkyard integration %s: %v\n", dest, err)
			continue
		}
		fmt.Printf("  Removed:   %s (obsolete auto-invoked integration)\n", dest)
	}
}

// destFor returns the destination path for an integration file, accounting
// for global vs project install mode.
func destFor(f integration, global bool, home, name string) string {
	if global && f.globalDest != "" {
		resolved, err := resolveGlobalDest(f.globalDestKind, f.globalDest, home)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving global destination for %s: %v\n", name, err)
			os.Exit(1)
		}
		return resolved
	}
	if global {
		if dest, ok := claudeConfigDirPath(f.dest); ok {
			return dest
		}
	}
	return f.dest
}

// claudeConfigDirPath maps a ".claude/" dest into $CLAUDE_CONFIG_DIR, which
// Claude Code reads user-level skills from instead of ~/.claude when set.
func claudeConfigDirPath(dest string) (string, bool) {
	rest, ok := strings.CutPrefix(filepath.ToSlash(dest), ".claude/")
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if !ok || dir == "" {
		return "", false
	}
	return filepath.Join(dir, filepath.FromSlash(rest)), true
}

// symlinkedSkillDir returns the skill folder containing dest (the folder
// directly under a "skills" directory) when that folder is a symlink, so an
// install never writes through it into another tool's copy of the skill.
func symlinkedSkillDir(dest string) string {
	parts := strings.Split(filepath.ToSlash(dest), "/")
	for i := len(parts) - 2; i > 0; i-- {
		if parts[i-1] != "skills" {
			continue
		}
		dir := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return dir
		}
		return ""
	}
	return ""
}

// installOneFile copies a single embedded integration file to dest, skipping
// if it already exists (unless force is set). Exits on I/O errors.
func installOneFile(f integration, dest string, force bool) {
	if link := symlinkedSkillDir(dest); link != "" {
		if !force {
			fmt.Printf("  Skipped:   %s (skill folder is a symlink, use --force to replace it with a copy)\n", dest)
			return
		}
		if err := os.Remove(link); err != nil {
			fmt.Fprintf(os.Stderr, "Error replacing symlinked skill folder %s: %v\n", link, err)
			os.Exit(1)
		}
		fmt.Printf("  Replaced:  %s (was a symlink; installing a copy)\n", link)
	}
	if !force {
		if _, err := os.Stat(dest); err == nil {
			fmt.Printf("  Skipped:   %s (already exists, use --force to overwrite)\n", dest)
			return
		}
	}
	data, err := integrationsFS.ReadFile(f.source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading embedded file %s: %v\n", f.source, err)
		os.Exit(1)
	}
	if err := atomicWriteFile(dest, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", dest, err)
		os.Exit(1)
	}
	fmt.Printf("  Installed: %s\n", dest)
}

func codexPluginMarketplacePath(global bool, home string) string {
	if global {
		return filepath.Join(home, ".agents", "plugins", "marketplace.json")
	}
	return filepath.Join(".agents", "plugins", "marketplace.json")
}

func codexPluginMarketplaceSourcePath(global bool) string {
	if global {
		return "./.codex/plugins/hunkyard"
	}
	return "./plugins/hunkyard"
}

type codexPluginMarketplace struct {
	Name      string
	Interface codexPluginMarketplaceInterface
	Plugins   []codexMarketplacePlugin
	Extra     map[string]json.RawMessage
}

func (m *codexPluginMarketplace) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["name"]; ok {
		if err := json.Unmarshal(v, &m.Name); err != nil {
			return fmt.Errorf("name: %w", err)
		}
		delete(raw, "name")
	}
	if v, ok := raw["interface"]; ok {
		if err := json.Unmarshal(v, &m.Interface); err != nil {
			return fmt.Errorf("interface: %w", err)
		}
		delete(raw, "interface")
	}
	if v, ok := raw["plugins"]; ok {
		if err := json.Unmarshal(v, &m.Plugins); err != nil {
			return fmt.Errorf("plugins: %w", err)
		}
		delete(raw, "plugins")
	}
	m.Extra = raw
	return nil
}

func (m codexPluginMarketplace) MarshalJSON() ([]byte, error) {
	raw := cloneRawMessages(m.Extra)
	if m.Name != "" {
		data, err := json.Marshal(m.Name)
		if err != nil {
			return nil, err
		}
		raw["name"] = data
	}
	if !m.Interface.empty() {
		data, err := json.Marshal(m.Interface)
		if err != nil {
			return nil, err
		}
		raw["interface"] = data
	}
	if m.Plugins != nil {
		data, err := json.Marshal(m.Plugins)
		if err != nil {
			return nil, err
		}
		raw["plugins"] = data
	}
	return json.Marshal(raw)
}

type codexPluginMarketplaceInterface struct {
	DisplayName string
	Extra       map[string]json.RawMessage
}

func (i *codexPluginMarketplaceInterface) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["displayName"]; ok {
		if err := json.Unmarshal(v, &i.DisplayName); err != nil {
			return fmt.Errorf("displayName: %w", err)
		}
		delete(raw, "displayName")
	}
	i.Extra = raw
	return nil
}

func (i codexPluginMarketplaceInterface) MarshalJSON() ([]byte, error) {
	raw := cloneRawMessages(i.Extra)
	if i.DisplayName != "" {
		data, err := json.Marshal(i.DisplayName)
		if err != nil {
			return nil, err
		}
		raw["displayName"] = data
	}
	return json.Marshal(raw)
}

func (i codexPluginMarketplaceInterface) empty() bool {
	return i.DisplayName == "" && len(i.Extra) == 0
}

type codexMarketplacePlugin struct {
	Name     string
	Source   codexMarketplacePluginSource
	Policy   codexMarketplacePluginPolicy
	Category string
	Extra    map[string]json.RawMessage
}

func (p *codexMarketplacePlugin) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["name"]; ok {
		if err := json.Unmarshal(v, &p.Name); err != nil {
			return fmt.Errorf("name: %w", err)
		}
		delete(raw, "name")
	}
	if v, ok := raw["source"]; ok {
		if err := json.Unmarshal(v, &p.Source); err != nil {
			return fmt.Errorf("source: %w", err)
		}
		delete(raw, "source")
	}
	if v, ok := raw["policy"]; ok {
		if err := json.Unmarshal(v, &p.Policy); err != nil {
			return fmt.Errorf("policy: %w", err)
		}
		delete(raw, "policy")
	}
	if v, ok := raw["category"]; ok {
		if err := json.Unmarshal(v, &p.Category); err != nil {
			return fmt.Errorf("category: %w", err)
		}
		delete(raw, "category")
	}
	p.Extra = raw
	return nil
}

func (p codexMarketplacePlugin) MarshalJSON() ([]byte, error) {
	raw := cloneRawMessages(p.Extra)
	if p.Name != "" {
		data, err := json.Marshal(p.Name)
		if err != nil {
			return nil, err
		}
		raw["name"] = data
	}
	if !p.Source.empty() {
		data, err := json.Marshal(p.Source)
		if err != nil {
			return nil, err
		}
		raw["source"] = data
	}
	if !p.Policy.empty() {
		data, err := json.Marshal(p.Policy)
		if err != nil {
			return nil, err
		}
		raw["policy"] = data
	}
	if p.Category != "" {
		data, err := json.Marshal(p.Category)
		if err != nil {
			return nil, err
		}
		raw["category"] = data
	}
	return json.Marshal(raw)
}

func (p codexMarketplacePlugin) matchesDesired(desired codexMarketplacePlugin) bool {
	return p.Name == desired.Name &&
		p.Source.matches(desired.Source) &&
		p.Policy.Installation == desired.Policy.Installation &&
		p.Policy.Authentication == desired.Policy.Authentication &&
		p.Category == desired.Category
}

type codexMarketplacePluginSource struct {
	Shorthand string
	Source    string
	Path      string
	Extra     map[string]json.RawMessage
}

func (s *codexMarketplacePluginSource) UnmarshalJSON(data []byte) error {
	var shorthand string
	if err := json.Unmarshal(data, &shorthand); err == nil {
		s.Shorthand = shorthand
		return nil
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["source"]; ok {
		if err := json.Unmarshal(v, &s.Source); err != nil {
			return fmt.Errorf("source: %w", err)
		}
		delete(raw, "source")
	}
	if v, ok := raw["path"]; ok {
		if err := json.Unmarshal(v, &s.Path); err != nil {
			return fmt.Errorf("path: %w", err)
		}
		delete(raw, "path")
	}
	s.Extra = raw
	return nil
}

func (s codexMarketplacePluginSource) MarshalJSON() ([]byte, error) {
	if s.Shorthand != "" && s.Source == "" && s.Path == "" && len(s.Extra) == 0 {
		return json.Marshal(s.Shorthand)
	}

	raw := cloneRawMessages(s.Extra)
	if s.Source != "" {
		data, err := json.Marshal(s.Source)
		if err != nil {
			return nil, err
		}
		raw["source"] = data
	}
	if s.Path != "" {
		data, err := json.Marshal(s.Path)
		if err != nil {
			return nil, err
		}
		raw["path"] = data
	}
	return json.Marshal(raw)
}

func (s codexMarketplacePluginSource) empty() bool {
	return s.Shorthand == "" && s.Source == "" && s.Path == "" && len(s.Extra) == 0
}

func (s codexMarketplacePluginSource) matches(desired codexMarketplacePluginSource) bool {
	if s.Shorthand != "" {
		return desired.Source == "local" && s.Shorthand == desired.Path
	}
	return s.Source == desired.Source && s.Path == desired.Path
}

func (s codexMarketplacePluginSource) matchesLocalPath(path string) bool {
	if s.Shorthand != "" {
		return s.Shorthand == path
	}
	return s.Source == "local" && s.Path == path
}

type codexMarketplacePluginPolicy struct {
	Installation   string
	Authentication string
	Extra          map[string]json.RawMessage
}

func (p *codexMarketplacePluginPolicy) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["installation"]; ok {
		if err := json.Unmarshal(v, &p.Installation); err != nil {
			return fmt.Errorf("installation: %w", err)
		}
		delete(raw, "installation")
	}
	if v, ok := raw["authentication"]; ok {
		if err := json.Unmarshal(v, &p.Authentication); err != nil {
			return fmt.Errorf("authentication: %w", err)
		}
		delete(raw, "authentication")
	}
	p.Extra = raw
	return nil
}

func (p codexMarketplacePluginPolicy) MarshalJSON() ([]byte, error) {
	raw := cloneRawMessages(p.Extra)
	if p.Installation != "" {
		data, err := json.Marshal(p.Installation)
		if err != nil {
			return nil, err
		}
		raw["installation"] = data
	}
	if p.Authentication != "" {
		data, err := json.Marshal(p.Authentication)
		if err != nil {
			return nil, err
		}
		raw["authentication"] = data
	}
	return json.Marshal(raw)
}

func (p codexMarketplacePluginPolicy) empty() bool {
	return p.Installation == "" && p.Authentication == "" && len(p.Extra) == 0
}

func cloneRawMessages(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in)+4)
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func desiredCodexMarketplacePlugin(sourcePath string) codexMarketplacePlugin {
	return codexMarketplacePlugin{
		Name: "hunkyard",
		Source: codexMarketplacePluginSource{
			Source: "local",
			Path:   sourcePath,
		},
		Policy: codexMarketplacePluginPolicy{
			Installation:   "INSTALLED_BY_DEFAULT",
			Authentication: "ON_INSTALL",
		},
		Category: "Developer Tools",
	}
}

//nolint:gocyclo // marketplace read-merge-write has inherent branching for force/exists/valid/changed
func installCodexPluginMarketplace(path, sourcePath string, force bool) (string, error) {
	existing := codexPluginMarketplace{}
	changed := false
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			if !force {
				return "", fmt.Errorf("%s contains invalid JSON - use --force to overwrite", path)
			}
			existing = codexPluginMarketplace{}
			changed = true
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	marketplaceName := existing.Name
	if marketplaceName == "" {
		marketplaceName = "local"
		existing.Name = "local"
		changed = true
	}
	if validated, err := validCodexPluginSegment(marketplaceName, "Codex marketplace name"); err != nil {
		if !force {
			return "", err
		}
		marketplaceName = "local"
		existing.Name = "local"
		changed = true
	} else {
		marketplaceName = validated
	}
	if strings.TrimSpace(existing.Interface.DisplayName) == "" {
		existing.Interface.DisplayName = "Local Plugins"
		changed = true
	}

	entry := desiredCodexMarketplacePlugin(sourcePath)
	next := make([]codexMarketplacePlugin, 0, len(existing.Plugins)+1)
	found := false
	for _, plugin := range existing.Plugins {
		if plugin.Name != "hunkyard" {
			next = append(next, plugin)
			continue
		}

		if found {
			changed = true
			continue
		}
		found = true

		if !force && plugin.matchesDesired(entry) {
			next = append(next, plugin)
			continue
		}
		next = append(next, entry)
		changed = true
	}

	if !found {
		next = append(next, entry)
		changed = true
	}
	if !changed {
		fmt.Printf("  Skipped:   %s (Hunkyard plugin already registered, use --force to overwrite)\n", path)
		return marketplaceName, nil
	}
	existing.Plugins = next
	return marketplaceName, writeCodexPluginMarketplace(path, existing)
}

func writeCodexPluginMarketplace(path string, marketplace codexPluginMarketplace) error {
	data, err := json.MarshalIndent(marketplace, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := atomicWriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("  Installed: %s\n", path)
	return nil
}

func codexHome(home string) string {
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		if home == "" || samePath(home, currentUserHome()) {
			return env
		}
	}
	return filepath.Join(home, ".codex")
}

func currentUserHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return absA == absB
}

const codexPluginEmbeddedPrefix = "integrations/codex/plugin/hunkyard/"

//nolint:unparam // global is passed for symmetry with other install* funcs; activation always targets ~/.codex
func installCodexPluginActivation(global bool, home, marketplaceName string) error {
	if marketplaceName == "" {
		marketplaceName = "local"
	}
	marketplaceName, err := validCodexPluginSegment(marketplaceName, "Codex marketplace name")
	if err != nil {
		return err
	}

	version, err := codexPluginEmbeddedManifestVersion()
	if err != nil {
		return err
	}

	codexRoot := codexHome(home)
	cacheBase := filepath.Join(codexRoot, "plugins", "cache")
	cacheRoot := filepath.Join(cacheBase, marketplaceName, "hunkyard", version)
	if err := requirePathWithin(cacheBase, cacheRoot); err != nil {
		return err
	}
	if err := copyCodexPluginCacheFromEmbedded(cacheRoot); err != nil {
		return fmt.Errorf("installing Codex plugin cache: %w", err)
	}
	fmt.Printf("  Installed: %s\n", cacheRoot)

	configPath := filepath.Join(codexRoot, "config.toml")
	if err := installCodexPluginConfig(configPath, "hunkyard@"+marketplaceName); err != nil {
		return err
	}
	return nil
}

func codexPluginManifestVersion(sourceRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(sourceRoot, ".codex-plugin", "plugin.json"))
	if err != nil {
		return "", fmt.Errorf("reading Codex plugin manifest: %w", err)
	}
	return codexPluginManifestVersionFromBytes(data)
}

func codexPluginEmbeddedManifestVersion() (string, error) {
	data, err := integrationsFS.ReadFile(codexPluginEmbeddedPrefix + ".codex-plugin/plugin.json")
	if err != nil {
		return "", fmt.Errorf("reading embedded Codex plugin manifest: %w", err)
	}
	return codexPluginManifestVersionFromBytes(data)
}

func codexPluginManifestVersionFromBytes(data []byte) (string, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parsing Codex plugin manifest: %w", err)
	}
	if manifest.Version != "" {
		return validCodexPluginSegment(manifest.Version, "Codex plugin version")
	}
	return "local", nil
}

func copyCodexPluginCacheFromEmbedded(dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	for _, f := range integrationMap["codex-plugin"] {
		rel, ok := strings.CutPrefix(f.source, codexPluginEmbeddedPrefix)
		if !ok {
			return fmt.Errorf("embedded plugin source %s is outside %s", f.source, codexPluginEmbeddedPrefix)
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := requirePathWithin(dst, target); err != nil {
			return err
		}
		data, err := integrationsFS.ReadFile(f.source)
		if err != nil {
			return fmt.Errorf("reading embedded plugin file %s: %w", f.source, err)
		}
		if err := atomicWriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
	}
	return nil
}

func validCodexPluginSegment(value, label string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", label)
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "", fmt.Errorf("%s %q must contain only ASCII letters, digits, `_`, and `-`", label, value)
	}
	return value, nil
}

func requirePathWithin(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return fmt.Errorf("checking cache path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("cache path %s escapes %s", target, base)
	}
	return nil
}

func installCodexPluginConfig(path, pluginKey string) error {
	readPath, writePath, err := resolveSymlinkWritePath(path)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", path, err)
	}

	data, err := os.ReadFile(readPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", readPath, err)
	}
	perm := os.FileMode(0o600)
	if info, statErr := os.Stat(readPath); statErr == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat %s: %w", readPath, statErr)
	}
	next := upsertCodexPluginConfig(string(data), pluginKey)
	if string(data) == next {
		fmt.Printf("  Skipped:   %s (Codex plugin already enabled)\n", path)
		return nil
	}
	if err := atomicWriteFile(writePath, []byte(next), perm); err != nil {
		return fmt.Errorf("writing %s: %w", writePath, err)
	}
	fmt.Printf("  Installed: %s\n", path)
	return nil
}

func resolveSymlinkWritePath(path string) (readPath, writePath string, err error) {
	root := path
	if abs, absErr := filepath.Abs(path); absErr == nil {
		root = abs
	}
	current := root
	seen := map[string]bool{}
	for {
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return current, current, nil
			}
			return "", "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, current, nil
		}
		if seen[current] {
			return "", "", fmt.Errorf("symlink cycle at %s", current)
		}
		seen[current] = true
		target, err := os.Readlink(current)
		if err != nil {
			return "", "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
}

func upsertCodexPluginConfig(raw, pluginKey string) string {
	raw = upsertTomlBool(raw, "features", "plugins", true)
	raw = upsertTomlBool(raw, "features", "hooks", true)
	raw = upsertTomlBool(raw, "features", "plugin_hooks", true)
	return upsertTomlBool(raw, fmt.Sprintf("plugins.%q", pluginKey), "enabled", true)
}

//nolint:unparam // value is always true today, but the function is designed for general TOML bool upsert
func upsertTomlBool(raw, table, key string, value bool) string {
	header := fmt.Sprintf("[%s]", table)
	replacement := fmt.Sprintf("%s = %t", key, value)
	if raw != "" && !strings.HasSuffix(raw, "\n") {
		raw += "\n"
	}

	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !tomlTableMatches(line, table) {
			continue
		}

		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if _, ok := tomlTableName(lines[j]); ok {
				end = j
				break
			}
		}
		for j := i + 1; j < end; j++ {
			if tomlKeyIs(lines[j], key) {
				lines[j] = leadingWhitespace(lines[j]) + replacement
				return ensureTrailingNewline(strings.Join(lines, "\n"))
			}
		}

		insert := end
		for insert > i+1 && strings.TrimSpace(lines[insert-1]) == "" {
			insert--
		}
		next := make([]string, 0, len(lines)+1)
		next = append(next, lines[:insert]...)
		next = append(next, replacement)
		next = append(next, lines[insert:]...)
		return ensureTrailingNewline(strings.Join(next, "\n"))
	}

	if strings.TrimSpace(raw) == "" {
		return header + "\n" + replacement + "\n"
	}
	return raw + "\n" + header + "\n" + replacement + "\n"
}

func tomlTableMatches(line, table string) bool {
	name, array, ok := tomlTableHeader(line)
	return ok && !array && name == table
}

func tomlTableName(line string) (string, bool) {
	name, _, ok := tomlTableHeader(line)
	return name, ok
}

func tomlTableHeader(line string) (name string, array bool, ok bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "[[") {
		end := strings.Index(trimmed, "]]")
		if end <= 2 {
			return "", false, false
		}
		rest := strings.TrimSpace(trimmed[end+2:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", false, false
		}
		return strings.TrimSpace(trimmed[2:end]), true, true
	}
	if !strings.HasPrefix(trimmed, "[") {
		return "", false, false
	}
	end := strings.IndexByte(trimmed, ']')
	if end <= 1 {
		return "", false, false
	}
	rest := strings.TrimSpace(trimmed[end+1:])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return "", false, false
	}
	return strings.TrimSpace(trimmed[1:end]), false, true
}

func tomlKeyIs(line, key string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	return trimmed == key || strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=")
}

func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// printUniqueHints prints each hint once, in the order it first appeared.
func printUniqueHints(hints []string) {
	seen := make(map[string]bool)
	for _, hint := range hints {
		if seen[hint] {
			continue
		}
		seen[hint] = true
		fmt.Printf("  %s\n", hint)
	}
}
