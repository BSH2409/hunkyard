package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func guardRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func stage(t *testing.T, repo, path string) {
	t.Helper()
	full := filepath.Join(repo, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "add", path).CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
}

func guardedStore(t *testing.T, kind string, patterns ...string) (*Store, string) {
	t.Helper()
	s := testStore(t)
	captureRule(t, s, "Keep plans and specs out of git.", "devops")
	current, _ := s.loadCurrent()
	ruleID := textField(current.Rules.rules[0]["id"])
	if _, err := s.addGuard(kind, ruleID, patterns); err != nil {
		t.Fatal(err)
	}
	return s, ruleID
}

func preToolUse(t *testing.T, s *Store, session, cwd string, command any) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "session_id": session, "cwd": cwd, "tool_name": "Bash", "tool_input": map[string]any{"command": command}})
	var out bytes.Buffer
	s.hook("PreToolUse", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("invalid response %q", out.String())
	}
	return response
}

func denied(response map[string]any) (bool, string) {
	output := record(response["hookSpecificOutput"])
	return output["permissionDecision"] == "deny", textField(output["permissionDecisionReason"])
}

func TestGuardBlocksStagedPlanFile(t *testing.T) {
	s, ruleID := guardedStore(t, "staged_paths", "docs/superpowers/**")
	repo := guardRepo(t)
	stage(t, repo, "docs/superpowers/plans/a.md")
	blocked, reason := denied(preToolUse(t, s, "s1", repo, `git commit -m "add plan"`))
	if !blocked || !strings.Contains(reason, "docs/superpowers/plans/a.md") || !strings.Contains(reason, ruleID) || !strings.Contains(reason, "preferences allow") {
		t.Fatalf("plan commit not blocked with a reason: %v %q", blocked, reason)
	}
	events, _ := s.Activity(1)
	if len(events) != 1 || events[0].Kind != activityBlocked {
		t.Fatalf("block not recorded: %+v", events)
	}
	stage(t, repo, "app/code.go")
	if out, err := exec.Command("git", "-C", repo, "reset", "-q", "docs").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "code"`)); blocked {
		t.Fatal("code-only commit blocked")
	}
}

func TestPreToolUseArrayCommand(t *testing.T) {
	s, _ := guardedStore(t, "staged_paths", "*-plan.md")
	repo := guardRepo(t)
	stage(t, repo, "release-plan.md")
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, []string{"bash", "-lc", "git commit -m x"})); !blocked {
		t.Fatal("array-form command not evaluated")
	}
}

func TestGuardBlocksCoauthorTrailer(t *testing.T) {
	s, _ := guardedStore(t, "no_coauthor_trailer")
	repo := guardRepo(t)
	stage(t, repo, "a.go")
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, "git commit -m \"fix\n\nCo-Authored-By: Bot <b@x>\"")); !blocked {
		t.Fatal("co-author trailer not blocked")
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "fix"`)); blocked {
		t.Fatal("plain commit blocked")
	}
}

func TestGuardBlocksBadCommitFormat(t *testing.T) {
	s, _ := guardedStore(t, "commit_message_format", `^Refs #\d+: `)
	repo := guardRepo(t)
	stage(t, repo, "a.go")
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "fix it"`)); !blocked {
		t.Fatal("bad format not blocked")
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "Refs #12: fix it"`)); blocked {
		t.Fatal("good format blocked")
	}
}

func TestGuardBlocksUnrequestedCommitThenAllowsAfterRequest(t *testing.T) {
	s, _ := guardedStore(t, "no_unrequested_commit")
	repo := guardRepo(t)
	stage(t, repo, "a.go")
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "x"`)); !blocked {
		t.Fatal("unrequested commit allowed")
	}
	if err := s.updateSession("s1", func(state *sessionState) { state.CommitRequested = true }); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m "x"`)); blocked {
		t.Fatal("requested commit blocked")
	}
}

func TestAllowOnceConsumedOnce(t *testing.T) {
	s, ruleID := guardedStore(t, "staged_paths", "*-plan.md")
	repo := guardRepo(t)
	stage(t, repo, "release-plan.md")
	if _, err := s.allowOnce(ruleID); err != nil {
		t.Fatal(err)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m x`)); blocked {
		t.Fatal("override not honoured")
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", repo, `git commit -m x`)); !blocked {
		t.Fatal("override reused")
	}
	events, _ := s.Activity(10)
	kinds := []string{}
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	if !strings.Contains(strings.Join(kinds, ","), activityOverridden) {
		t.Fatalf("override not recorded: %v", kinds)
	}
}

func TestAgentCannotRunAllow(t *testing.T) {
	s, _ := guardedStore(t, "no_unrequested_commit")
	blocked, reason := denied(preToolUse(t, s, "s1", t.TempDir(), "hunkyard preferences allow rule_x --once"))
	if !blocked || !strings.Contains(reason, "Only the user") {
		t.Fatalf("agent granted its own override: %v %q", blocked, reason)
	}
}

func TestPreToolUseAllowsWhenStoreUnreadable(t *testing.T) {
	s, err := Open(Config{Root: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if blocked, _ := denied(preToolUse(t, s, "s1", t.TempDir(), `git commit -m x`)); blocked {
		t.Fatal("missing store blocked a commit")
	}
}

func TestGuardAndAllowCLI(t *testing.T) {
	s, ruleID := guardedStore(t, "no_unrequested_commit")
	run := func(args ...string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		if exit := Run(append(args, "--root", s.config.Root, "--json"), strings.NewReader(""), &out, &errOut); exit != 0 {
			t.Fatalf("%v exited %d: %s %s", args, exit, out.String(), errOut.String())
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("%v: invalid JSON %q", args, out.String())
		}
		return result
	}
	added := run("guard", "add", "--kind", "staged_paths", "--rule", ruleID, "--pattern", "docs/**", "--pattern", "*-plan.md")
	id := textField(record(added["guard"])["id"])
	if added["status"] != "success" || id == "" {
		t.Fatalf("guard add: %v", added)
	}
	listed := run("guard", "list")
	if guards, _ := listed["guards"].([]any); len(guards) != 2 {
		t.Fatalf("guard list: %v", listed)
	}
	if allowed := run("allow", ruleID, "--once"); allowed["status"] != "success" {
		t.Fatalf("allow: %v", allowed)
	}
	if removed := run("guard", "remove", id); removed["status"] != "success" {
		t.Fatalf("guard remove: %v", removed)
	}
}
