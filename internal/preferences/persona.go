package preferences

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
)

const personaHeader = generatedHeader + "\n<!-- Defaults only: current user instructions and repository standards win. -->\n"

func estimatedTokens(text string) int { return (len(text) + 3) / 4 }
func isCoreRule(r rule) bool {
	return r["status"] == "core" || r["origin"] == "source_pack" && r["load_tier"] == "core"
}
func directiveKey(r rule) string  { return strings.ToLower(normalize(fmt.Sprint(r["directive"]))) }
func personaBullet(r rule) string { return "- " + strings.TrimSpace(fmt.Sprint(r["directive"])) + "\n" }
func personaMarkdown(rules []rule) string {
	var text strings.Builder
	text.WriteString(personaHeader)
	for _, r := range rules {
		text.WriteString(personaBullet(r))
	}
	return text.String()
}

func rendererPrecedence(r rule) []string {
	scope, _ := r["scope"].(map[string]any)
	kind := "1"
	if scope["kind"] == "repository" {
		kind = "0"
	}
	origin := "0"
	if r["origin"] == "source_pack" {
		origin = "1"
	}
	tier := "1"
	if isCoreRule(r) {
		tier = "0"
	}
	return []string{kind, origin, tier, fmt.Sprint(r["directive"]), fmt.Sprint(r["id"])}
}

//nolint:gocyclo // One rendering pass applies scope, provenance, duplication, and budget rules.
func renderPersona(rules []rule) (map[string]string, map[string]any) {
	active := []rule{}
	for _, r := range rules {
		if r["status"] == "active" || r["status"] == "core" {
			active = append(active, r)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		return slices.Compare(rendererPrecedence(active[i]), rendererPrecedence(active[j])) < 0
	})
	byPath := map[string][]rule{"core.md": {}}
	for _, r := range active {
		scope, _ := r["scope"].(map[string]any)
		prefix := ""
		if scope["kind"] == "repository" {
			prefix = "repositories/" + fmt.Sprint(scope["repo_key"]) + "/"
			if _, ok := byPath[prefix+"core.md"]; !ok {
				byPath[prefix+"core.md"] = []rule{}
			}
		}
		if isCoreRule(r) {
			byPath[prefix+"core.md"] = append(byPath[prefix+"core.md"], r)
			continue
		}
		values, _ := stringArray(r["surfaces"])
		for _, surface := range sortedUnique(values) {
			if slices.Contains(surfaces, surface) {
				name := prefix + surface + ".md"
				byPath[name] = append(byPath[name], r)
			}
		}
	}
	files := map[string]string{}
	rulePaths := map[string][]string{}
	for name, pathRules := range byPath {
		personal := map[string]bool{}
		for _, r := range pathRules {
			if r["origin"] != "source_pack" {
				personal[directiveKey(r)] = true
			}
		}
		selected := []rule{}
		for _, r := range pathRules {
			if r["origin"] == "source_pack" && personal[directiveKey(r)] {
				continue
			}
			if path.Base(name) == "core.md" && estimatedTokens(personaMarkdown(append(append([]rule{}, selected...), r))) > 600 {
				continue
			}
			selected = append(selected, r)
		}
		files[name] = personaMarkdown(selected)
		for _, r := range selected {
			id := r["id"].(string)
			rulePaths[id] = append(rulePaths[id], name)
		}
	}
	manifestRules := map[string]any{}
	for _, r := range active {
		id := r["id"].(string)
		paths := rulePaths[id]
		if len(paths) == 0 {
			continue
		}
		sort.Strings(paths)
		values, _ := stringArray(r["surfaces"])
		manifestRules[id] = map[string]any{"estimated_tokens": estimatedTokens(personaBullet(r)), "origin": r["origin"], "paths": paths, "scope": r["scope"], "surfaces": sortedUnique(values)}
	}
	return files, map[string]any{"rules": manifestRules}
}
