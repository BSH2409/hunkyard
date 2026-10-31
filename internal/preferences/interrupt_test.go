package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexInterruptMarksNextPromptAsCorrection(t *testing.T) {
	s := testStore(t)
	s.hostKind = "codex"
	data, _ := json.Marshal(map[string]any{"hook_event_name": "Interrupt", "session_id": "s1", "cwd": t.TempDir()})
	var out bytes.Buffer
	if code := s.hook("Interrupt", bytes.NewReader(data), &out, &bytes.Buffer{}); code != 0 || !json.Valid(out.Bytes()) {
		t.Fatalf("interrupt hook: %d %q", code, out.String())
	}
	promptContext(t, s, "s1", "use the existing client for the payment service here")
	observations := pendingObservations(t, s)
	if len(observations) != 1 || !observations[0].Signals.Correction {
		t.Fatalf("prompt after interrupt not captured as a correction: %+v", observations)
	}
	events, _ := s.Activity(10)
	found := false
	for _, e := range events {
		found = found || e.Kind == activityInterrupted
	}
	if !found {
		t.Fatal("interrupt not in activity")
	}
}

func TestInterruptClearsAfterOnePrompt(t *testing.T) {
	s := testStore(t)
	if err := s.updateSession("s1", func(state *sessionState) { state.InterruptedAt = "2026-08-19T12:00:00Z" }); err != nil {
		t.Fatal(err)
	}
	promptContext(t, s, "s1", "use the existing client for the payment service here")
	promptContext(t, s, "s1", "now update the readme for the payment service")
	if got := len(pendingObservations(t, s)); got != 1 {
		t.Fatalf("interrupt applied to more than one prompt: %d observations", got)
	}
}

func TestClaudeTranscriptInterruptDetected(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	transcript := filepath.Join(home, "projects", "repo", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0700); err != nil {
		t.Fatal(err)
	}
	lines := `{"type":"user","sessionId":"s1","message":{"role":"user","content":"refactor the payment service"}}
{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Creating a helper module"}]}}
{"type":"user","sessionId":"s1","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}
`
	if err := os.WriteFile(transcript, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": "keep it inside the existing service class", "transcript_path": transcript})
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	observations := pendingObservations(t, s)
	if len(observations) != 1 || !observations[0].Signals.Correction {
		t.Fatalf("prompt after Claude interrupt not captured: %+v", observations)
	}
}
