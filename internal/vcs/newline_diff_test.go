package vcs

import "testing"

func TestUnifiedDiffPreservesFinalNewlineMarkers(t *testing.T) {
	for _, side := range []string{"old", "new", "both"} {
		t.Run(side, func(t *testing.T) {
			patch := "@@ -1 +1 @@\n-before\n"
			if side != "new" {
				patch += "\\ No newline at end of file\n"
			}
			patch += "+after\n"
			if side != "old" {
				patch += "\\ No newline at end of file\n"
			}
			hunks := ParseUnifiedDiff(patch)
			if len(hunks) != 1 || len(hunks[0].Lines) != 2 {
				t.Fatal("marker changed hunk line numbering")
			}
			if hunks[0].Lines[0].OldNum != 1 || hunks[0].Lines[1].NewNum != 1 {
				t.Fatal("newline marker shifted source line anchors")
			}
			if hunks[0].Lines[0].NoFinalNewline != (side != "new") || hunks[0].Lines[1].NoFinalNewline != (side != "old") {
				t.Fatal("final-newline metadata attached to wrong side")
			}
		})
	}
}
