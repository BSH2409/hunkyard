package preferences

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/config"
)

func TestParseGitInvocationHeredocMessage(t *testing.T) {
	command := "git commit -m \"$(cat <<'EOF'\nRefs #12: fix the guard\n\nBody line.\nEOF\n)\""
	action, ok := parseGitInvocation(command)
	if !ok || action.Message != "Refs #12: fix the guard\n\nBody line." {
		t.Fatalf("heredoc message not extracted: %q %v", action.Message, ok)
	}
	if action, _ := parseGitInvocation(`git commit -m "$(generate-message)"`); action.Message != "" {
		t.Fatalf("unknown substitution must skip format checks: %q", action.Message)
	}
}

func TestParseGitInvocationShellWrappers(t *testing.T) {
	for _, command := range []string{`sh -c 'git commit -m x'`, `bash -lc "git commit -m x"`, `env git commit -m x`, `/usr/bin/env FOO=1 git commit -m x`} {
		if action, ok := parseGitInvocation(command); !ok || action.Verb != "commit" {
			t.Errorf("wrapped commit missed: %q", command)
		}
	}
	if text := shellCommandText([]any{"bash", "-l", "-c", "git commit -m x"}); !strings.HasPrefix(text, "git commit") {
		t.Fatalf("array with separate -c not unwrapped: %q", text)
	}
}

func TestAgentCannotRemoveGuardsOrRetireRules(t *testing.T) {
	s, _ := guardedStore(t, "no_unrequested_commit")
	for _, command := range []string{"hunkyard preferences 'allow' rule_x --once", "hunkyard preferences guard remove g_123", "hunkyard preferences retire rule_x --json"} {
		if blocked, _ := denied(preToolUse(t, s, "s1", t.TempDir(), command)); !blocked {
			t.Errorf("agent ran %q", command)
		}
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", t.TempDir(), "hunkyard preferences guard add --kind no_coauthor_trailer --rule rule_x --json")); blocked {
		t.Fatal("guard add must stay available to agents")
	}
}

func TestNegatedCommitIntentNotRecorded(t *testing.T) {
	for _, prompt := range []string{"don't commit yet, just fix the test", "never push to main directly please", "the last commit broke the tests, look"} {
		s := testStore(t)
		promptContext(t, s, "s1", prompt)
		if s.loadSession("s1").CommitRequested {
			t.Errorf("%q granted commit intent", prompt)
		}
	}
	for _, prompt := range []string{"commit and push this", "please commit the fix", "now push it to the branch"} {
		s := testStore(t)
		promptContext(t, s, "s1", prompt)
		if !s.loadSession("s1").CommitRequested {
			t.Errorf("%q did not grant commit intent", prompt)
		}
	}
}

func TestAutomaticMaintenanceSkipsWhenAlreadyCompleteToday(t *testing.T) {
	s := storeWithPending(t)
	calls := 0
	if result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, validProposalRunner(t, s, &calls)); err != nil || result["status"] != "success" {
		t.Fatalf("first run: %v %v", result, err)
	}
	input := captureInput()
	input.Feedback, input.TurnID = "no, stop adding helpers again", "later"
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, validProposalRunner(t, s, &calls))
	if err != nil || result["status"] != "already_complete" || calls != 1 {
		t.Fatalf("model ran again after today's success: %v %v calls=%d", result, err, calls)
	}
}

func TestAutomaticMaintenanceRejectsUnknownHost(t *testing.T) {
	s := storeWithPending(t)
	runner := func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		t.Fatal("runner called for an unknown host")
		return nil, nil
	}
	result, _ := s.runAutomaticMaintenance(context.Background(), config.PreferencesMaintenance{Host: "claude-code"}, false, runner)
	if result["status"] != "failed" || result["error_class"] != "invalid_config" {
		t.Fatalf("unknown host: %v", result)
	}
}

func TestMaintenanceChildEnvironmentAndDirectory(t *testing.T) {
	env := maintenanceChildEnv([]string{"CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=x", "CODEX_THREAD_ID=y", "CODEX_HOME=/c", "CLAUDE_CONFIG_DIR=/d", "PATH=/bin"})
	joined := strings.Join(env, " ")
	if strings.Contains(joined, "CLAUDECODE") || strings.Contains(joined, "CLAUDE_CODE_SESSION_ID") || strings.Contains(joined, "CODEX_THREAD_ID") {
		t.Fatalf("host session variables leaked: %v", env)
	}
	if !strings.Contains(joined, "CODEX_HOME=/c") || !strings.Contains(joined, "CLAUDE_CONFIG_DIR=/d") || !strings.Contains(joined, "LEARN_PREFERENCES_MAINTENANCE=1") {
		t.Fatalf("needed variables dropped: %v", env)
	}
	dir := t.TempDir()
	out, err := execMaintenanceRunner(context.Background(), []string{"sh", "-c", "pwd"}, nil, dir, nil)
	resolved, _ := filepath.EvalSymlinks(dir)
	if err != nil || strings.TrimSpace(string(out)) != resolved {
		t.Fatalf("child not run in store directory: %q %v", out, err)
	}
}

func TestStopIgnoresLinesPresentAtTurnStart(t *testing.T) {
	s, repo := stopRepo(t)
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "update the readme wording for the service"})
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("docs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "add", "-N", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if response := stopResponse(t, s, repo, false); response["decision"] == "block" {
		t.Fatalf("blocked on the user's own earlier line: %v", response["reason"])
	}
	if err := os.WriteFile(filepath.Join(repo, "app.rb"), []byte("x\ndef a; 1; end\ndef b; 2; end\n"), 0600); err != nil {
		t.Fatal(err)
	}
	response := stopResponse(t, s, repo, false)
	reason := textField(response["reason"])
	if response["decision"] != "block" || !strings.Contains(reason, "app.rb:3") || strings.Contains(reason, "app.rb:2") {
		t.Fatalf("new violation not isolated: %q", reason)
	}
}

func TestGuardOnlyAppliesInItsRulesRepository(t *testing.T) {
	s := testStore(t)
	repo := guardRepo(t)
	key, err := repositoryKey(repo)
	if err != nil || key == "" {
		t.Skip("repository identity unavailable")
	}
	input := CaptureInput{Feedback: "only commit when I ask", Directive: "Do not commit unless asked in this repository.", Scope: "repository", RepoKey: "orion-a1b2c3d4e5f6", Surfaces: []string{"devops"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "seed"}
	result, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addGuard("no_unrequested_commit", result.RuleID, nil); err != nil {
		t.Fatal(err)
	}
	stage(t, repo, "a.go")
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m x`)); blocked {
		t.Fatal("another repository's rule blocked this commit")
	}
}

func TestStopChecksNewUntrackedFiles(t *testing.T) {
	s, repo := stopRepo(t)
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "add the new service file please"})
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if err := os.MkdirAll(filepath.Join(repo, "lib"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "lib", "service.rb"), []byte("class Service\n  def call; run; end\nend\n"), 0600); err != nil {
		t.Fatal(err)
	}
	response := stopResponse(t, s, repo, false)
	if response["decision"] != "block" || !strings.Contains(textField(response["reason"]), "lib/service.rb:2") {
		t.Fatalf("new untracked file not checked: %v", response["reason"])
	}
}
