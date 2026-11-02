package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuardSeesPathsStagedEarlierInTheSameCommand(t *testing.T) {
	s, _ := guardedStore(t, "staged_paths", "docs/superpowers/**")
	repo := guardRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs/superpowers/plans"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs/superpowers/plans/b.md"), []byte("plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app.go"), []byte("package app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		`git add . && git commit -m "update"`,
		`git add -A; git commit -m "update"`,
		`git add docs/superpowers/plans/b.md && git commit -m "update"`,
		`git add docs && git commit -m "update"`,
	} {
		if blocked, _ := denied(preToolUse(t, s, "s1", repo, command)); !blocked {
			t.Errorf("%s committed the plan file", command)
		}
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git add app.go && git commit -m "code"`)); blocked {
		t.Error("staging only code was blocked")
	}
}
