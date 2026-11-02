package annotate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseFixture(t *testing.T, format, name, tool string, opts Options) Annotations {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, err := Parse(format, f, tool, opts)
	if err != nil {
		t.Fatalf("Parse(%s): %v", format, err)
	}
	return got
}

func TestParse(t *testing.T) {
	cases := []struct {
		name, format, file, tool string
		opts                     Options
		wantTool                 string
		wantFindings             []Finding
		wantMarks                map[string][]Mark
		wantIgnored              int
	}{
		{name: "coverprofile", format: "coverprofile", file: "cover.out", opts: Options{ModulePath: "example.com/demo"}, wantTool: "coverage",
			wantFindings: []Finding{},
			wantMarks:    map[string][]Mark{"server.go": {{Start: 7, End: 9, Kind: "uncovered"}}, "util/util.go": {{Start: 1, End: 2, Kind: "uncovered"}}}},
		{name: "lcov", format: "lcov", file: "lcov.info", wantTool: "coverage",
			wantFindings: []Finding{},
			wantMarks:    map[string][]Mark{"web/app.js": {{Start: 2, End: 3, Kind: "uncovered"}}}},
		{name: "sarif", format: "sarif", file: "results.sarif", opts: Options{Root: "/repo"}, wantTool: "golangci-lint",
			wantFindings: []Finding{
				{Path: "server.go", Line: 7, EndLine: 8, Message: "x is unused", Rule: "unused", Level: "warning"},
				{Path: "util/util.go", Line: 0, EndLine: 0, Message: "missing header", Rule: "license", Level: "note"},
			},
			wantMarks: map[string][]Mark{}},
		{name: "go-test-json", format: "go-test-json", file: "gotest.json", wantTool: "go test",
			wantFindings: []Finding{{Path: "server_test.go", Line: 12, EndLine: 12, Message: "TestServe failed:\nserver_test.go:12: want 200, got 500", Rule: "TestServe", Level: "error"}},
			wantMarks:    map[string][]Mark{}, wantIgnored: 1},
		{name: "explicit tool name wins", format: "sarif", file: "results.sarif", tool: "lint", opts: Options{Root: "/repo"}, wantTool: "lint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFixture(t, tc.format, tc.file, tc.tool, tc.opts)
			if got.Tool != tc.wantTool {
				t.Fatalf("tool = %q, want %q", got.Tool, tc.wantTool)
			}
			if tc.wantFindings != nil && !reflect.DeepEqual(got.Findings, tc.wantFindings) {
				t.Fatalf("findings = %#v\nwant %#v", got.Findings, tc.wantFindings)
			}
			if tc.wantMarks != nil && !reflect.DeepEqual(got.Marks, tc.wantMarks) {
				t.Fatalf("marks = %#v\nwant %#v", got.Marks, tc.wantMarks)
			}
			if got.IgnoredFiles != tc.wantIgnored {
				t.Fatalf("ignored = %d, want %d", got.IgnoredFiles, tc.wantIgnored)
			}
		})
	}
}

func TestParseCoverprofileKeepsPathsOutsideTheModule(t *testing.T) {
	got, err := Parse("coverprofile", strings.NewReader("mode: set\nother.org/x/a.go:1.1,2.2 1 0\n"), "", Options{ModulePath: "example.com/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Marks["other.org/x/a.go"]; !ok {
		t.Fatalf("path outside the module was rewritten: %#v", got.Marks)
	}
}

func TestParseUnknownFormat(t *testing.T) {
	if _, err := Parse("junit", strings.NewReader(""), "", Options{}); err == nil {
		t.Fatal("unknown format accepted")
	}
}

func TestParseMalformedInput(t *testing.T) {
	for _, format := range []string{"sarif", "go-test-json"} {
		if _, err := Parse(format, strings.NewReader("{not json"), "", Options{}); err == nil {
			t.Fatalf("%s accepted malformed input", format)
		}
	}
}

func TestParseGoTestJoinsThePackageDirectory(t *testing.T) {
	stream := `{"Action":"output","Package":"example.com/demo/internal/api","Test":"TestX","Output":"    api_test.go:9: boom\n"}
{"Action":"fail","Package":"example.com/demo/internal/api","Test":"TestX"}
`
	got, err := Parse("go-test-json", strings.NewReader(stream), "", Options{ModulePath: "example.com/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Path != "internal/api/api_test.go" {
		t.Fatalf("findings = %#v", got.Findings)
	}
}

// Two packages with a test of the same name: a passing test's log lines in
// one package must not become the other package's failure location.
func TestParseGoTestKeepsPackagesApart(t *testing.T) {
	stream := `{"Action":"output","Package":"example.com/demo/a","Test":"TestParse","Output":"    parse_test.go:12: just a log line\n"}
{"Action":"pass","Package":"example.com/demo/a","Test":"TestParse"}
{"Action":"output","Package":"example.com/demo/b","Test":"TestParse","Output":"    parse_test.go:40: wrong result\n"}
{"Action":"fail","Package":"example.com/demo/b","Test":"TestParse"}
`
	got, err := Parse("go-test-json", strings.NewReader(stream), "", Options{ModulePath: "example.com/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Path != "b/parse_test.go" || got.Findings[0].Line != 40 || strings.Contains(got.Findings[0].Message, "log line") {
		t.Fatalf("findings = %#v", got.Findings)
	}
}

// `go test -coverpkg=./... -coverprofile` writes a block once per test
// binary, and merged lcov files repeat SF records: a line is uncovered only
// when no run covered it.
func TestCoverageSumsRepeatedEntries(t *testing.T) {
	profile := "mode: set\nexample.com/demo/a.go:3.1,5.2 2 0\nexample.com/demo/a.go:3.1,5.2 2 1\nexample.com/demo/a.go:7.1,8.2 1 0\nexample.com/demo/a.go:7.1,8.2 1 0\n"
	got, err := Parse("coverprofile", strings.NewReader(profile), "", Options{ModulePath: "example.com/demo"})
	if err != nil {
		t.Fatal(err)
	}
	if marks := got.Marks["a.go"]; len(marks) != 1 || marks[0].Start != 7 {
		t.Fatalf("coverprofile marks = %+v, want only lines 7-8", marks)
	}
	lcov := "SF:a.go\nDA:3,0\nDA:7,0\nend_of_record\nSF:a.go\nDA:3,2\nDA:7,0\nend_of_record\n"
	got, err = Parse("lcov", strings.NewReader(lcov), "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if marks := got.Marks["a.go"]; len(marks) != 1 || marks[0].Start != 7 {
		t.Fatalf("lcov marks = %+v, want only line 7", marks)
	}
}
