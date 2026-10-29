package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestHostSkillInstallIncludesLocalProtocol(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Chdir(home)
	sentinel := filepath.Join(home, ".claude", "settings.json")
	testutil.WriteFile(t, sentinel, `{"unrelated":"preserve"}`)
	defer func() {
		content, err := os.ReadFile(sentinel)
		if err != nil || string(content) != `{"unrelated":"preserve"}` {
			t.Error("skill installation changed unrelated host settings")
		}
	}()
	for _, host := range []struct{ name, dir string }{{"codex", ".agents"}, {"claude-code", ".claude"}} {
		if err := installIntegration(host.name, false); err != nil {
			t.Fatal(err)
		}
		entry := filepath.Join(home, host.dir, "skills", "hunkyard", "SKILL.md")
		content, err := os.ReadFile(entry)
		if err != nil {
			t.Fatal(err)
		}
		reference := filepath.Join(filepath.Dir(entry), "references", "review-workflow.md")
		protocol, err := os.ReadFile(reference)
		if err != nil {
			t.Fatalf("%s adapter cannot find shared protocol: %v", host.name, err)
		}
		canonical, err := integrationsFS.ReadFile("integrations/shared/review-workflow.md")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(protocol, canonical) {
			t.Fatal("installed host protocol differs")
		}
		if !strings.Contains(string(content), "references/review-workflow.md") {
			t.Fatal("host entrypoint does not route to its protocol")
		}
		if host.name == "claude-code" && !strings.Contains(string(content), "claude:${CLAUDE_SESSION_ID}") {
			t.Fatal("Claude adapter has no stable launching session identity")
		}
	}
}

func TestHostGlobalInstallFlag(t *testing.T) {
	target, force, global := parseInstallArgs([]string{"--global", "--force", "codex"})
	if target != "codex" || !force || !global {
		t.Fatal("global installation flags did not reach installer")
	}
}
