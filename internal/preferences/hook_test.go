package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookBoundaryReferenceCases(t *testing.T) {
	for _, name := range []string{"test_hook_omits_additional_context_when_selection_has_no_rules", "test_hook_unreadable_generation_fails_open_without_prompt_in_diagnostic", "test_hook_capture_lock_failure_preserves_selected_context_and_fails_open", "test_hook_ordinary_prompt_injects_without_capture_and_correction_captures_once", "test_hook_secret_shaped_correction_is_not_captured", "test_hook_adapter_never_reads_transcript_path_and_disk_errors_are_advisory"} {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			prompt := "Again, always prefer meaningful domain names"
			payload := map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "fixture-session", "turn_id": "fixture-turn", "cwd": t.TempDir(), "prompt": prompt, "transcript_path": "/must/not/be/read.jsonl"}
			if name != "test_hook_omits_additional_context_when_selection_has_no_rules" {
				input := captureInput()
				input.Explicit = true
				input.Scope = "global"
				input.RepoKey = ""
				input.Directive = "Use naming-analyzer when choosing names."
				input.Surfaces = []string{"naming"}
				if _, err := s.Capture(input); err != nil {
					t.Fatal(err)
				}
			}
			before := s.pendingCount()
			hasContext := true
			diagnostic := ""
			switch name {
			case "test_hook_omits_additional_context_when_selection_has_no_rules":
				payload["prompt"] = "Explain this text"
				hasContext = false
			case "test_hook_unreadable_generation_fails_open_without_prompt_in_diagnostic":
				current, err := s.loadCurrent()
				if err != nil {
					t.Fatal(err)
				}
				path := s.dataPath("generations", current.ID, "manifest.json")
				if err := os.Chmod(path, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("not JSON"), 0600); err != nil {
					t.Fatal(err)
				}
				hasContext = false
				diagnostic = "selection_failed"
			case "test_hook_capture_lock_failure_preserves_selected_context_and_fails_open":
				file, err := os.OpenFile(s.dataPath("observations", ".pending.lock"), os.O_RDWR, 0600)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err := lockFile(file, false); err != nil {
					t.Fatal(err)
				}
				defer unlockFile(file)
				diagnostic = "capture_failed"
			case "test_hook_ordinary_prompt_injects_without_capture_and_correction_captures_once":
				payload["prompt"] = "continue with the naming changes"
			case "test_hook_secret_shaped_correction_is_not_captured":
				payload["prompt"] = "Always use password=synthetic-value"
			case "test_hook_adapter_never_reads_transcript_path_and_disk_errors_are_advisory":
				if err := os.Remove(s.dataPath("observations", "pending.jsonl")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(s.dataPath("observations", "pending.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
				diagnostic = "capture_failed"
			}
			run := func() map[string]any {
				t.Helper()
				data, _ := json.Marshal(payload)
				var out, errOut bytes.Buffer
				s.hook("UserPromptSubmit", bytes.NewReader(data), &out, &errOut)
				var result map[string]any
				if json.Unmarshal(out.Bytes(), &result) != nil || result["continue"] != true {
					t.Fatal("hook blocked")
				}
				if (result["hookSpecificOutput"] != nil) != hasContext {
					t.Fatal("hook context differs")
				}
				if strings.Contains(errOut.String(), textField(payload["prompt"])) || strings.Contains(errOut.String(), "/must/not/be/read") {
					t.Fatal("diagnostic leaked input")
				}
				if diagnostic != "" && !strings.Contains(errOut.String(), diagnostic) {
					t.Fatal("advisory diagnostic missing")
				}
				return result
			}
			run()
			if name == "test_hook_ordinary_prompt_injects_without_capture_and_correction_captures_once" {
				if s.pendingCount() != before {
					t.Fatal("ordinary prompt captured")
				}
				payload["prompt"] = prompt
				run()
				run()
				if s.pendingCount() != before+1 {
					t.Fatal("correction replay inflated evidence")
				}
			} else if name != "test_hook_adapter_never_reads_transcript_path_and_disk_errors_are_advisory" && s.pendingCount() != before {
				t.Fatal("failed or noncapturable hook changed inbox")
			}
		})
	}
}

func TestHookHostPayloadsAndAdvisoryFailures(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.Explicit = true
	input.Scope = "global"
	input.RepoKey = ""
	input.Directive = "Use naming-analyzer when choosing domain names."
	input.Surfaces = []string{"naming"}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"codex", "claude"} {
		t.Run(host, func(t *testing.T) {
			payload := map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": host + "-session", "cwd": t.TempDir(), "prompt": "Always prefer clear domain names"}
			if host == "codex" {
				payload["turn_id"] = "turn-1"
			}
			data, _ := json.Marshal(payload)
			var out, errOut bytes.Buffer
			s.hook("UserPromptSubmit", bytes.NewReader(data), &out, &errOut)
			var result map[string]any
			if json.Unmarshal(out.Bytes(), &result) != nil || result["continue"] != true {
				t.Fatal("hook blocked session")
			}
			context := record(result["hookSpecificOutput"])
			if context["hookEventName"] != "UserPromptSubmit" || !strings.Contains(textField(context["additionalContext"]), "naming-analyzer") {
				t.Fatal("hook omitted naming context")
			}
		})
	}
	for _, payload := range []string{"{", strings.Repeat("x", 1024*1024+1), `{"hook_event_name":"Other","prompt":"Prefer safe names"}`} {
		var out, errOut bytes.Buffer
		s.hook("UserPromptSubmit", strings.NewReader(payload), &out, &errOut)
		if strings.TrimSpace(out.String()) != `{"continue":true}` || len(payload) > 10 && strings.Contains(errOut.String(), payload) {
			t.Fatal("malformed payload blocked session or leaked input")
		}
	}
	t.Setenv("LEARN_PREFERENCES_MAINTENANCE", "1")
	var out, errOut bytes.Buffer
	s.hook("UserPromptSubmit", strings.NewReader("not read"), &out, &errOut)
	if strings.TrimSpace(out.String()) != `{"continue":true}` || errOut.Len() != 0 {
		t.Fatal("maintenance recursion was not suppressed")
	}
}

func TestHookTranscriptSessionConfinement(t *testing.T) {
	s := testStore(t)
	cwd := t.TempDir()
	root := filepath.Join(cwd, ".claude")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, "fixture.jsonl")
	transcript := strings.Join([]string{
		`{"sessionId":"synthetic-session","type":"user","message":{"role":"user","content":"Refactor the parser"}}`,
		`{"sessionId":"other-session","type":"assistant","message":{"role":"assistant","content":"Foreign session prose"}}`,
		`{"sessionId":"synthetic-session","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I introduced a one-use helper."}]}}`,
		`{"sessionId":"synthetic-session","isMeta":true,"type":"user","message":{"role":"user","content":"Metadata should be ignored"}}`,
		`{"sessionId":"synthetic-session","type":"user","message":{"role":"user","content":"No, inline that helper instead"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(name, []byte(transcript), 0600); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"session_id": "synthetic-session", "cwd": cwd, "prompt": "No, inline that helper instead", "transcript_path": name}
	context := s.transcriptContext(payload)
	if context.PreviousUserRequest != "Refactor the parser" || context.AgentResponse != "I introduced a one-use helper." {
		t.Fatalf("neighboring context differs: %#v", context)
	}
	payload["session_id"] = "wrong-session"
	if context := s.transcriptContext(payload); context.Source != "" {
		t.Fatal("foreign session transcript accepted")
	}
	payload["session_id"] = "synthetic-session"
	link := filepath.Join(root, "linked.jsonl")
	if err := os.Symlink(name, link); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	payload["transcript_path"] = link
	if context := s.transcriptContext(payload); context.Source != "" {
		t.Fatal("symlink transcript accepted")
	}
}

func TestHookUnknownEventContinues(t *testing.T) {
	s := testStore(t)
	var out, errOut bytes.Buffer
	s.hook("Notification", strings.NewReader(`{"hook_event_name":"Notification"}`), &out, &errOut)
	var response map[string]any
	if json.Unmarshal(out.Bytes(), &response) != nil || response["continue"] != true {
		t.Fatalf("unknown events must continue: %q", out.String())
	}
}
