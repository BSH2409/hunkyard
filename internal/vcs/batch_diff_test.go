package vcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// batchFixture builds a working tree whose changes cover the cases a batched
// diff must split correctly: a rename with spaces and an edit, a quoted name,
// a UTF-8 name, a binary file, a mode-only change, a deletion, a file whose
// name starts with "diff --git", a staged added file and an untracked file.
func batchFixture(t *testing.T) string {
	t.Helper()
	dir := initTestRepo(t)
	files := map[string]string{
		"a b.txt":            "one\ntwo\nthree\n",
		"spécial.txt":        "café\n",
		"run.sh":             "echo hi\n",
		"gone.txt":           "bye\n",
		"diff --git a b.txt": "line\n",
		"keep.txt":           "x\n",
	}
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base")
	gitT(t, dir, "mv", "a b.txt", "c d.txt")
	writeFile(t, filepath.Join(dir, "c d.txt"), "one\nTWO\nthree\n")
	writeFile(t, filepath.Join(dir, "spécial.txt"), "café au lait\n")
	writeFile(t, filepath.Join(dir, "diff --git a b.txt"), "line\ndiff --git x y\n")
	writeFile(t, filepath.Join(dir, "keep.txt"), "y\n")
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0, 9, 9, 9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	// Windows file names cannot contain a double quote.
	if runtime.GOOS != "windows" {
		writeFile(t, filepath.Join(dir, `quo"te.txt`), "quoted\n")
		gitT(t, dir, "add", `quo"te.txt`)
	}
	writeFile(t, filepath.Join(dir, "new.txt"), "brand\nnew\n")
	return dir
}

// perFileHunks is what the existing per-file path returns for a change.
func perFileHunks(t *testing.T, dir string, fc FileChange) []DiffHunk {
	t.Helper()
	if fc.Status == "added" || fc.Status == "untracked" {
		data, err := os.ReadFile(filepath.Join(dir, fc.Path))
		if err != nil {
			t.Fatal(err)
		}
		return FileDiffUnifiedNewFile(string(data))
	}
	hunks, err := FileDiffUnifiedCtx(context.Background(), fc.Path, fc.OldPath, "HEAD", dir, false)
	if err != nil {
		t.Fatalf("per-file diff %s: %v", fc.Path, err)
	}
	return hunks
}

func TestSplitBatchedPatchEdgeCases(t *testing.T) {
	dir := batchFixture(t)
	changes, err := changedFilesOnDefaultInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) < 9 {
		t.Fatalf("fixture produced only %d changes: %+v", len(changes), changes)
	}
	batch, err := (&GitVCS{}).DiffAll(context.Background(), dir, DiffSpec{Base: "HEAD", WholeAdded: true}, changes)
	if err != nil {
		t.Fatal(err)
	}
	for _, fc := range changes {
		want := perFileHunks(t, dir, fc)
		if got := batch[fc.Path]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s (%s): batched hunks differ\n got %+v\nwant %+v", fc.Path, fc.Status, got, want)
		}
	}
}

func TestNumstatAllMatchesHunkCounts(t *testing.T) {
	dir := batchFixture(t)
	changes, err := changedFilesOnDefaultInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := (&GitVCS{}).NumstatAll(context.Background(), dir, DiffSpec{Base: "HEAD", WholeAdded: true}, changes)
	if err != nil {
		t.Fatal(err)
	}
	for _, fc := range changes {
		got := stats[fc.Path]
		if fc.Path == "bin.dat" {
			if !got.Binary {
				t.Errorf("bin.dat not reported as binary: %+v", got)
			}
			continue
		}
		var adds, dels int
		for _, h := range perFileHunks(t, dir, fc) {
			for _, l := range h.Lines {
				switch l.Type {
				case "add":
					adds++
				case "del":
					dels++
				}
			}
		}
		if got.Additions != adds || got.Deletions != dels {
			t.Errorf("%s: numstat +%d -%d, hunks +%d -%d", fc.Path, got.Additions, got.Deletions, adds, dels)
		}
	}
}

func TestDiffAllFallsBackWhenRenameDetectionDisagrees(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\nfour\n")
	gitT(t, dir, "add", "a.txt")
	gitT(t, dir, "commit", "-qm", "base")
	gitT(t, dir, "mv", "a.txt", "b.txt")
	// With renames off the review lists a deletion and an addition, while
	// the batch (-M) pairs them as a rename: a.txt would get no hunks.
	changes := []FileChange{{Path: "a.txt", Status: "deleted"}, {Path: "b.txt", Status: "added"}}
	_, err := (&GitVCS{}).DiffAll(context.Background(), dir, DiffSpec{Base: "HEAD"}, changes)
	if !errors.Is(err, errBatchSplit) {
		t.Fatalf("err = %v, want errBatchSplit so callers diff file by file", err)
	}
}

func TestBatchDiffsOnlyTheRequestedFiles(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "one.go"), "package p\n")
	writeFile(t, filepath.Join(dir, "two.go"), "package p\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-qm", "base")
	writeFile(t, filepath.Join(dir, "one.go"), "package p\n\nvar A = 1\n")
	writeFile(t, filepath.Join(dir, "two.go"), "package p\n\nvar B = 2\n")
	out, err := (&GitVCS{}).runBatch(context.Background(), dir, DiffSpec{Base: "HEAD"}, []FileChange{{Path: "one.go", Status: "modified"}}, "--raw", "-z", "-p")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "one.go") || strings.Contains(string(out), "two.go") {
		t.Fatalf("batch for one.go diffed other files:\n%s", out)
	}
}
