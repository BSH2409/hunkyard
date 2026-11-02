package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

// A team pack is a folder a team shares (committed in a repository, or passed
// around as files): SKILL.md lists reference/<surface>.md files under
// "## References", and each "- directive" bullet in them becomes one rule of
// that surface. pack.json names the pack and pins its content digest. Packs
// are text: importing one only adds rules; it never changes guards, hooks or
// configuration, and personal rules keep winning over pack rules.

const (
	packFileLimit  = 1 << 20
	packFilesLimit = 64
	// RepositoryPackDir is where a repository keeps its team pack.
	RepositoryPackDir = ".hunkyard/preferences-pack"
)

type packManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Author        string `json:"author,omitempty"`
	CreatedAt     string `json:"created_at"`
	Digest        string `json:"digest"`
}

type packDirective struct{ surface, directive string }

type packContent struct {
	digest     string
	directives []packDirective
	invalid    int
}

// packTarget says where a pack's rules apply.
type packTarget struct {
	scope      string // global | repository
	repoKey    string
	repository bool // the repository's own pack (named repo-<key>, pack.json optional)
}

// packResult reports one import.
type packResult struct {
	Status          string `json:"status"` // imported | updated | unchanged | dry_run
	Pack            string `json:"pack"`
	Version         string `json:"version,omitempty"`
	Added           int    `json:"added"`
	Kept            int    `json:"kept"`
	Removed         int    `json:"removed"`
	SkippedPersonal int    `json:"skipped_personal"`
	SkippedInvalid  int    `json:"skipped_invalid"`
	GenerationID    string `json:"generation_id,omitempty"`
}

// packEntry is one imported pack in packs.json.
type packEntry struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // shared | repository
	Scope      string `json:"scope"`
	RepoKey    string `json:"repo_key,omitempty"`
	Version    string `json:"version,omitempty"`
	Author     string `json:"author,omitempty"`
	Digest     string `json:"digest"`
	ImportedAt string `json:"imported_at"`
	Rules      int    `json:"rules"`
	// Declined marks a repository pack the user removed: session start
	// leaves it out until the pack's content (its digest) changes.
	Declined bool `json:"declined,omitempty"`
}

type packRegistry struct {
	SchemaVersion int                  `json:"schema_version"`
	Packs         map[string]packEntry `json:"packs"`
}

// readPackDir reads a pack folder with the source-pack rules: SKILL.md plus
// only the reference/*.md files its References section links, each a
// regular file inside the folder.
//
//nolint:gocyclo // Every file is confined, size-checked and allowlisted before it is read.
func readPackDir(dir string) (packContent, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return packContent{}, err
	}
	read := func(name string) (string, error) {
		path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("pack file escapes the pack folder: %s", name)
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Size() > packFileLimit {
			return "", fmt.Errorf("pack file must be a regular file of at most 1 MiB: %s", name)
		}
		data, err := os.ReadFile(path)
		return string(data), err
	}
	main, err := read("SKILL.md")
	if err != nil {
		return packContent{}, err
	}
	refs := []string{}
	inReferences := false
	for _, line := range strings.Split(main, "\n") {
		if referencesHeading.MatchString(line) {
			inReferences = true
			continue
		}
		if sectionHeading.MatchString(line) {
			inReferences = false
		}
		if !inReferences {
			continue
		}
		for _, link := range referenceLink.FindAllStringSubmatch(line, -1) {
			if !referencePath.MatchString(link[1]) {
				return packContent{}, fmt.Errorf("pack reference is not allowlisted: %q", link[1])
			}
			if !slices.Contains(refs, link[1]) {
				refs = append(refs, link[1])
			}
		}
	}
	if len(refs) > packFilesLimit {
		return packContent{}, fmt.Errorf("a pack lists at most %d reference files", packFilesLimit)
	}
	sort.Strings(refs)
	var combined strings.Builder
	combined.WriteString("SKILL.md\x00" + digest(main))
	content := packContent{}
	for _, name := range refs {
		text, err := read(name)
		if err != nil {
			return packContent{}, err
		}
		combined.WriteString(name + "\x00" + digest(text))
		surface := strings.TrimSuffix(strings.TrimPrefix(name, "reference/"), ".md")
		for _, line := range strings.Split(text, "\n") {
			bullet, ok := strings.CutPrefix(strings.TrimSpace(line), "- ")
			if !ok {
				continue
			}
			directive := normalize(bullet)
			privacy := sanitize(directive)
			if !slices.Contains(surfaces, surface) || directive == "" || len([]rune(directive)) > 240 || !privacy.Accepted || privacy.Excerpt != directive {
				content.invalid++
				continue
			}
			content.directives = append(content.directives, packDirective{surface: surface, directive: directive})
		}
	}
	content.digest = digest(combined.String())
	return content, nil
}

// repositoryPackDir returns the team pack folder of the repository at cwd.
func repositoryPackDir(cwd string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	top := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if strings.TrimSpace(top) == "" {
		return "", fmt.Errorf("not inside a git repository")
	}
	return filepath.Join(strings.TrimSpace(top), filepath.FromSlash(RepositoryPackDir)), nil
}

func readPackManifest(dir string) (packManifest, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "pack.json"))
	if errors.Is(err, os.ErrNotExist) {
		return packManifest{}, false, nil
	}
	if err != nil {
		return packManifest{}, false, err
	}
	var m packManifest
	if err := strictjson.Decode(data, &m); err != nil || m.SchemaVersion != 1 {
		return packManifest{}, true, fmt.Errorf("invalid pack.json")
	}
	if !packNamePattern.MatchString("shared-"+m.Name) || strings.TrimSpace(m.Version) == "" || len(m.Version) > 64 || len(m.Author) > 128 {
		return packManifest{}, true, fmt.Errorf("pack.json needs a lowercase name and a version")
	}
	return m, true, nil
}

func (s *Store) loadPackRegistry() packRegistry {
	registry := packRegistry{SchemaVersion: 1, Packs: map[string]packEntry{}}
	data, err := os.ReadFile(s.dataPath("packs.json"))
	if err == nil {
		_ = json.Unmarshal(data, &registry)
	}
	if registry.Packs == nil {
		registry.Packs = map[string]packEntry{}
	}
	return registry
}

func (s *Store) savePackRegistry(registry packRegistry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	if err := ensureDirectory(s.dataPath()); err != nil {
		return err
	}
	return atomicReplace(s.dataPath("packs.json"), append(data, '\n'))
}

// importPack makes the rules of pack dir match its content: new directives
// become rules, dropped ones are removed, and a directive a personal or
// source rule already holds is left to that rule.
//
//nolint:gocyclo // One locked pass computes, applies and records the pack's rule set.
func (s *Store) importPack(dir string, target packTarget, dryRun bool) (packResult, error) {
	content, err := readPackDir(dir)
	if err != nil {
		return packResult{}, err
	}
	manifest, hasManifest, err := readPackManifest(dir)
	if err != nil {
		return packResult{}, err
	}
	result := packResult{SkippedInvalid: content.invalid}
	entry := packEntry{Kind: "shared", Scope: target.scope, RepoKey: target.repoKey, Digest: content.digest}
	switch {
	case target.repository:
		if !repositoryKeyPattern.MatchString(target.repoKey) {
			return result, fmt.Errorf("a repository pack needs the repository's key")
		}
		entry.Name, entry.Kind, entry.Scope = "repo-"+target.repoKey, "repository", "repository"
	case !hasManifest:
		return result, fmt.Errorf("a shared pack needs pack.json")
	default:
		entry.Name = "shared-" + manifest.Name
	}
	if hasManifest {
		if manifest.Digest != content.digest {
			return result, fmt.Errorf("pack content does not match the digest in pack.json")
		}
		entry.Version, entry.Author = manifest.Version, manifest.Author
	}
	if entry.Scope != "global" && entry.Scope != "repository" || entry.Scope == "repository" && !repositoryKeyPattern.MatchString(target.repoKey) {
		return result, fmt.Errorf("pack scope must be global, or repository with a repository key")
	}
	scope := map[string]any{"kind": entry.Scope, "repo_key": nil}
	if entry.Scope == "repository" {
		scope["repo_key"] = target.repoKey
	} else {
		entry.RepoKey = ""
	}
	result.Pack, result.Version = entry.Name, entry.Version
	err = s.withGenerationLock(func() error {
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		registry := s.loadPackRegistry()
		catalog, err := s.catalogFromGeneration(current)
		if err != nil {
			return err
		}
		existing := map[string]rule{}
		for _, r := range catalog.rules {
			existing[textField(r["id"])] = r
		}
		now := s.config.Clock()
		wanted := map[string]bool{}
		var next []rule
		for _, d := range content.directives {
			id := ruleID(d.directive, scope, []string{d.surface})
			if wanted[id] {
				continue
			}
			if have, ok := existing[id]; ok {
				if have["pack"] == entry.Name {
					wanted[id] = true
					result.Kept++
				} else {
					result.SkippedPersonal++
				}
				continue
			}
			wanted[id] = true
			r := newRule(d.directive, scope, []string{d.surface}, "team_pack", now)
			r["pack"] = entry.Name
			if err := recomputeRule(r, now); err != nil {
				return err
			}
			next = append(next, r)
			result.Added++
		}
		removed := map[string]bool{}
		for id, r := range existing {
			if r["pack"] == entry.Name && !wanted[id] {
				removed[id] = true
			}
		}
		result.Removed = len(removed)
		previous, known := registry.Packs[entry.Name]
		if result.Added == 0 && result.Removed == 0 && known && previous.Digest == entry.Digest {
			result.Status = "unchanged"
			return nil
		}
		result.Status = "imported"
		if known {
			result.Status = "updated"
		}
		if dryRun {
			result.Status = "dry_run"
			return nil
		}
		rules := withoutRules(catalog.rules, removed)
		rules = append(rules, next...)
		id, err := s.commitPackRules(current, rules, map[string]any{"pack": entry.Name, "action": "import"})
		if err != nil {
			return err
		}
		result.GenerationID = id
		entry.ImportedAt = now.UTC().Format(time.RFC3339)
		entry.Rules = result.Added + result.Kept
		registry.Packs[entry.Name] = entry
		return s.savePackRegistry(registry)
	})
	return result, err
}

// syncRepositoryPack keeps the repository's committed team pack applied: a
// new or changed pack is imported, and a pack whose folder was deleted is
// removed. Failures are advisory: the session starts with what it has.
func (s *Store) syncRepositoryPack(cwd, repoKey string) {
	if repoKey == "" {
		return
	}
	name := "repo-" + repoKey
	known, imported := s.loadPackRegistry().Packs[name]
	dir, err := repositoryPackDir(cwd)
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		if imported {
			s.forgetPack(name)
		}
		return
	}
	content, err := readPackDir(dir)
	if err != nil || imported && known.Digest == content.digest {
		return // unchanged, or declined at this digest
	}
	_, _ = s.importPack(dir, packTarget{scope: "repository", repoKey: repoKey, repository: true}, false)
}

// removePack deletes a team pack's rules and its registry entry.
func (s *Store) removePack(name string) (packResult, error) {
	result := packResult{Pack: name, Status: "removed"}
	if !packNamePattern.MatchString(name) {
		return result, fmt.Errorf("only team packs (shared-… or repo-…) can be removed")
	}
	err := s.withGenerationLock(func() error {
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		registry := s.loadPackRegistry()
		removed := map[string]bool{}
		var rules []rule
		if current != nil {
			for _, r := range current.Rules.rules {
				if r["pack"] == name {
					removed[textField(r["id"])] = true
				}
			}
			rules = withoutRules(current.Rules.rules, removed)
		}
		entry, known := registry.Packs[name]
		if (!known || entry.Declined) && len(removed) == 0 {
			return fmt.Errorf("no team pack named %s", name)
		}
		result.Removed = len(removed)
		if len(removed) > 0 {
			id, err := s.commitPackRules(current, rules, map[string]any{"pack": name, "action": "remove"})
			if err != nil {
				return err
			}
			result.GenerationID = id
		}
		if entry.Kind == "repository" && known {
			// The repository still ships the pack: remember the decline at
			// this digest so session start does not bring it back.
			entry.Declined, entry.Rules = true, 0
			registry.Packs[name] = entry
		} else {
			delete(registry.Packs, name)
		}
		return s.savePackRegistry(registry)
	})
	return result, err
}

// forgetPack drops a pack's registry entry and rules (its folder is gone).
func (s *Store) forgetPack(name string) {
	_, _ = s.removePack(name)
	_ = s.withGenerationLock(func() error {
		registry := s.loadPackRegistry()
		delete(registry.Packs, name)
		return s.savePackRegistry(registry)
	})
}

// listPacks returns imported team packs with their current rule counts.
func (s *Store) listPacks() []packEntry {
	registry := s.loadPackRegistry()
	counts := map[string]int{}
	if current, err := s.loadCurrent(); err == nil && current != nil {
		for _, r := range current.Rules.rules {
			if name := textField(r["pack"]); name != "" && r["status"] == "active" {
				counts[name]++
			}
		}
	}
	out := []packEntry{}
	for name, e := range registry.Packs {
		e.Name, e.Rules = name, counts[name]
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// withoutRules drops the removed rules and every reference other rules hold
// to them, so the rule graph stays valid.
func withoutRules(rules []rule, removed map[string]bool) []rule {
	kept := make([]rule, 0, len(rules))
	for _, r := range rules {
		if removed[textField(r["id"])] {
			continue
		}
		r = rule(cloneMap(r))
		for _, field := range []string{"conflicts_with", "supersedes"} {
			if refs := stringValues(r[field]); len(refs) > 0 {
				left := slices.DeleteFunc(slices.Clone(refs), func(id string) bool { return removed[id] })
				if len(left) == 0 {
					delete(r, field)
				} else {
					r[field] = left
				}
			}
		}
		if removed[textField(r["superseded_by"])] {
			delete(r, "superseded_by")
		}
		kept = append(kept, r)
	}
	return kept
}

// commitPackRules publishes rules as a new generation with fresh personas.
// Caller holds the generation lock.
func (s *Store) commitPackRules(current *generation, rules []rule, note map[string]any) (string, error) {
	normalized, err := normalizeRules(rules, s.config.Clock())
	if err != nil {
		return "", err
	}
	catalog := &catalog{store: s, rules: normalized}
	files, manifest := renderPersona(catalog.orderedActive())
	next := generation{Rules: catalog.ruleSet(), ProcessedIDs: []string{}, SourcePacks: map[string]any{}, PersonaFiles: files, Manifest: baseManifest(current)}
	if current != nil {
		next.ProcessedIDs, next.SourcePacks = current.ProcessedIDs, current.SourcePacks
	}
	for key, value := range manifest {
		next.Manifest[key] = value
	}
	next.Manifest["team_pack"] = note
	return s.commitLocked(next)
}
