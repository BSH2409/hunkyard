package diff

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// checkDiff verifies that entries rebuild both inputs with consistent line
// numbers and returns how many lines it kept unchanged.
func checkDiff(t *testing.T, oldText, newText string, entries []DiffEntry) int {
	t.Helper()
	var rebuiltOld, rebuiltNew []string
	unchanged := 0
	for _, e := range entries {
		switch e.Type {
		case "unchanged":
			unchanged++
			rebuiltOld, rebuiltNew = append(rebuiltOld, e.Text), append(rebuiltNew, e.Text)
			if e.OldLine != len(rebuiltOld) || e.NewLine != len(rebuiltNew) {
				t.Fatalf("unchanged line numbers %d/%d, want %d/%d", e.OldLine, e.NewLine, len(rebuiltOld), len(rebuiltNew))
			}
		case "removed":
			rebuiltOld = append(rebuiltOld, e.Text)
			if e.OldLine != len(rebuiltOld) {
				t.Fatalf("removed line number %d, want %d", e.OldLine, len(rebuiltOld))
			}
		case "added":
			rebuiltNew = append(rebuiltNew, e.Text)
			if e.NewLine != len(rebuiltNew) {
				t.Fatalf("added line number %d, want %d", e.NewLine, len(rebuiltNew))
			}
		}
	}
	if strings.Join(rebuiltOld, "\n") != strings.Join(SplitLines(oldText), "\n") || strings.Join(rebuiltNew, "\n") != strings.Join(SplitLines(newText), "\n") {
		t.Fatal("diff does not rebuild its inputs")
	}
	return unchanged
}

func TestComputeLineDiffStaysOptimalWithTrimming(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	words := []string{"a", "b", "c", "a", "b", "- item", "", "## h"}
	gen := func(n int) string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = words[rng.Intn(len(words))]
		}
		return strings.Join(lines, "\n")
	}
	for i := 0; i < 300; i++ {
		oldText, newText := gen(rng.Intn(30)), gen(rng.Intn(30))
		if i%3 == 0 { // shared prefix and suffix, the case trimming shortcuts
			oldText, newText = "p\nq\n"+oldText+"\nz", "p\nq\n"+newText+"\nz"
		}
		got := checkDiff(t, oldText, newText, ComputeLineDiff(oldText, newText))
		want := checkDiff(t, oldText, newText, hirschbergDiff(SplitLines(oldText), SplitLines(newText), 0, 0))
		if got != want {
			t.Fatalf("case %d: kept %d unchanged lines, the full diff keeps %d", i, got, want)
		}
	}
}

func TestComputeLineDiffLargePlanWithOneEdit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&b, "- plan line %d\n", i)
	}
	oldText := b.String()
	newText := strings.Replace(oldText, "- plan line 5000\n", "- plan line 5000 (edited)\n", 1)
	start := time.Now()
	entries := ComputeLineDiff(oldText, newText)
	elapsed := time.Since(start)
	checkDiff(t, oldText, newText, entries)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("one edit in a 10,000-line plan took %v", elapsed)
	}
}
