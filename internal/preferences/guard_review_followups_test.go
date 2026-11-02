package preferences

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// Two commands racing on one allow-once: exactly one may pass.
func TestOneAllowOnceAuthorizesOneCommand(t *testing.T) {
	s, ruleID := guardedStore(t, "staged_paths", "*-plan.md")
	repo := guardRepo(t)
	stage(t, repo, "release-plan.md")
	if _, err := s.allowOnce(ruleID); err != nil {
		t.Fatal(err)
	}
	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	passed := 0
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			verdict := s.evaluateCommand(s.loadGuards(), gitAction{Verb: "commit"}, `git commit -m x`, repo, sessionState{}, "s1", true)
			if !verdict.blocked {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if passed != 1 {
		t.Fatalf("%d commands passed on one allow-once, want 1", passed)
	}
}

// `git -C other add . && git -C other commit` names the repository with a
// relative path; the staged files of that repository are checked.
func TestGuardResolvesRelativeGitCAgainstTheShellDirectory(t *testing.T) {
	s, _ := guardedStore(t, "staged_paths", "docs/superpowers/**")
	parent := t.TempDir()
	other := filepath.Join(parent, "other")
	for _, args := range [][]string{{"init", "-q", other}, {"-C", other, "config", "user.email", "t@example.com"}, {"-C", other, "config", "user.name", "t"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(other, "docs/superpowers/plans"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "docs/superpowers/plans/b.md"), []byte("plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", parent, `git -C other add . && git -C other commit -m update`)); !blocked {
		t.Fatal("a relative git -C staged the plan file unchecked")
	}
}
