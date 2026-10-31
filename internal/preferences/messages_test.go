package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickMessageDeterministic(t *testing.T) {
	first := pickMessage(spinnerVerbs, "s1:t1")
	if again := pickMessage(spinnerVerbs, "s1:t1"); again != first {
		t.Fatal("same seed picked different messages")
	}
	seen := map[string]bool{}
	for _, seed := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[pickMessage(afterHookNotes, seed)] = true
	}
	if len(seen) < 2 {
		t.Fatal("pool never varies")
	}
	if len(spinnerVerbs) != 9 || len(afterHookNotes) != 4 || !strings.Contains(afterHookNotes[0], "{n}") {
		t.Fatal("pools must match the approved copy")
	}
}

func claudeSettings(t *testing.T, s *Store, initial string) map[string]any {
	t.Helper()
	dir := filepath.Join(s.config.Root, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if initial != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(initial), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if _, err := s.installHook("claude"); err != nil {
			t.Fatal(err)
		}
	}
	return readJSONFile(filepath.Join(dir, "settings.json"))
}

func TestInstallMergesSpinnerSettingsWithoutClobbering(t *testing.T) {
	s := testStore(t)
	settings := claudeSettings(t, s, `{"spinnerVerbs":{"mode":"append","verbs":["Thinking"]}}`)
	verbs := stringValues(record(settings["spinnerVerbs"])["verbs"])
	if verbs[0] != "Thinking" || len(verbs) != 1+len(spinnerVerbs) {
		t.Fatalf("verbs clobbered or duplicated: %v", verbs)
	}
	tips := stringValues(record(settings["spinnerTipsOverride"])["tips"])
	if len(tips) != len(spinnerTips) {
		t.Fatalf("tips: %v", tips)
	}
}

func TestInstallKeepsExistingStatusLine(t *testing.T) {
	s := testStore(t)
	settings := claudeSettings(t, s, `{"statusLine":{"type":"command","command":"mine"}}`)
	if record(settings["statusLine"])["command"] != "mine" {
		t.Fatalf("status line replaced: %v", settings["statusLine"])
	}
	fresh := claudeSettings(t, testStore(t), "")
	if !strings.Contains(textField(record(fresh["statusLine"])["command"]), "preferences statusline") {
		t.Fatalf("status line not offered: %v", fresh["statusLine"])
	}
}

func TestCodexInstallAddsNoSpinnerSettings(t *testing.T) {
	s := testStore(t)
	result, err := s.installHook("codex")
	if err != nil {
		t.Fatal(err)
	}
	settings := readJSONFile(textField(result["path"]))
	if settings["spinnerVerbs"] != nil || settings["statusLine"] != nil {
		t.Fatal("codex config got Claude-only settings")
	}
}

func TestStatuslineOutput(t *testing.T) {
	s := storeWithPending(t)
	captureRule(t, s, "Prefer clear names.", "naming")
	var out bytes.Buffer
	if exit := Run([]string{"statusline", "--root", s.config.Root}, strings.NewReader(""), &out, &bytes.Buffer{}); exit != 0 {
		t.Fatalf("exit %d", exit)
	}
	line := strings.TrimSpace(out.String())
	if !strings.HasPrefix(line, "◆ yard:") || !strings.Contains(line, "1 pending") || len([]rune(line)) > 80 {
		t.Fatalf("status line %q", line)
	}
}

func TestPromptHookAddsAfterHookNote(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Prefer clear names for services.", "naming")
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": "rename this service with better names"})
	var out bytes.Buffer
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	_ = json.Unmarshal(out.Bytes(), &response)
	note := textField(response["systemMessage"])
	if note == "" || strings.Contains(note, "{n}") {
		t.Fatalf("after-hook note missing or unfilled: %q", note)
	}
}

func TestPromptHookAnnouncesCapture(t *testing.T) {
	s := testStore(t)
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": "no, never add single line helpers again"})
	var out bytes.Buffer
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	_ = json.Unmarshal(out.Bytes(), &response)
	note := textField(response["systemMessage"])
	found := false
	for _, message := range capturingMessages {
		found = found || note == message
	}
	if !found {
		t.Fatalf("capture not announced: %q", note)
	}
}
