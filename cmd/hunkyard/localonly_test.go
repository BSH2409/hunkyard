package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestLocalRuntimeDoesNotProbeForgeOrOtherAgents(t *testing.T) {
	if os.Getenv("HUNKYARD_LOCAL_TEST_CHILD") == "1" {
		localonly.Install()
		html := filepath.Join(t.TempDir(), "index.html")
		if err := os.WriteFile(html, []byte("<html><body>synthetic preview</body></html>"), 0600); err != nil {
			t.Fatal(err)
		}
		if routePositionalArgs([]string{html}) != positionalRouteReview {
			t.Fatal("HTML source was routed to executable preview")
		}
		if got := checkForgeCLIs(); len(got) != 0 {
			t.Fatal("local runtime probed forge authentication")
		}
		_ = detectInstalledIntegrations(t.TempDir(), t.TempDir())
		_ = detectPresentAgents(t.TempDir())
		scope := session.StoryScope{}
		fillStoryPRContext(&scope)
		if got := availableIntegrations(); !reflect.DeepEqual(got, []string{"claude-code", "codex"}) {
			t.Fatalf("supported integrations: %v", got)
		}
		for _, backend := range []string{"jj", "sl"} {
			if vcs.DetectVCS(backend) != nil {
				t.Fatal("unvalidated VCS backend is enabled", backend)
			}
		}
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".jj"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		if vcs.DetectVCS("") != nil {
			t.Fatal("unvalidated VCS backend was auto-detected")
		}
		return
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "executed")
	for _, name := range []string{"gh", "jj", "sl"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho called >> \"$HUNKYARD_PROBE_LOG\"\necho 2.0.0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLocalRuntimeDoesNotProbeForgeOrOtherAgents$", "-test.count=1")
	cmd.Env = append(os.Environ(), "HUNKYARD_LOCAL_TEST_CHILD=1", "HUNKYARD_PROBE_LOG="+marker, "PATH="+bin, "HOME="+root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("local probe test: %s: %v", output, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("runtime launched a potentially outbound probe")
	}
}
