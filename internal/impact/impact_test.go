package impact

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

func hunk(lines ...diff.DiffLine) []diff.DiffHunk { return []diff.DiffHunk{{Lines: lines}} }
func add(text string) diff.DiffLine               { return diff.DiffLine{Type: "add", Content: text, NewNum: 1} }
func del(text string) diff.DiffLine               { return diff.DiffLine{Type: "del", Content: text, OldNum: 1} }
func ctx(text string) diff.DiffLine {
	return diff.DiffLine{Type: "context", Content: text, NewNum: 1, OldNum: 1}
}

func TestSymbols(t *testing.T) {
	many := []diff.DiffLine{}
	for i := 0; i < 30; i++ {
		many = append(many, add("func Handler"+strconv.Itoa(100+i)+"() {"))
	}
	cases := []struct {
		name, path string
		hunks      []diff.DiffHunk
		want       []string
	}{
		{"go", "a.go", hunk(add("func (s *Server) ServeLogin(w http.ResponseWriter) {"), add("type Session struct {"), add("const MaxRetries = 3"), ctx("func Untouched() {"), add("	return nil")), []string{"MaxRetries", "ServeLogin", "Session"}},
		{"go deleted", "a.go", hunk(del("func OldName() error {")), []string{"OldName"}},
		{"js", "a.ts", hunk(add("export function renderTree(node) {"), add("class FileTree extends Base {"), add("export const loadFiles = async () => {"), add("  refreshTree(force) {")), []string{"FileTree", "loadFiles", "refreshTree", "renderTree"}},
		{"ruby", "a.rb", hunk(add("  def self.authorize!(user)"), add("class Session < Base"), add("module Billing")), []string{"Billing", "Session", "authorize!"}},
		{"python", "a.py", hunk(add("def load_config(path):"), add("class Loader:")), []string{"Loader", "load_config"}},
		{"stop list and short names", "a.go", hunk(add("func main() {"), add("func init() {"), add("func Run() {"), add("func ab() {")), nil},
		{"unknown language", "a.txt", hunk(add("func Thing() {")), nil},
		{"cap", "a.go", hunk(many...), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Symbols(tc.path, tc.hunks)
			if tc.name == "cap" {
				if len(got) != maxSymbols {
					t.Fatalf("len = %d", len(got))
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Symbols = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsTestPath(t *testing.T) {
	for path, want := range map[string]bool{
		"a_test.go": true, "web/x.spec.ts": true, "web/x.test.js": true, "spec/models/user_spec.rb": true,
		"tests/test_api.py": true, "web/__tests__/x.js": true, "test/e2e/run.sh": true,
		"server.go": false, "contest.go": false, "latest/x.go": false,
	} {
		if IsTestPath(path) != want {
			t.Errorf("IsTestPath(%q) = %v", path, !want)
		}
	}
}

func fakeGrep(hits []vcs.GrepHit) GrepFunc {
	return func(context.Context, []string, string) ([]vcs.GrepHit, error) { return hits, nil }
}

func TestSearchSplitsCallersAndTests(t *testing.T) {
	hits := []vcs.GrepHit{
		{Path: "routes.go", Line: 4, Text: "r.Get(\"/login\", ServeLogin)"},
		{Path: "api_test.go", Line: 9, Text: "ServeLogin(w)"},
		{Path: "api_test.go", Line: 20, Text: "ServeLogin(w)"},
		{Path: "web/login.spec.ts", Line: 2, Text: "ServeLogin"},
		{Path: "vendor/x.go", Line: 1, Text: "ServeLogin"},
	}
	ignored := func(p string) bool { return p == "vendor/x.go" }
	got, err := Search(context.Background(), fakeGrep(hits), "api.go", []string{"ServeLogin"}, map[string]bool{"web/login.spec.ts": true}, ignored)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Callers) != 1 || got.Callers[0] != (Reference{Path: "routes.go", Line: 4, Symbol: "ServeLogin", Text: "r.Get(\"/login\", ServeLogin)"}) {
		t.Fatalf("callers = %+v", got.Callers)
	}
	want := []TestRef{{Path: "api_test.go", Hits: 2}, {Path: "web/login.spec.ts", Hits: 1, Changed: true}}
	if !reflect.DeepEqual(got.Tests, want) {
		t.Fatalf("tests = %+v", got.Tests)
	}
}

func TestSearchTruncates(t *testing.T) {
	var hits []vcs.GrepHit
	for i := 0; i < maxCallers+5; i++ {
		hits = append(hits, vcs.GrepHit{Path: "c" + strconv.Itoa(i) + ".go", Line: 1, Text: "ServeLogin"})
	}
	for i := 0; i < maxTests+3; i++ {
		hits = append(hits, vcs.GrepHit{Path: "t" + strconv.Itoa(i) + "_test.go", Line: 1, Text: "ServeLogin"})
	}
	got, _ := Search(context.Background(), fakeGrep(hits), "api.go", []string{"ServeLogin"}, nil, nil)
	if len(got.Callers) != maxCallers || !got.Truncated.Callers || len(got.Tests) != maxTests || !got.Truncated.Tests {
		t.Fatalf("callers %d %v, tests %d %v", len(got.Callers), got.Truncated.Callers, len(got.Tests), got.Truncated.Tests)
	}
}

func TestSearchWithoutSymbols(t *testing.T) {
	got, err := Search(context.Background(), func(context.Context, []string, string) ([]vcs.GrepHit, error) {
		t.Fatal("grep ran without symbols")
		return nil, nil
	}, "a.go", nil, nil, nil)
	if err != nil || got.Callers == nil || got.Tests == nil || got.Symbols == nil {
		t.Fatalf("got %+v %v", got, err)
	}
}
