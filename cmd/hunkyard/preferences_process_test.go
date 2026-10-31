package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPreferencesBinaryWithoutRubyOrProviderCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("compiled preference CLI")
	}
	if runtime.GOOS == "windows" {
		t.Skip("native Windows hook installation requires separate acceptance")
	}
	binary := filepath.Join(t.TempDir(), "hunkyard")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	home := t.TempDir()
	root := filepath.Join(home, ".agents")
	env := []string{"HOME=" + home, "USERPROFILE=" + home, "LEARN_PREFERENCES_HOME=" + root, "CODEX_HOME=" + filepath.Join(home, ".codex"), "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"), "PATH=" + filepath.Join(home, "empty-path")}
	run := func(input string, args ...string) map[string]any {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir = home
		command.Env = env
		command.Stdin = strings.NewReader(input)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", strings.Join(args, " "), err, output)
		}
		var result map[string]any
		if json.Unmarshal(output, &result) != nil {
			t.Fatalf("non-JSON output from %s", args[0])
		}
		return result
	}
	if result := run("", "preferences", "status", "--json"); result["status"] != "no_generation" {
		t.Fatal(result)
	}
	if result := run("Always use descriptive domain names", "preferences", "capture", "--explicit", "--directive", "Use naming-analyzer when choosing domain names.", "--scope", "global", "--surface", "naming", "--json"); result["action"] != "activated" {
		t.Fatal(result)
	}
	if result := run("", "preferences", "select", "--query", "continue", "--json"); !strings.Contains(result["text"].(string), "naming-analyzer") {
		t.Fatal("compiled selector omitted naming")
	}
	for _, command := range []string{"build", "audit", "search"} {
		run("", "preferences", command, "--json")
	}
	for _, host := range []string{"codex", "claude"} {
		result := run("", "preferences", "install-hook", "--host", host, "--json")
		if result["installed"] != true || result["trust_required"] != true {
			t.Fatal("compiled hook installer failed")
		}
	}
	for _, settings := range []string{filepath.Join(home, ".codex", "hooks.json"), filepath.Join(home, ".claude", "settings.json")} {
		if _, err := os.Stat(settings); err != nil {
			t.Fatal("hook not written to host configuration", err)
		}
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "fixture-host", "cwd": home, "prompt": "continue with the naming changes"})
	result := run(string(payload), "preferences", "hook")
	if result["continue"] != true || result["hookSpecificOutput"] == nil {
		t.Fatal("compiled hook did not deliver persona")
	}
	for _, event := range []string{"SessionStart", "PreToolUse", "Stop"} {
		eventPayload, _ := json.Marshal(map[string]any{"hook_event_name": event, "session_id": "fixture-host", "cwd": home, "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
		if result := run(string(eventPayload), "preferences", "hook", "--event", event); result["continue"] != true {
			t.Fatalf("%s hook did not continue: %v", event, result)
		}
	}
	source := filepath.Join(root, "learn-preferences", "sources", "code-like-him")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# Fixture\n## References\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result = run("", "preferences", "maintain", "--dry-run", "--json")
	if result["status"] != "needs_proposal" || result["batch"] == nil {
		t.Fatal("compiled maintenance did not return local input")
	}
	command := exec.Command(binary, "install", "--global", "preferences")
	command.Dir = home
	command.Env = env
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("skill install: %v %s", err, output)
	}
	for _, path := range []string{filepath.Join(root, "skills", "learn-preferences", "SKILL.md"), filepath.Join(home, ".claude", "skills", "learn-preferences", "SKILL.md")} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "hunkyard preferences") {
			t.Fatal("Go preference skill missing", err)
		}
	}
}
