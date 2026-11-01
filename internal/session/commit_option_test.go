package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestCommitSelectionCannotPassGitOptions(t *testing.T) {
	newBudgetRepo(t, 25, 0)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	sess.GetSessionInfoScoped(context.Background(), "all", "--output="+victim)
	sess.GetFileDiffSnapshotScoped("src/file1.go", "all", "--output="+victim, false)
	if data, _ := os.ReadFile(victim); string(data) != "precious" {
		t.Fatalf("a commit selection reached git as an option: victim now %q", data)
	}
}

func TestBaseRefCannotPassGitOptions(t *testing.T) {
	newBudgetRepo(t, 25, 0)
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sess.ChangeBaseBranch("--output=" + victim); err == nil {
		t.Fatal("a base ref starting with - was accepted")
	}
	if data, _ := os.ReadFile(victim); string(data) != "precious" {
		t.Fatalf("a base ref reached git as an option: victim now %q", data)
	}
}
