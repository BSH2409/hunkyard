package preferences

import (
	"os"
	"testing"
)

// TestMain keeps tests away from the user's real configuration, so a
// configured maintenance host never starts a real model run during tests.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "preferences-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Unsetenv("LEARN_PREFERENCES_HOME")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
