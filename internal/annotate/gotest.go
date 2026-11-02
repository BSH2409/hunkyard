package annotate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

var testFileLine = regexp.MustCompile(`([\w./-]+_test\.go):(\d+):`)

// parseGoTest reads `go test -json`. Each failing test becomes a finding on
// the test file line its output names; a failure with no file reference is
// counted as ignored.
func parseGoTest(r io.Reader, opts Options) (Annotations, error) {
	out := Annotations{Tool: "go test", Findings: []Finding{}}
	output := map[string][]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e testEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return Annotations{}, fmt.Errorf("invalid go test -json line: %w", err)
		}
		if e.Test == "" {
			continue
		}
		// Tests are identified by package and name: two packages can both
		// have a TestParse, and their output must not mix.
		key := e.Package + "\x00" + e.Test
		switch e.Action {
		case "output":
			if text := strings.TrimSpace(e.Output); text != "" && !strings.HasPrefix(text, "=== ") && !strings.HasPrefix(text, "--- ") {
				output[key] = append(output[key], text)
			}
		case "pass", "skip":
			delete(output, key)
		case "fail":
			f, ok := failureFinding(e.Test, packageDir(e.Package, opts), output[key], opts)
			delete(output, key)
			if !ok {
				out.IgnoredFiles++
				continue
			}
			out.Findings = append(out.Findings, f)
		}
	}
	return out, scanner.Err()
}

// packageDir maps an import path inside the module to its directory.
func packageDir(pkg string, opts Options) string {
	if opts.ModulePath == "" || pkg == opts.ModulePath {
		return ""
	}
	if rest, ok := strings.CutPrefix(pkg, strings.TrimSuffix(opts.ModulePath, "/")+"/"); ok {
		return rest
	}
	return ""
}

func failureFinding(test, dir string, lines []string, opts Options) (Finding, bool) {
	for _, text := range lines {
		m := testFileLine.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		const maxLines = 20
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		message := test + " failed:\n" + strings.Join(lines, "\n")
		file := m[1]
		if dir != "" && !strings.Contains(file, "/") {
			file = dir + "/" + file
		}
		return Finding{Path: relativePath(file, opts), Line: n, EndLine: n, Message: message, Rule: test, Level: "error"}, true
	}
	return Finding{}, false
}
