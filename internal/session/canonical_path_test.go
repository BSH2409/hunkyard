package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalAbsPathResolvesMissingFileThroughExistingParent(t *testing.T) {
	real := t.TempDir()
	real, _ = filepath.EvalSymlinks(real)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	got, err := canonicalAbsPath(filepath.Join(link, "src", "deleted.go"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(real, "src", "deleted.go"); got != want {
		t.Fatalf("canonicalAbsPath = %q, want %q", got, want)
	}
}
