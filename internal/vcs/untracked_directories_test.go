package vcs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestUntrackedDirectorySymlinkDiscovery(t *testing.T) {
	root := testutil.InitTestRepo(t)
	testutil.WriteFile(t, filepath.Join(root, ".gitignore"), "node_modules/\n")
	testutil.Git(t, root, "add", ".gitignore")
	testutil.Git(t, root, "commit", "-m", "ignore directories")
	target := t.TempDir()
	testutil.WriteFile(t, filepath.Join(target, "dependency.js"), "external\n")
	if err := os.Symlink(target, filepath.Join(root, "node_modules")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	testutil.WriteFile(t, filepath.Join(root, "regular.txt"), "regular\n")
	if err := os.Symlink("regular.txt", filepath.Join(root, "file-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.txt", filepath.Join(root, "broken-link")); err != nil {
		t.Fatal(err)
	}
	scans := map[string]func() ([]FileChange, error){
		"normal":  func() ([]FileChange, error) { return untrackedFilesInDir(root) },
		"full":    func() ([]FileChange, error) { return untrackedFilesFullScanInDir(root) },
		"listing": func() ([]FileChange, error) { return workingTreeListing(root, nil) },
		"selected": func() ([]FileChange, error) {
			return workingTreeListing(root, []string{"node_modules", "regular.txt", "file-link", "broken-link"})
		},
		"initial": func() ([]FileChange, error) {
			changes, _, _, err := initialGitChangesAndScopes("", root)
			return changes, err
		},
	}
	for name, scan := range scans {
		t.Run(name, func(t *testing.T) {
			changes, err := scan()
			if err != nil {
				t.Fatal(err)
			}
			paths := map[string]bool{}
			for _, change := range changes {
				paths[change.Path] = true
			}
			if paths["node_modules"] {
				t.Fatalf("directory symlink included: %+v", changes)
			}
			for _, path := range []string{"regular.txt", "file-link", "broken-link"} {
				if !paths[path] {
					t.Errorf("%s missing: %+v", path, changes)
				}
			}
		})
	}
}
