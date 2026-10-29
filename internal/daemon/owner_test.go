package daemon

import (
	"os"
	"os/exec"
	"testing"

	"github.com/BSH2409/hunkyard/internal/localonly"
)

func TestOwnerSessionKeyIsolation(t *testing.T) {
	if os.Getenv("HUNKYARD_OWNER_KEY_TEST") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestOwnerSessionKeyIsolation$", "-test.count=1")
		command.Env = append(os.Environ(), "HUNKYARD_OWNER_KEY_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("owner key check: %s: %v", output, err)
		}
		return
	}
	localonly.Install()
	t.Setenv("HUNKYARD_OWNER_ID", "codex:one")
	first := SessionKey("/same/repo", "feature", nil)
	if first != SessionKey("/same/repo", "feature", nil) {
		t.Fatal("owner reconnect changed session key")
	}
	if first == SessionKey("/same/repo", "other-branch", nil) {
		t.Fatal("owner scoping lost branch isolation")
	}
	file := SessionKey("/same/repo", "feature", []string{"notes.md"})
	if file != SessionKey("/same/repo", "other-branch", []string{"notes.md"}) {
		t.Fatal("file review became branch-dependent")
	}
	t.Setenv("HUNKYARD_OWNER_ID", "claude:two")
	if first == SessionKey("/same/repo", "feature", nil) || file == SessionKey("/same/repo", "feature", []string{"notes.md"}) {
		t.Fatal("different launching sessions shared a review key")
	}
}
