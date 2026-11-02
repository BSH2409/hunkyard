package vcs

import "testing"

// One diff can hold two sections for a path (a file replaced by a symlink,
// or a rename shown as add plus delete); the second section's header lines
// are not lines of the first hunk.
func TestParseUnifiedDiffStopsAtTheNextSection(t *testing.T) {
	diff := "diff --git a/tc b/tc\n" +
		"deleted file mode 100644\n" +
		"--- a/tc\n" +
		"+++ /dev/null\n" +
		"@@ -1,2 +0,0 @@\n" +
		"-one\n" +
		"-two\n" +
		"diff --git a/tc b/tc\n" +
		"new file mode 120000\n" +
		"--- /dev/null\n" +
		"+++ b/tc\n" +
		"@@ -0,0 +1 @@\n" +
		"+target\n" +
		"\\ No newline at end of file\n"
	hunks := ParseUnifiedDiff(diff)
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2: %+v", len(hunks), hunks)
	}
	if got := len(hunks[0].Lines); got != 2 {
		t.Fatalf("first hunk has %d lines, want 2: %+v", got, hunks[0].Lines)
	}
	if got := hunks[1].Lines; len(got) != 1 || got[0].Content != "target" || !got[0].NoFinalNewline {
		t.Fatalf("second hunk = %+v", got)
	}
}
