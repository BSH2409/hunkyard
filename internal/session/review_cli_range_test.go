package session

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMovedRangeHeadNote(t *testing.T) {
	dir := initTestRepo(t)
	t.Chdir(dir)
	pinned := gitT(t, dir, "rev-parse", "HEAD")
	focus := &Focus{Kind: FocusRange, HeadSHA: pinned, RangeSpec: "main...HEAD"}

	if note := movedRangeHeadNote(focus, ""); note != "" {
		t.Fatalf("head unchanged: got note %q", note)
	}

	writeFile(t, filepath.Join(dir, "later.txt"), "later\n")
	gitT(t, dir, "add", "later.txt")
	gitT(t, dir, "commit", "-qm", "later")
	note := movedRangeHeadNote(focus, "")
	if !strings.Contains(note, "pinned to main...HEAD at "+pinned[:7]) || !strings.Contains(note, "hunkyard --range main...HEAD") {
		t.Fatalf("head moved: got note %q", note)
	}

	for name, f := range map[string]*Focus{
		"nil":          nil,
		"working tree": {Kind: FocusWorkingTree},
		"no spec":      {Kind: FocusRange, HeadSHA: pinned},
	} {
		if note := movedRangeHeadNote(f, ""); note != "" {
			t.Errorf("%s: got note %q", name, note)
		}
	}
}
