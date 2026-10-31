package preferences

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func coreAndSurfaceRules() []rule {
	global := map[string]any{"kind": "global", "repo_key": nil}
	core := newRule("Apply naming-analyzer when writing code.", global, []string{"naming"}, "feedback", fixedNow)
	core["status"] = "core"
	surface := newRule("Use only info and error log levels.", global, []string{"ruby"}, "feedback", fixedNow)
	surface["status"] = "active"
	otherRepo := newRule("Keep vulnerability ingestion errors namespaced.", map[string]any{"kind": "repository", "repo_key": "orion-a1b2c3d4e5f6"}, []string{"ruby"}, "feedback", fixedNow)
	otherRepo["status"] = "core"
	return []rule{core, surface, otherRepo}
}

func TestSelectCoreOnlyCoreRules(t *testing.T) {
	selection := selectCore(coreAndSurfaceRules(), "blogvault-a1b2c3d4e5f6")
	if !strings.Contains(selection.Text, "naming-analyzer") {
		t.Fatalf("core rule missing: %s", selection.Text)
	}
	if strings.Contains(selection.Text, "log levels") || strings.Contains(selection.Text, "vulnerability") {
		t.Fatalf("non-core or out-of-scope rule injected: %s", selection.Text)
	}
	if len(selection.RuleIDs) != 1 {
		t.Fatalf("rule ids: %v", selection.RuleIDs)
	}
}

func sessionStartResponse(t *testing.T, s *Store, cwd string) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "SessionStart", "session_id": "s1", "cwd": cwd, "source": "startup"})
	var out bytes.Buffer
	s.hook("SessionStart", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("invalid response %q", out.String())
	}
	return response
}

func TestSessionStartInjectsCoreAndHealth(t *testing.T) {
	s := testStore(t)
	captureCoreRule(t, s)
	for i, text := range []string{"no, never use warn again", "don't add helpers again", "stop adding comments"} {
		input := captureInput()
		input.Feedback, input.TurnID = text, "pending-"+string(rune('a'+i))
		if _, err := s.Capture(input); err != nil {
			t.Fatal(err)
		}
	}
	response := sessionStartResponse(t, s, t.TempDir())
	context := textField(record(response["hookSpecificOutput"])["additionalContext"])
	if !strings.Contains(context, "naming-analyzer") || !strings.Contains(context, "3 observations pending") {
		t.Fatalf("core rule or health line missing: %q", context)
	}
	if ids := s.loadSession("s1").InjectedRuleIDs; len(ids) != 1 {
		t.Fatalf("injected ids not recorded: %v", ids)
	}
}

func TestSessionStartNoHealthWhenClean(t *testing.T) {
	s := testStore(t)
	captureCoreRule(t, s)
	context := textField(record(sessionStartResponse(t, s, t.TempDir())["hookSpecificOutput"])["additionalContext"])
	if strings.Contains(context, "pending") {
		t.Fatalf("health line without pending work: %q", context)
	}
}

var fixedNow = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

func captureCoreRule(t *testing.T, s *Store) {
	t.Helper()
	for i := range 5 {
		input := CaptureInput{Feedback: "always run naming-analyzer", Directive: "Apply naming-analyzer when writing code.", Scope: "global", Surfaces: []string{"naming"}, Explicit: true, Source: "cli", SessionID: "session-" + string(rune('a'+i%3)), TurnID: "turn-" + string(rune('a'+i))}
		if _, err := s.Capture(input); err != nil {
			t.Fatal(err)
		}
	}
}
