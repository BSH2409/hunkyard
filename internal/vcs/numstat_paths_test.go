package vcs

import "testing"

// Renames inside a folder ("dir/{old.go => new.go}") and non-ASCII paths
// (quoted by git) keep their line counts.
func TestDiffNumstatKeysRenamesAndNonASCIIPaths(t *testing.T) {
	dir := InitTestRepo(t)
	base := CommitAtForTest(t, dir, "dir/old.go", "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n", "add")
	CommitAtForTest(t, dir, "dir/é.go", "package a\n", "add accented")
	GitRun(t, dir, "mv", "dir/old.go", "dir/new.go")
	head := CommitAtForTest(t, dir, "dir/new.go", "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\nfunc D() {}\n", "rename")
	stats, err := runDiffNumstat(dir, "-M", base, head)
	if err != nil {
		t.Fatal(err)
	}
	if got := stats["dir/new.go"]; got.Additions != 1 {
		t.Errorf("renamed file stats = %+v, want 1 addition (all: %v)", got, stats)
	}
	if got := stats["dir/é.go"]; got.Additions != 1 {
		t.Errorf("non-ASCII path stats = %+v, want 1 addition (all: %v)", got, stats)
	}
}
