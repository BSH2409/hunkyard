package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPackRoundTrips(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Prefer table tests in Go.", "testing")
	captureRule(t, s, "Return early from handlers.", "api", "services")
	out := filepath.Join(t.TempDir(), "pack")
	result, err := s.exportPack(packExport{name: "backend", version: "1.0.0", author: "team", scope: "global", out: out})
	if err != nil {
		t.Fatal(err)
	}
	if result["rules"] != 2 || result["skipped"] != 0 {
		t.Fatalf("export = %+v", result)
	}
	api, err := os.ReadFile(filepath.Join(out, "reference", "api.md"))
	if err != nil || !strings.Contains(string(api), "- Return early from handlers.") {
		t.Fatalf("api.md = %q %v", api, err)
	}
	for _, leak := range []string{"obs_", "rule_", "session", "evidence"} {
		data, _ := os.ReadFile(filepath.Join(out, "reference", "testing.md"))
		if strings.Contains(string(data), leak) {
			t.Fatalf("export leaked %q: %s", leak, data)
		}
	}
	other := testStore(t)
	imported, err := other.importPack(out, packTarget{scope: "global"}, false)
	if err != nil || imported.Added != 2 || imported.Pack != "shared-backend" {
		t.Fatalf("re-import of the export = %+v %v", imported, err)
	}
}

func TestExportPackRejectsBadNamesAndExistingFolders(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Prefer table tests in Go.", "testing")
	if _, err := s.exportPack(packExport{name: "Back End", version: "1", scope: "global", out: filepath.Join(t.TempDir(), "p")}); err == nil {
		t.Fatal("invalid pack name accepted")
	}
	existing := t.TempDir()
	if err := os.WriteFile(filepath.Join(existing, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exportPack(packExport{name: "backend", version: "1", scope: "global", out: existing}); err == nil {
		t.Fatal("export overwrote a non-empty folder")
	}
}

func TestExportRepositoryMarkdown(t *testing.T) {
	s := testStore(t)
	if got := s.repositoryMarkdown("orion-0123456789ab"); !strings.Contains(got, "No repository preferences yet.") {
		t.Fatalf("empty export = %q", got)
	}
	if _, err := s.Capture(CaptureInput{Feedback: "x", Directive: "Keep migrations reversible.", Scope: "repository", RepoKey: "orion-0123456789ab", Surfaces: []string{"database"}, Explicit: true, Source: "cli", SessionID: "s", TurnID: "t"}); err != nil {
		t.Fatal(err)
	}
	got := s.repositoryMarkdown("orion-0123456789ab")
	if !strings.Contains(got, "# Repository preferences") || !strings.Contains(got, "## Database") || !strings.Contains(got, "- Keep migrations reversible.") {
		t.Fatalf("export = %q", got)
	}
}
