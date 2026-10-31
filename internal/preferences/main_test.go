package preferences

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps tests away from the user's real configuration, so a
// configured maintenance host never starts a real model run during tests.
func TestMain(m *testing.M) {
	if !SupportedPlatform {
		// TestOpenRefusesUnsupportedPlatform covers this platform from Linux.
		fmt.Println("skipping preference engine tests:", ErrUnsupportedPlatform)
		os.Exit(0)
	}
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
