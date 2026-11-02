package preferences

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A diff check bound to another repository's rule must not block finishing
// work here.
func TestStopIgnoresDiffChecksOfOtherRepositories(t *testing.T) {
	s := testStore(t)
	input := CaptureInput{Feedback: "Write methods over several lines.", Directive: "Write methods over several lines.", Scope: "repository", RepoKey: "orion-0123456789ab", Surfaces: []string{"ruby"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "seed"}
	result, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addGuard("diff_check", result.RuleID, []string{"ruby_single_line_def"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	repo := guardRepo(t)
	stage(t, repo, "app.rb")
	if out, err := exec.Command("git", "-C", repo, "commit", "-qm", "init").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "app.rb"), []byte("x\ndef a; 1; end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if response := stopResponse(t, s, repo, false); response["decision"] == "block" {
		t.Fatalf("another repository's rule blocked this repository: %v", response)
	}
}
