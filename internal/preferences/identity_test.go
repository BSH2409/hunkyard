package preferences

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitCommand(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_COMMITTER_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.test", "GIT_COMMITTER_EMAIL=test@example.test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return string(out)
}

func TestRepositoryIdentityReferenceParity(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	other := filepath.Join(root, "other")
	linked := filepath.Join(root, "linked")
	for _, path := range []string{repo, other} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		gitCommand(t, path, "init", "-q")
		gitCommand(t, path, "commit", "--allow-empty", "-qm", "initial")
	}
	gitCommand(t, repo, "worktree", "add", "--detach", linked)
	gitCommand(t, repo, "config", "extensions.worktreeConfig", "true")
	gitCommand(t, repo, "config", "--worktree", "remote.origin.url", "git@example.invalid:fixture/one.git")
	gitCommand(t, linked, "config", "--worktree", "remote.origin.url", "git@example.invalid:fixture/two.git")
	a, err := repositoryKey(repo)
	if err != nil {
		t.Fatal(err)
	}
	b, err := repositoryKey(linked)
	if err != nil {
		t.Fatal(err)
	}
	c, err := repositoryKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b || a == c {
		t.Fatalf("repository identity: %q %q %q", a, b, c)
	}
	if got, err := repositoryKey(root); err != nil || got != "" {
		t.Fatalf("non-repo key = %q, %v", got, err)
	}
	want := "repo-" + digest(filepath.Join(repo, ".git"))[:12]
	if a != want {
		t.Fatalf("key differs from common-dir reference: %q", a)
	}
}
