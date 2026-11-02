package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type packExport struct {
	name, version, author, scope, repoKey, out string
}

// personalRulesBySurface groups the user's own active rules of one scope by
// their first surface. Only the directive text leaves the store.
func (s *Store) personalRulesBySurface(scope, repoKey string) (map[string][]string, int) {
	grouped := map[string][]string{}
	skipped := 0
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return grouped, 0
	}
	for _, r := range current.Rules.rules {
		sc := record(r["scope"])
		if importedDefault(r) || (r["status"] != "active" && r["status"] != "core") || sc["kind"] != scope || scope == "repository" && textField(sc["repo_key"]) != repoKey {
			continue
		}
		directive := normalize(textField(r["directive"]))
		if privacy := sanitize(directive); !privacy.Accepted || privacy.Excerpt != directive || len([]rune(directive)) > 240 {
			skipped++
			continue
		}
		surfaceList := stringValues(r["surfaces"])
		sort.Strings(surfaceList)
		grouped[surfaceList[0]] = append(grouped[surfaceList[0]], directive)
	}
	for surface := range grouped {
		sort.Strings(grouped[surface])
	}
	return grouped, skipped
}

func surfaceTitle(surface string) string {
	return strings.ToUpper(surface[:1]) + surface[1:]
}

// exportPack writes the user's rules of one scope as a team pack another
// developer can import.
func (s *Store) exportPack(e packExport) (map[string]any, error) {
	if !packNamePattern.MatchString("shared-"+e.name) || strings.TrimSpace(e.version) == "" {
		return nil, fmt.Errorf("pack name must be lowercase letters, digits and dashes, and a version is required")
	}
	if entries, err := os.ReadDir(e.out); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty", e.out)
	}
	grouped, skipped := s.personalRulesBySurface(e.scope, e.repoKey)
	if len(grouped) == 0 {
		return nil, fmt.Errorf("no %s rules to export", e.scope)
	}
	if err := os.MkdirAll(filepath.Join(e.out, "reference"), 0o755); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(grouped))
	for surface := range grouped {
		names = append(names, surface)
	}
	sort.Strings(names)
	var refs strings.Builder
	count := 0
	for _, surface := range names {
		var body strings.Builder
		body.WriteString("# " + surfaceTitle(surface) + "\n\n")
		for _, d := range grouped[surface] {
			body.WriteString("- " + d + "\n")
			count++
		}
		if err := os.WriteFile(filepath.Join(e.out, "reference", surface+".md"), []byte(body.String()), 0o644); err != nil {
			return nil, err
		}
		refs.WriteString("- [" + surfaceTitle(surface) + "](reference/" + surface + ".md)\n")
	}
	skill := "---\nname: " + e.name + "\ndescription: Team coding conventions exported from hunkyard preferences.\n---\n\n# " + e.name + "\n\n" +
		"Import with `hunkyard preferences pack import FOLDER --json`. One rule per bullet; personal preferences win over these.\n\n## References\n\n" + refs.String()
	if err := os.WriteFile(filepath.Join(e.out, "SKILL.md"), []byte(skill), 0o644); err != nil {
		return nil, err
	}
	content, err := readPackDir(e.out)
	if err != nil {
		return nil, err
	}
	manifest, _ := json.MarshalIndent(packManifest{SchemaVersion: 1, Name: e.name, Version: e.version, Author: e.author, CreatedAt: s.config.Clock().UTC().Format(time.RFC3339), Digest: content.digest}, "", "  ")
	if err := os.WriteFile(filepath.Join(e.out, "pack.json"), append(manifest, '\n'), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"status": "success", "out": e.out, "rules": count, "skipped": skipped, "digest": content.digest}, nil
}

// repositoryMarkdown renders this repository's own rules as one markdown
// document a developer can commit (for example as the seed of a pack).
func (s *Store) repositoryMarkdown(repoKey string) string {
	grouped, _ := s.personalRulesBySurface("repository", repoKey)
	var out strings.Builder
	out.WriteString("# Repository preferences\n\n")
	if len(grouped) == 0 {
		out.WriteString("No repository preferences yet.\n")
		return out.String()
	}
	names := make([]string, 0, len(grouped))
	for surface := range grouped {
		names = append(names, surface)
	}
	slices.Sort(names)
	for _, surface := range names {
		out.WriteString("## " + surfaceTitle(surface) + "\n\n")
		for _, d := range grouped[surface] {
			out.WriteString("- " + d + "\n")
		}
		out.WriteString("\n")
	}
	return out.String()
}
