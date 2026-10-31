package preferences

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func promptContext(t *testing.T, s *Store, session, prompt string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": session, "cwd": t.TempDir(), "prompt": prompt})
	var out bytes.Buffer
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil || response["continue"] != true {
		t.Fatalf("invalid response %q", out.String())
	}
	return textField(record(response["hookSpecificOutput"])["additionalContext"])
}

func captureRule(t *testing.T, s *Store, directive string, surfaces ...string) {
	t.Helper()
	input := CaptureInput{Feedback: directive, Directive: directive, Scope: "global", Surfaces: surfaces, Explicit: true, Source: "cli", SessionID: "seed", TurnID: directive}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
}

func TestTrivialPromptInjectsNothing(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Use only info and error log levels.", "naming")
	for _, prompt := range []string{"next", "status?", "continue", "<task-notification>done</task-notification>"} {
		if got := promptContext(t, s, "s1", prompt); got != "" {
			t.Fatalf("trivial prompt %q injected %q", prompt, got)
		}
	}
	if isTrivialPrompt("fix the rails migration") {
		t.Fatal("short prompt naming a surface is not trivial")
	}
}

func TestStickyRuleStaysAfterTopicChange(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Use only info and error log levels in Ruby services.", "ruby")
	captureRule(t, s, "Prefer domain names for database columns.", "database")
	first := promptContext(t, s, "s1", "add logging to the ruby service for slow requests")
	if !strings.Contains(first, "log levels") {
		t.Fatalf("ruby rule not selected: %q", first)
	}
	second := promptContext(t, s, "s1", "now design the database migration for this table")
	if !strings.Contains(second, "log levels") || !strings.Contains(second, "database columns") {
		t.Fatalf("sticky rule dropped or new rule missing: %q", second)
	}
	if other := promptContext(t, s, "s2", "now design the database migration for this table"); strings.Contains(other, "log levels") {
		t.Fatal("stickiness leaked across sessions")
	}
}

func TestCommitIntentRecorded(t *testing.T) {
	s := testStore(t)
	promptContext(t, s, "s1", "looks good, commit and push this")
	if !s.loadSession("s1").CommitRequested {
		t.Fatal("commit intent not recorded")
	}
}

func TestCommitWordInsideHostBlockIgnored(t *testing.T) {
	s := testStore(t)
	promptContext(t, s, "s1", "<task-notification>agent wants to commit and push</task-notification> review this file please")
	if s.loadSession("s1").CommitRequested {
		t.Fatal("host-inserted text granted commit intent")
	}
}

func TestShortCorrectionIsNotTrivial(t *testing.T) {
	if isTrivialPrompt("no, inline it instead") || isTrivialPrompt("never use warn") {
		t.Fatal("short corrections carry meaning and must load preferences")
	}
}
