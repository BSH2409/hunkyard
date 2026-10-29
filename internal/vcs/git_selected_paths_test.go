package vcs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGitForPathsTreatsPathspecCharactersLiterally(t *testing.T) {
	dir := InitTestRepo(t)
	for _, name := range []string{"code[1].go", "code1.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := (&GitVCS{}).ForPaths([]string{"code[1].go"})
	changes, err := git.ChangedFilesOnDefaultInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileChange{{Path: "code[1].go", Status: "untracked"}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("selected changes = %+v, want %+v", changes, want)
	}
	GitRun(t, dir, "--literal-pathspecs", "add", ".")
	GitRun(t, dir, "commit", "-m", "base literal paths")
	base := GitRun(t, dir, "rev-parse", "HEAD")
	for name, content := range map[string]string{"code[1].go": "literal_after\n", "code1.go": "other_after\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(hunks []DiffHunk, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if len(hunks) != 1 {
			t.Fatalf("got %d hunks, want only the literal file", len(hunks))
		}
		for _, line := range hunks[0].Lines {
			if strings.Contains(line.Content, "other_after") {
				t.Fatal("selected diff included an unselected file")
			}
		}
	}
	check(git.FileDiffUnified("code[1].go", base, dir, false))
	check(git.FileDiffScoped("code[1].go", "unstaged", base, dir, false))
	GitRun(t, dir, "add", ".")
	check(git.FileDiffScoped("code[1].go", "staged", base, dir, false))
	GitRun(t, dir, "commit", "-m", "change both files")
	head := GitRun(t, dir, "rev-parse", "HEAD")
	check(git.FileDiffForCommit("code[1].go", head, dir, false))
	check(git.FileDiffBetweenSHAs("code[1].go", "", base, head, dir, false))
}

func TestParseNameStatusZPreservesFilenameWhitespaceAndRenames(t *testing.T) {
	changes := parseNameStatusZ([]byte("M\x00line\nbreak.go\x00R100\x00old\tname.go\x00new\tname.go\x00D\x00removed.go\x00"))
	want := []FileChange{
		{Path: "line\nbreak.go", Status: "modified"},
		{OldPath: "old\tname.go", Path: "new\tname.go", Status: "renamed"},
		{Path: "removed.go", Status: "deleted"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %+v, want %+v", changes, want)
	}
}
