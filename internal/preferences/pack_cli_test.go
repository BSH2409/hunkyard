package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runPack(t *testing.T, root string, args ...string) (map[string]any, int) {
	t.Helper()
	var out bytes.Buffer
	code := Run(append(append([]string{"pack"}, args...), "--root", root, "--json"), strings.NewReader(""), &out, &out)
	var decoded map[string]any
	_ = json.Unmarshal(out.Bytes(), &decoded)
	return decoded, code
}

func TestPackCLI(t *testing.T) {
	s := testStore(t)
	root := s.config.Root
	dir := t.TempDir()
	writePack(t, dir, "backend", "1.0.0", map[string][]string{"api": {"Return 404 for unknown ids."}})

	if out, code := runPack(t, root, "import", dir, "--dry-run"); code != 0 || out["status"] != "dry_run" {
		t.Fatalf("dry run = %v %d", out, code)
	}
	out, code := runPack(t, root, "import", dir)
	if code != 0 || out["status"] != "imported" || out["pack"] != "shared-backend" {
		t.Fatalf("import = %v %d", out, code)
	}
	out, code = runPack(t, root, "list")
	packs, _ := out["packs"].([]any)
	if code != 0 || len(packs) != 1 {
		t.Fatalf("list = %v %d", out, code)
	}
	if out, code = runPack(t, root, "remove", "shared-backend"); code != 0 || out["removed"] != float64(1) {
		t.Fatalf("remove = %v %d", out, code)
	}
	for _, args := range [][]string{{}, {"import"}, {"remove"}, {"frobnicate"}, {"import", dir, "--scope", "galaxy"}} {
		if _, code := runPack(t, root, args...); code != 2 {
			t.Fatalf("%v exit = %d, want usage 2", args, code)
		}
	}
	if out, code := runPack(t, root, "remove", "shared-missing"); code != 3 || out["status"] != "rejected" {
		t.Fatalf("remove unknown = %v %d", out, code)
	}
}

func TestExportCLIWritesRepositoryMarkdown(t *testing.T) {
	s := testStore(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	target := filepath.Join(t.TempDir(), "conventions.md")
	var out bytes.Buffer
	code := Run([]string{"export", "--scope", "repository", "--cwd", repo, "--to", target, "--root", s.config.Root, "--json"}, strings.NewReader(""), &out, &out)
	if code != 0 {
		t.Fatalf("export exit %d: %s", code, out.String())
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), "# Repository preferences") {
		t.Fatalf("file = %q %v", data, err)
	}
	out.Reset()
	if code := Run([]string{"export", "--scope", "global", "--to", target, "--root", s.config.Root}, strings.NewReader(""), &out, &out); code != 2 {
		t.Fatalf("global export exit = %d, want usage", code)
	}
}
