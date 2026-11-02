// Package impact finds what a changed file touches: the symbols its diff
// defines or removes, the other files that mention them, and which of those
// are tests. Matching is textual (word boundaries), not a call graph.
package impact

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

const (
	maxSymbols  = 20
	maxMentions = 50
	maxTests    = 20
)

// Mention is a line in another file that mentions a changed symbol.
type Mention struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
	Text   string `json:"text"`
}

// TestFile is a test file that mentions changed symbols.
type TestFile struct {
	Path    string `json:"path"`
	Hits    int    `json:"hits"`
	Changed bool   `json:"changed"`
}

// Truncation says which lists were cut at their cap.
type Truncation struct {
	Mentions bool `json:"callers"`
	Tests    bool `json:"tests"`
}

// Result is the impact of one changed file.
type Result struct {
	Symbols   []string   `json:"symbols"`
	Mentions  []Mention  `json:"callers"`
	Tests     []TestFile `json:"tests"`
	Truncated Truncation `json:"truncated"`
}

// GrepFunc searches tracked files for whole-word mentions of words, leaving
// out excludePath.
type GrepFunc func(ctx context.Context, words []string, excludePath string) ([]vcs.GrepHit, error)

var definitions = map[string][]*regexp.Regexp{
	"go": {
		regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`),
		regexp.MustCompile(`^\s*type\s+([A-Za-z_]\w*)`),
		regexp.MustCompile(`^\s*(?:var|const)\s+([A-Za-z_]\w*)`),
	},
	"js": {
		regexp.MustCompile(`\bfunction\s*\*?\s*([A-Za-z_$][\w$]*)`),
		regexp.MustCompile(`\bclass\s+([A-Za-z_$][\w$]*)`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`),
		regexp.MustCompile(`^\s+(?:async\s+)?([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{`),
	},
	"ruby": {
		regexp.MustCompile(`^\s*def\s+(?:self\.)?([A-Za-z_]\w*[?!]?)`),
		regexp.MustCompile(`^\s*(?:class|module)\s+([A-Z]\w*)`),
	},
	"python": {
		regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)`),
		regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)`),
	},
}

var languages = map[string]string{
	".go": "go", ".js": "js", ".mjs": "js", ".cjs": "js", ".jsx": "js", ".ts": "js", ".tsx": "js",
	".rb": "ruby", ".py": "python",
}

// stopSymbols are names too common to search for usefully.
var stopSymbols = map[string]bool{"main": true, "init": true, "new": true, "test": true, "setup": true, "run": true, "get": true, "set": true, "if": true, "for": true, "while": true, "switch": true, "catch": true, "constructor": true, "render": true}

// Symbols lists the names the diff defines or removes (added and deleted
// definition lines), sorted, at most maxSymbols.
func Symbols(filePath string, hunks []diff.DiffHunk) []string {
	patterns := definitions[languages[strings.ToLower(path.Ext(filePath))]]
	if patterns == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Type != "add" && l.Type != "del" {
				continue
			}
			for _, p := range patterns {
				m := p.FindStringSubmatch(l.Content)
				if m == nil {
					continue
				}
				name := m[1]
				if len(strings.TrimRight(name, "?!")) >= 3 && !stopSymbols[strings.ToLower(name)] {
					seen[name] = true
				}
				break
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	if len(out) > maxSymbols {
		out = out[:maxSymbols]
	}
	return out
}

// IsTestPath reports whether a path is a test file by common conventions.
func IsTestPath(p string) bool {
	base := path.Base(p)
	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".spec.") || strings.Contains(base, ".test.") ||
		strings.HasSuffix(base, "_spec.rb") || strings.HasSuffix(base, "_test.py") || strings.HasPrefix(base, "test_") {
		return true
	}
	for _, dir := range strings.Split(path.Dir(p), "/") {
		switch dir {
		case "test", "tests", "spec", "__tests__":
			return true
		}
	}
	return false
}

// Search finds mentions of symbols outside filePath and splits them into
// callers and tests. changed marks files that are part of the review;
// ignored drops paths the review ignores.
func Search(ctx context.Context, grep GrepFunc, filePath string, symbols []string, changed map[string]bool, ignored func(string) bool) (Result, error) {
	result := Result{Symbols: append([]string{}, symbols...), Mentions: []Mention{}, Tests: []TestFile{}}
	if len(symbols) == 0 {
		return result, nil
	}
	hits, err := grep(ctx, symbols, filePath)
	if err != nil {
		return result, err
	}
	tests := map[string]*TestFile{}
	var testOrder []string
	for _, h := range hits {
		if h.Path == filePath || (ignored != nil && ignored(h.Path)) {
			continue
		}
		if IsTestPath(h.Path) {
			if tests[h.Path] == nil {
				tests[h.Path] = &TestFile{Path: h.Path, Changed: changed[h.Path]}
				testOrder = append(testOrder, h.Path)
			}
			tests[h.Path].Hits++
			continue
		}
		if len(result.Mentions) >= maxMentions {
			result.Truncated.Mentions = true
			continue
		}
		result.Mentions = append(result.Mentions, Mention{Path: h.Path, Line: h.Line, Symbol: matchedSymbol(h.Text, symbols), Text: truncate(h.Text, 200)})
	}
	sort.Strings(testOrder)
	for _, p := range testOrder {
		if len(result.Tests) >= maxTests {
			result.Truncated.Tests = true
			break
		}
		result.Tests = append(result.Tests, *tests[p])
	}
	return result, nil
}

func matchedSymbol(text string, symbols []string) string {
	for _, s := range symbols {
		if regexp.MustCompile(`(^|[^\w$])` + regexp.QuoteMeta(s) + `($|[^\w$])`).MatchString(text) {
			return s
		}
	}
	return symbols[0]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
