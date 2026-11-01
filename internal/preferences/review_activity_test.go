package preferences

import (
	"bytes"
	"encoding/json"
	"testing"
)

func recordReviewActivity(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	previous := reviewActivity
	reviewActivity = func(hostKind, sessionID, state string) {
		calls = append(calls, hostKind+":"+sessionID+":"+state)
	}
	t.Cleanup(func() { reviewActivity = previous })
	return &calls
}

func runHook(s *Store, event string, payload map[string]any) {
	payload["hook_event_name"] = event
	data, _ := json.Marshal(payload)
	s.hook(event, bytes.NewReader(data), &bytes.Buffer{}, &bytes.Buffer{})
}

func TestHooksReportAgentTurnsToOwnedReviews(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	calls := recordReviewActivity(t)
	runHook(s, "UserPromptSubmit", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "prompt": "fix the parser"})
	runHook(s, "PreToolUse", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "tool_name": "Read", "tool_input": map[string]any{}})
	runHook(s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir()})
	want := []string{"claude:s1:turn", "claude:s1:turn", "claude:s1:idle"}
	if len(*calls) != len(want) {
		t.Fatalf("activity reports = %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("activity reports = %v, want %v", *calls, want)
		}
	}
}

func TestHookStillWorksWithReviewActivityReported(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	recordReviewActivity(t)
	captureRule(t, s, "Prefer clear names for services.", "naming")
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": "rename this service with better names"})
	var out bytes.Buffer
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &out, &bytes.Buffer{})
	if !bytes.Contains(out.Bytes(), []byte("Prefer clear names")) {
		t.Fatalf("the prompt hook lost its payload after reporting activity: %s", out.String())
	}
}
