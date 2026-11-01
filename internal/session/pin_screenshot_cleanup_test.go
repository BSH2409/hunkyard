package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinScreenshotCleanupOnApproval(t *testing.T) {
	const pin = "a1b2c3d4-0000-4000-8000-000000000001.png"
	const pasted = "a1b2c3d4-0000-4000-8000-000000000002.png"
	review := `{"files":{"/":{"status":"","file_hash":"","comments":[{"id":"c1","body":"x","pin_screenshot":{"file":"` + pin + `"},"replies":[]},{"id":"c2","body":"![a](attachments/` + pasted + `)"}]}}}`
	setup := func(t *testing.T) ReviewPaths {
		t.Helper()
		p := ReviewPathsFor(t.TempDir())
		if err := os.MkdirAll(p.Attachments, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{p.Review: review, filepath.Join(p.Attachments, pin): "png", filepath.Join(p.Attachments, pasted): "png"} {
			if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }

	t.Run("cleanup off removes only pin screenshots", func(t *testing.T) {
		p := setup(t)
		cleanupOnApproval(true, p.Folder, false)
		if exists(filepath.Join(p.Attachments, pin)) {
			t.Error("pin screenshot kept after approval")
		}
		if !exists(filepath.Join(p.Attachments, pasted)) || !exists(p.Review) {
			t.Error("review or pasted image removed with cleanup_on_approve off")
		}
	})
	t.Run("not approved keeps everything", func(t *testing.T) {
		p := setup(t)
		cleanupOnApproval(false, p.Folder, false)
		if !exists(filepath.Join(p.Attachments, pin)) {
			t.Error("pin screenshot removed without approval")
		}
	})
	t.Run("cleanup on keeps published feedback but removes pin screenshots", func(t *testing.T) {
		p := setup(t)
		if err := os.MkdirAll(filepath.Join(p.Folder, "feedback"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Folder, "feedback", "state.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		cleanupOnApproval(true, p.Folder, true)
		if exists(filepath.Join(p.Attachments, pin)) {
			t.Error("pin screenshot kept in a review folder preserved for its feedback")
		}
		if !exists(filepath.Join(p.Folder, "feedback", "state.json")) {
			t.Error("published feedback removed")
		}
	})
	t.Run("cleanup on removes the folder", func(t *testing.T) {
		p := setup(t)
		cleanupOnApproval(true, p.Folder, true)
		if exists(p.Folder) {
			t.Error("review folder kept with cleanup_on_approve on")
		}
	})
}
