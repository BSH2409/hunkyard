package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	prefixInline = "inline:"
	prefixFile   = "file:"
)

// LoadTemplate resolves a config value (inline: or file:) to template text.
// baseDir is the directory for relative file: paths (project root or home).
func LoadTemplate(value, baseDir string) (string, error) {
	switch {
	case strings.HasPrefix(value, prefixInline):
		return strings.TrimPrefix(value, prefixInline), nil
	case strings.HasPrefix(value, prefixFile):
		path := strings.TrimPrefix(value, prefixFile)
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("prompt value must start with %q or %q", prefixInline, prefixFile)
	}
}

// LoadProjectTemplate is LoadTemplate for a project config layer: a file:
// value must name a file inside projectDir, after symlinks are resolved, so
// an untrusted repository cannot read files elsewhere on the machine.
func LoadProjectTemplate(value, projectDir string) (string, error) {
	if !strings.HasPrefix(value, prefixFile) {
		return LoadTemplate(value, projectDir)
	}
	path, err := projectFilePath(strings.TrimPrefix(value, prefixFile), projectDir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func projectFilePath(path, projectDir string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}
	root, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("project prompt file %q is outside the project", path)
	}
	return resolved, nil
}

// Render executes a Go text/template with the given context.
func Render(tmplText string, ctx Context) (string, error) {
	return RenderData(tmplText, ctx.TemplateData())
}

// RenderData executes a Go text/template against a raw snake_case data map
// (used by hooks that don't have a dedicated Context type, e.g. on_story_generate).
func RenderData(tmplText string, data map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// TemplateSource describes where a template value came from.
func TemplateSource(layer, value string) string {
	if strings.HasPrefix(value, prefixFile) {
		return layer + ":" + strings.TrimPrefix(value, prefixFile)
	}
	return layer + ":inline"
}
