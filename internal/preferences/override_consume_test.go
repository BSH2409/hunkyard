package preferences

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// An override that could not be removed was not used: the guard must not
// let the action through, or the override would allow it twice.
func TestConsumeOverrideSingleUseAfterFailedWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory permissions enforced for a non-root user")
	}
	s := testStore(t)
	if err := os.MkdirAll(filepath.Dir(s.dataPath("overrides.json")), 0o755); err != nil {
		t.Fatal(err)
	}
	expires := s.config.Clock().Add(overrideLifetime).UTC().Format(time.RFC3339)
	if err := s.writeOverrides([]override{{RuleID: "r1", ExpiresAt: expires}}); err != nil {
		t.Fatal(err)
	}
	lock := s.dataPath("overrides.json") + ".lock"
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(s.dataPath("overrides.json"))
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if s.consumeOverride("r1") {
		t.Error("override reported as used although it is still on disk")
	}
	_ = os.Chmod(dir, 0o755)
	if !s.consumeOverride("r1") {
		t.Error("the override was lost: the positive control could not use it")
	}
}
