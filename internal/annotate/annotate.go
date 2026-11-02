// Package annotate turns the output of local developer tools (coverage
// profiles, SARIF reports, go test -json streams) into review findings and
// uncovered-line marks. It reads files only; it never runs a tool.
package annotate

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one tool message on a file. Line 0 means the whole file.
type Finding struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
	Level   string `json:"level,omitempty"`
}

// Mark is a line range a tool flags without a message, such as code no test
// covered.
type Mark struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
}

// Annotations is everything one tool run says about the review.
type Annotations struct {
	Tool         string            `json:"tool"`
	Findings     []Finding         `json:"findings"`
	Marks        map[string][]Mark `json:"marks"`
	IgnoredFiles int               `json:"ignored_files"`
}

// Options tells parsers how to make tool paths repository-relative.
type Options struct {
	// ModulePath is the Go module path stripped from coverprofile paths.
	ModulePath string
	// Root is the repository root stripped from absolute or file:// paths.
	Root string
}

// Formats lists the accepted --format values.
var Formats = []string{"coverprofile", "lcov", "sarif", "go-test-json"}

const markUncovered = "uncovered"

// Parse reads one tool output. An empty tool name takes the format's default
// (or the SARIF driver name).
func Parse(format string, r io.Reader, tool string, opts Options) (Annotations, error) {
	var (
		out Annotations
		err error
	)
	switch format {
	case "coverprofile":
		out, err = parseCoverprofile(r, opts)
	case "lcov":
		out, err = parseLcov(r, opts)
	case "sarif":
		out, err = parseSarif(r, opts)
	case "go-test-json":
		out, err = parseGoTest(r, opts)
	default:
		return Annotations{}, fmt.Errorf("unknown format %q (want one of %s)", format, strings.Join(Formats, ", "))
	}
	if err != nil {
		return Annotations{}, err
	}
	if tool != "" {
		out.Tool = tool
	}
	if out.Findings == nil {
		out.Findings = []Finding{}
	}
	if out.Marks == nil {
		out.Marks = map[string][]Mark{}
	}
	for p, marks := range out.Marks {
		out.Marks[p] = mergeMarks(marks)
	}
	return out, nil
}

// relativePath makes a tool-reported path relative to the repository.
func relativePath(p string, opts Options) string {
	p = strings.TrimPrefix(p, "file://")
	p = filepath.ToSlash(p)
	if opts.Root != "" {
		root := strings.TrimSuffix(filepath.ToSlash(opts.Root), "/") + "/"
		p = strings.TrimPrefix(p, root)
	}
	return strings.TrimPrefix(path.Clean(p), "./")
}

// mergeMarks sorts marks and joins ranges that touch or overlap.
func mergeMarks(marks []Mark) []Mark {
	sort.Slice(marks, func(i, j int) bool { return marks[i].Start < marks[j].Start })
	merged := []Mark{}
	for _, m := range marks {
		if n := len(merged); n > 0 && merged[n-1].Kind == m.Kind && m.Start <= merged[n-1].End+1 {
			if m.End > merged[n-1].End {
				merged[n-1].End = m.End
			}
			continue
		}
		merged = append(merged, m)
	}
	return merged
}
