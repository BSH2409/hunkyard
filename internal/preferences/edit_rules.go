package preferences

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "apply_patch": true}

// pathSurfaces maps edited files to the rule surfaces that apply to them, so
// rules load when the agent edits a matching file, not only when a prompt
// happens to mention the topic.
var pathSurfaces = []struct {
	pattern  string
	surfaces []string
}{
	{"*.rb", []string{"ruby"}},
	{"app/**", []string{"rails"}}, {"config/**", []string{"rails"}}, {"db/**", []string{"rails"}}, {"*.rake", []string{"rails"}},
	{"db/migrate/**", []string{"database"}}, {"*.sql", []string{"database"}},
	{"*.js", []string{"frontend"}}, {"*.ts", []string{"frontend"}}, {"*.tsx", []string{"frontend"}}, {"*.jsx", []string{"frontend"}}, {"*.css", []string{"frontend"}}, {"*.html", []string{"frontend"}},
	{"*_spec.rb", []string{"testing"}}, {"*_test.go", []string{"testing"}}, {"spec/**", []string{"testing"}}, {"test/**", []string{"testing"}},
	{"app/jobs/**", []string{"jobs"}}, {"*_job.rb", []string{"jobs"}},
	{"app/services/**", []string{"services"}},
	{"app/controllers/**", []string{"api"}}, {"*_controller.rb", []string{"api"}},
	{"*.md", []string{"communication"}},
	{"Dockerfile", []string{"devops"}}, {"*.yml", []string{"devops"}}, {"*.yaml", []string{"devops"}}, {".github/**", []string{"devops"}},
	{"wp-content/**", []string{"wordpress"}}, {"*.php", []string{"wordpress"}},
}

const editRuleLimit = 10

// editedPaths returns the files an edit tool call will change: Claude's
// file_path, or the files named in a Codex apply_patch patch.
func editedPaths(toolName string, input map[string]any) []string {
	if !editTools[toolName] {
		return nil
	}
	if name := textField(input["file_path"]); name != "" {
		return []string{name}
	}
	var texts []string
	for _, value := range input {
		switch v := value.(type) {
		case string:
			texts = append(texts, v)
		case []any:
			texts = append(texts, stringValues(v)...)
		}
	}
	var paths []string
	for _, text := range texts {
		for _, line := range strings.Split(text, "\n") {
			for _, prefix := range []string{"*** Update File: ", "*** Add File: "} {
				if name, ok := strings.CutPrefix(line, prefix); ok && strings.TrimSpace(name) != "" {
					paths = append(paths, strings.TrimSpace(name))
				}
			}
		}
	}
	return paths
}

// matchPathSuffix matches a repository-relative glob against any trailing
// part of a path, so absolute editor paths match patterns like "app/**".
func matchPathSuffix(pattern, name string) bool {
	parts := strings.Split(path.Clean(strings.ReplaceAll(name, "\\", "/")), "/")
	for i := range parts {
		if matchGuardPattern(pattern, strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

// pathSurfacesFor maps edited files to surfaces. Paths inside cwd match on
// their repository-relative form, so parent folders named test/ or app/ do
// not pick surfaces; other paths fall back to suffix matching.
func pathSurfacesFor(paths []string, cwd string) []string {
	var found []string
	for _, name := range paths {
		match := matchPathSuffix
		if rel, err := filepath.Rel(cwd, name); cwd != "" && filepath.IsAbs(name) && err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			name = filepath.ToSlash(rel)
			match = matchGuardPattern
		}
		for _, mapping := range pathSurfaces {
			if match(mapping.pattern, name) {
				found = append(found, mapping.surfaces...)
			}
		}
	}
	return sortedUnique(found)
}

// editRulesResponse adds rules for the edited files that this session has
// not seen yet. It never denies an edit.
//
//nolint:gocyclo // Scope, status, session and limit filters apply together to each rule.
func (s *Store) editRulesResponse(payload map[string]any) map[string]any {
	paths := editedPaths(textField(payload["tool_name"]), record(payload["tool_input"]))
	surfaces := pathSurfacesFor(paths, textField(payload["cwd"]))
	if len(surfaces) == 0 {
		return nil
	}
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return nil
	}
	session := textField(payload["session_id"])
	repo, _ := repositoryKey(textField(payload["cwd"]))
	seen := map[string]bool{}
	for _, id := range s.loadSession(session).InjectedRuleIDs {
		seen[id] = true
	}
	var selected []rule
	var ids []string
	reasons := map[string]string{}
	for _, r := range current.Rules.rules {
		scope := record(r["scope"])
		inScope := scope["kind"] == "global" || scope["kind"] == "repository" && scope["repo_key"] == repo
		active := r["status"] == "active" || r["status"] == "core"
		id := textField(r["id"])
		if !inScope || !active || seen[id] || overlapCount(stringValues(r["surfaces"]), surfaces) == 0 || len(selected) >= editRuleLimit {
			continue
		}
		selected = append(selected, r)
		ids = append(ids, id)
		reasons[id] = "editing " + editedPathFor(r, paths, textField(payload["cwd"]))
	}
	if len(selected) == 0 {
		return nil
	}
	_ = s.updateSession(session, func(state *sessionState) {
		state.InjectedRuleIDs = append(state.InjectedRuleIDs, ids...)
		state.noteLoads(reasons)
	})
	var text strings.Builder
	fmt.Fprintf(&text, "Preferences for %s:\n", strings.Join(slices.Compact(paths), ", "))
	for _, r := range selected {
		text.WriteString(personaBullet(r))
	}
	return map[string]any{"hookEventName": "PreToolUse", "additionalContext": text.String()}
}

// editedPathFor names the first edited path whose surfaces the rule covers.
func editedPathFor(r rule, paths []string, cwd string) string {
	for _, path := range paths {
		if overlapCount(stringValues(r["surfaces"]), pathSurfacesFor([]string{path}, cwd)) > 0 {
			return path
		}
	}
	return paths[0]
}
