package preferences

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestUserAuthoredTextStripsHostBlocks(t *testing.T) {
	tests := []struct{ in, want string }{
		{"<task-notification><task-id>a</task-id><status>completed</status></task-notification>", ""},
		{"fix it\n<system-reminder>x</system-reminder>", "fix it"},
		{`<pasted_content id="1">never do X</pasted_content id="1"> use this`, "use this"},
		{"no, don't add helpers", "no, don't add helpers"},
	}
	for _, tt := range tests {
		if got := userAuthoredText(tt.in); got != tt.want {
			t.Errorf("userAuthoredText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHookSkipsCaptureForHostOnlyPrompt(t *testing.T) {
	s := testStore(t)
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": "<task-notification>stop doing that again, instead always retry</task-notification>"})
	var out, errOut bytes.Buffer
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &out, &errOut)
	if len(pendingObservations(t, s)) != 0 {
		t.Fatal("host-inserted text was captured as user feedback")
	}
	var response map[string]any
	if json.Unmarshal(out.Bytes(), &response) != nil || response["continue"] != true {
		t.Fatalf("hook must continue: %s", out.String())
	}
}
