package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReviewerEdits(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	agent := "a\nb\nc\nd\n"
	s := &Session{Mode: "git", RepoRoot: dir, Files: []*FileEntry{
		{Path: "edited.go", AbsPath: write("edited.go", "a\nB2\nc\nd\nadded\n"), FileType: "code", Status: "modified", Content: agent},
		{Path: "same.go", AbsPath: write("same.go", agent), FileType: "code", Status: "modified", Content: agent},
		{Path: "image.png", AbsPath: write("image.png", "new bytes"), FileType: "binary", Status: "modified", Content: "old bytes"},
		{Path: "gone.go", AbsPath: write("gone.go", "x\n"), FileType: "code", Status: "deleted", Content: ""},
	}}
	got := s.ReviewerEdits()
	want := []ReviewerEdit{
		{Path: "edited.go", StartLine: 2, EndLine: 2, Before: []string{"b"}, After: []string{"B2"}},
		{Path: "edited.go", StartLine: 4, EndLine: 4, Before: nil, After: []string{"added"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edits:\n got %#v\nwant %#v", got, want)
	}
}

func reviewerEditFixture(t *testing.T) *Session {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("one\nTWO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Session{Mode: "git", RepoRoot: dir, ReviewRound: 2, Files: []*FileEntry{{Path: "a.go", AbsPath: path, FileType: "code", Status: "modified", Content: "one\ntwo\n"}}}
}

func TestReviewerEditsSkippedWhenDiskIsNotTheReviewedSource(t *testing.T) {
	ranged := reviewerEditFixture(t)
	ranged.Focus = Focus{Kind: FocusRange}
	if edits := ranged.ReviewerEdits(); len(edits) != 0 {
		t.Fatalf("range focus compared a commit blob with the working tree: %+v", edits)
	}
	replied := reviewerEditFixture(t)
	replied.Files[0].Comments = []Comment{{ID: "c1", Replies: []Reply{{ID: "r1", AuthorKind: "agent", ReviewRound: 2}}}}
	if edits := replied.ReviewerEdits(); len(edits) != 0 {
		t.Fatalf("agent worked this round, so disk edits may be its own: %+v", edits)
	}
	live := reviewerEditFixture(t)
	live.Files[0].Comments = []Comment{{ID: "c1", Live: true}}
	if edits := live.ReviewerEdits(); len(edits) != 0 {
		t.Fatalf("a live agent request was sent this round: %+v", edits)
	}
	if edits := reviewerEditFixture(t).ReviewerEdits(); len(edits) != 1 {
		t.Fatalf("plain reviewer edit lost: %+v", edits)
	}
}
