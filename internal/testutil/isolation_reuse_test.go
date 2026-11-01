package testutil

import (
	"os"
	"testing"
)

func TestIsolatedHomeIsReusedByChildProcesses(t *testing.T) {
	t.Setenv(isolatedHomeEnv, "")
	home, cleanup := isolatedHome()
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("parent home not created: %v", err)
	}
	if os.Getenv(isolatedHomeEnv) != home {
		t.Fatal("parent did not record its home for child processes")
	}
	// A helper child sees the recorded home: it reuses it and must not remove it.
	childHome, childCleanup := isolatedHome()
	if childHome != home {
		t.Fatalf("child created its own home %s, want %s", childHome, home)
	}
	childCleanup()
	if _, err := os.Stat(home); err != nil {
		t.Fatal("a child removed the parent's home")
	}
	cleanup()
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("the parent's home was not removed")
	}
}

func TestForgeStubsAreReusedByChildProcesses(t *testing.T) {
	t.Setenv(forgeStubsEnv, "")
	t.Setenv("PATH", os.Getenv("PATH"))
	cleanup := PrependFailingForgeCLIs()
	dir := os.Getenv(forgeStubsEnv)
	if dir == "" {
		t.Fatal("parent did not record its stub folder")
	}
	childCleanup := PrependFailingForgeCLIs()
	childCleanup()
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("a child removed the parent's stubs")
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("the parent's stubs were not removed")
	}
}
