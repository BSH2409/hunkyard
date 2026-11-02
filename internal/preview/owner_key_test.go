package preview

import (
	"os"
	"os/exec"
	"testing"

	"github.com/BSH2409/hunkyard/internal/localonly"
)

func TestPreviewSessionKeyIsScopedToTheLaunchingSession(t *testing.T) {
	if os.Getenv("HUNKYARD_OWNER_KEY_TEST") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestPreviewSessionKeyIsScopedToTheLaunchingSession$", "-test.count=1")
		command.Env = append(os.Environ(), "HUNKYARD_OWNER_KEY_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("owner key check: %s: %v", output, err)
		}
		return
	}
	localonly.Install()
	t.Setenv("HUNKYARD_OWNER_ID", "codex:one")
	first := PreviewSessionKey("/repo", "/repo/page.html")
	if first != PreviewSessionKey("/repo", "/repo/page.html") {
		t.Fatal("owner reconnect changed the preview key")
	}
	t.Setenv("HUNKYARD_OWNER_ID", "claude:two")
	if first == PreviewSessionKey("/repo", "/repo/page.html") {
		t.Fatal("different launching sessions shared a preview review key")
	}
}
