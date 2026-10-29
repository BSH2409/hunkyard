package main

import (
	"strings"
	"testing"
)

func readIntegrationForPolicyTest(t *testing.T, path string) string {
	t.Helper()
	data, err := integrationsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestInteractiveSkillsRequireExplicitHunkyardWording(t *testing.T) {
	paths := []string{
		"integrations/claude-code/skills/hunkyard/SKILL.md",
		"integrations/codex/skills/hunkyard/SKILL.md",
		"integrations/codex/plugin/hunkyard/skills/hunkyard/SKILL.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := readIntegrationForPolicyTest(t, path)
			if strings.Contains(content, "disable-model-invocation: true") {
				t.Fatalf("%s still uses hard disable-model-invocation; prefer explicit-Hunkyard wording", path)
			}
			if strings.Contains(content, "allow_implicit_invocation: false") {
				t.Fatalf("%s still uses hard allow_implicit_invocation:false; prefer explicit-Hunkyard wording", path)
			}
			if !strings.Contains(content, "generic review request") || !strings.Contains(content, "Hunkyard") {
				t.Fatal("local adapter lost its invocation boundary")
			}
		})
	}
}

func TestCodexNoLongerShipsImplicitInvocationPolicyFile(t *testing.T) {
	paths := []string{
		"integrations/codex/skills/hunkyard/agents/openai.yaml",
		"integrations/codex/plugin/hunkyard/skills/hunkyard/agents/openai.yaml",
	}
	for _, path := range paths {
		if _, err := integrationsFS.ReadFile(path); err == nil {
			t.Fatalf("%s should have been removed; use skill wording instead of policy YAML", path)
		}
	}
}

func TestHunkyardCLIStaysModelDiscoverableWithoutStartingInteractiveHunkyard(t *testing.T) {
	paths := []string{
		"integrations/claude-code/skills/hunkyard-cli/SKILL.md",
		"integrations/codex/skills/hunkyard-cli/SKILL.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := readIntegrationForPolicyTest(t, path)
			if !strings.Contains(content, "name: hunkyard-cli") {
				t.Fatalf("%s is not a hunkyard-cli skill", path)
			}
			if strings.Contains(content, "disable-model-invocation: true") {
				t.Fatalf("%s must remain model-discoverable", path)
			}
		})
	}
}

func TestHunkyardStorySkillsAreWordingGated(t *testing.T) {
	paths := []string{
		"integrations/claude-code/skills/hunkyard-story/SKILL.md",
		"integrations/codex/skills/hunkyard-story/SKILL.md",
		"integrations/codex/plugin/hunkyard/skills/hunkyard-story/SKILL.md",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := readIntegrationForPolicyTest(t, path)
			if !strings.Contains(content, "name: hunkyard-story") {
				t.Fatalf("%s is not a hunkyard-story skill", path)
			}
			if !strings.Contains(content, "Do not infer") {
				t.Fatalf("%s lacks wording-gate language", path)
			}
			if strings.Contains(content, "disable-model-invocation: true") {
				t.Fatalf("%s uses hard disable; prefer wording gate", path)
			}
		})
	}
}

func TestLocalHostsDoNotInterceptPlanExit(t *testing.T) {
	for _, path := range []string{"integrations/claude-code/hooks/hooks.json", "integrations/codex/plugin/hunkyard/hooks/hooks.json"} {
		content := readIntegrationForPolicyTest(t, path)
		if strings.Contains(content, "plan-hook") {
			t.Fatal("local host installed an implicit legacy review hook")
		}
	}
}
