package vcs

import "testing"

// With diff.renames=false in the user's config, a rename between two SHAs
// still diffs as a rename, not as a whole-file add plus delete.
func TestFileDiffBetweenSHAsPairsARenameDespiteUserConfig(t *testing.T) {
	dir := InitTestRepo(t)
	GitRun(t, dir, "config", "diff.renames", "false")
	base := CommitAtForTest(t, dir, "old.go", "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n", "add")
	GitRun(t, dir, "mv", "old.go", "new.go")
	head := CommitAtForTest(t, dir, "new.go", "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\nfunc D() {}\n", "rename")
	hunks, err := FileDiffBetweenSHAs("new.go", "old.go", base, head, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	adds := 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Type == "add" {
				adds++
			}
		}
	}
	if adds != 1 {
		t.Fatalf("rename diff has %d added lines, want 1: %+v", adds, hunks)
	}
}
