package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func repositoryInventory(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, ".git"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = computeFileHash(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

type changingSnapshotVCS struct {
	vcs.VCS
	change func()
}

func (v *changingSnapshotVCS) FileContentAtRef(_, _, _ string) (string, error) {
	v.change()
	return "base\n", nil
}

func TestSnapshotConcurrentEdits(t *testing.T) {
	s := &Session{ReviewRound: 1, BaseRef: "base", Files: []*FileEntry{{Path: "changing.go", Status: "modified", Content: "initial\n"}}}
	attempts := 0
	s.VCS = &changingSnapshotVCS{change: func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		attempts++
		s.Files[0].Content = strings.Repeat("changed", attempts)
	}}
	if _, err := s.CaptureReviewSnapshot(); err == nil || !strings.Contains(err.Error(), "changing.go") {
		t.Fatal("continuously changing view was accepted")
	}
	if attempts != 3 {
		t.Fatalf("snapshot attempted %d times, want 3", attempts)
	}
}

func TestSnapshotGitModesSymlinkAndSubmodule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires executable bits and symlink privileges")
	}
	root := initTestRepo(t)
	base := gitT(t, root, "rev-parse", "HEAD")
	for name, content := range map[string][]byte{"script.sh": []byte("#!/bin/sh\n"), "binary.bin": {0, 1, 2, 255}} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("README.md", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "add", ".")
	gitT(t, root, "update-index", "--add", "--cacheinfo", "160000,"+base+",module")
	gitT(t, root, "commit", "-m", "mode and object fixture")
	head := gitT(t, root, "rev-parse", "HEAD")
	s := &Session{Mode: "git", VCS: &vcs.GitVCS{}, RepoRoot: root, ReviewRound: 1, Focus: Focus{Kind: FocusRange, BaseSHA: base, HeadSHA: head}, Files: []*FileEntry{{Path: "script.sh", Status: "added"}, {Path: "link", Status: "added"}, {Path: "module", Status: "added"}, {Path: "binary.bin", Status: "added"}}}
	snapshot, err := s.CaptureReviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"100755", "120000", "160000"} {
		if !bytes.Contains(snapshot.RawChanges, []byte(mode)) {
			t.Fatalf("snapshot lost Git mode %s", mode)
		}
	}
	if !bytes.Contains(snapshot.RawChanges, []byte(base)) {
		t.Fatal("submodule commit identity missing")
	}
}

func TestSnapshotPreservesPartialStaging(t *testing.T) {
	root := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_OPTIONAL_LOCKS", "0")
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("# Staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitT(t, root, "add", "README.md")
	stagedOID := gitT(t, root, "rev-parse", ":README.md")
	if err := os.WriteFile(path, []byte("# Working\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := repositoryInventory(t, root)
	s, err := NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CaptureReviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(snapshot.StagedChanges, []byte(stagedOID)) || string(snapshot.Files[0].Content) != "# Working\n" {
		t.Fatal("snapshot conflated staged and working bytes")
	}
	if !reflect.DeepEqual(before, repositoryInventory(t, root)) {
		t.Fatal("capturing a partial stage changed the index or object store")
	}
}

func TestSnapshotNoRepositoryWrites(t *testing.T) {
	root := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_OPTIONAL_LOCKS", "0")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Modified\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.go"), []byte("package local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := repositoryInventory(t, root)
	s, err := NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CaptureReviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID == "" || len(snapshot.Files) != 2 || string(snapshot.Files[0].Content) != "# Modified\n" {
		t.Fatal("snapshot did not capture displayed content")
	}
	dir := t.TempDir()
	if err := SaveReviewSnapshot(dir, snapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, repositoryInventory(t, root)) {
		t.Fatal("review mutated Git state or object inventory")
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Changed after snapshot\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if string(snapshot.Files[0].Content) != "# Modified\n" {
		t.Fatal("snapshot changed with the working tree")
	}
	if err := SaveReviewSnapshot(dir, snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotCases(t *testing.T) {
	s := &Session{ReviewRound: 1, Files: []*FileEntry{
		{Path: "renamed.txt", OldPath: "old.txt", Status: "renamed", Content: "a\r\nb\r\n"},
		{Path: "deleted.txt", Status: "deleted"},
		{Path: "binary.bin", Status: "added", Content: string([]byte{0, 1, 2, 0xff})},
	}}
	first, err := s.CaptureReviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CaptureReviewSnapshot()
	if err != nil || first.ID != second.ID {
		t.Fatal("unchanged view changed identity")
	}
	byPath := map[string]ReviewSnapshotFile{}
	for _, file := range first.Files {
		byPath[file.Path] = file
		if file.ContentID != fileHash(file.Content) {
			t.Fatal("content identity differs from comment content hash")
		}
	}
	if byPath["renamed.txt"].OldPath != "old.txt" || byPath["deleted.txt"].Status != "deleted" || !bytes.Equal(byPath["binary.bin"].Content, []byte(s.Files[2].Content)) {
		t.Fatal("snapshot lost rename/deletion/binary bytes")
	}
	dir := t.TempDir()
	if err := SaveReviewSnapshot(dir, first); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(filepath.Join(dir, "review-snapshots", first.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var restored ReviewSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, first) {
		t.Fatal("persisted snapshot lost binary or diff content")
	}
	s.mu.Lock()
	s.Files[0].Content = "changed"
	s.mu.Unlock()
	changed, err := s.CaptureReviewSnapshot()
	if err != nil || changed.ID == first.ID {
		t.Fatal("changed content retained snapshot identity")
	}
}

func TestRealSessionsPreserveBinaryChanges(t *testing.T) {
	root := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	files := map[string][]byte{"image.png": {0, 1, 255}, "data.bin": {0, 254}, "drawing.svg": []byte("<svg><title>source</title></svg>\n")}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(s *Session) {
		t.Helper()
		snapshot, err := s.CaptureReviewSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Files) != len(files) {
			t.Fatalf("review omitted files: %#v", snapshot.Files)
		}
		for _, file := range snapshot.Files {
			if !bytes.Equal(file.Content, files[file.Path]) {
				t.Fatalf("lost bytes for %s", file.Path)
			}
			view, ok := s.GetFileSnapshot(file.Path)
			if !ok {
				t.Fatal("missing file", file.Path)
			}
			if file.Path == "drawing.svg" {
				if view["file_type"] != "code" || view["content"] != string(files[file.Path]) {
					t.Fatal("SVG must remain reviewable source", view)
				}
			} else {
				if view["file_type"] != "binary" || view["content"] != "" {
					t.Fatal("binary bytes exposed as text", view)
				}
				diff, ok := s.GetFileDiffSnapshot(file.Path, false)
				if !ok || len(diff["hunks"].([]vcs.DiffHunk)) != 0 {
					t.Fatal("binary diff rendered as text", diff)
				}
			}
		}
	}
	s, err := NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	check(s)
	base := gitT(t, root, "rev-parse", "HEAD")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-m", "binary changes")
	s, err = NewGitSessionLenient(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFocus(Focus{Kind: FocusRange, BaseSHA: base, HeadSHA: gitT(t, root, "rev-parse", "HEAD")}); err != nil {
		t.Fatal(err)
	}
	check(s)
}

func TestCommitRangeRemainsAvailableWithGitFilters(t *testing.T) {
	root := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	base := gitT(t, root, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(root, "README.md"), "# Updated\n")
	gitT(t, root, "add", "README.md")
	gitT(t, root, "commit", "-m", "range change")
	writeFile(t, filepath.Join(root, ".gitattributes"), "README.md filter=lfs\n")
	if _, err := NewSessionFromGit(nil); err == nil {
		t.Fatal("dirty filtered comparison accepted")
	}
	if _, err := NewSessionFromFiles([]string{"README.md"}, nil); err == nil {
		t.Fatal("file-mode filtered comparison accepted")
	}
	s, err := NewGitSessionLenient(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFocus(Focus{Kind: FocusRange, BaseSHA: base, HeadSHA: gitT(t, root, "rev-parse", "HEAD")}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CaptureReviewSnapshot()
	if err != nil || len(snapshot.Files) != 1 || string(snapshot.Files[0].Content) != "# Updated\n" {
		t.Fatal("raw commit comparison unavailable", snapshot, err)
	}
}
