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

func TestRubySingleLineDefCheck(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"def a; 1; end", true},
		{"  def name = @name", true},
		{"def empty; end", true},
		{"def call(x)", false},
		{"  default = 1", false},
	}
	for _, tt := range tests {
		if got := len(diffChecks["ruby_single_line_def"]("a.rb", []string{tt.line})) > 0; got != tt.want {
			t.Errorf("ruby_single_line_def(%q) = %v", tt.line, got)
		}
	}
	if len(diffChecks["ruby_single_line_def"]("a.go", []string{"def a; 1; end"})) != 0 {
		t.Fatal("ruby check ran on a Go file")
	}
}

func TestRubySemicolonStatementCheck(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"a = 1; b = 2", true},
		{`puts "a; b"`, false},
		{"x = 1 # one; two", false},
	}
	for _, tt := range tests {
		if got := len(diffChecks["ruby_semicolon_statement"]("a.rb", []string{tt.line})) > 0; got != tt.want {
			t.Errorf("ruby_semicolon_statement(%q) = %v", tt.line, got)
		}
	}
}

func stopRepo(t *testing.T) (*Store, string) {
	t.Helper()
	s, _ := guardedStore(t, "diff_check", "ruby_single_line_def")
	repo := guardRepo(t)
	stage(t, repo, "app.rb")
	if out, err := exec.Command("git", "-C", repo, "commit", "-qm", "init").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "app.rb"), []byte("x\ndef a; 1; end\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, repo
}

func stopResponse(t *testing.T, s *Store, repo string, active bool) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": "s1", "cwd": repo, "stop_hook_active": active})
	var out bytes.Buffer
	s.hook("Stop", bytes.NewReader(payload), &out, &bytes.Buffer{})
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("invalid response %q", out.String())
	}
	return response
}

func TestStopBlocksOnceThenAllows(t *testing.T) {
	s, repo := stopRepo(t)
	first := stopResponse(t, s, repo, false)
	if first["decision"] != "block" || !strings.Contains(textField(first["reason"]), "app.rb:2") {
		t.Fatalf("violation not blocked: %v", first)
	}
	if second := stopResponse(t, s, repo, false); second["decision"] == "block" {
		t.Fatal("blocked twice for the same changes")
	}
}

func TestStopSkipsWhenTurnChangedNothing(t *testing.T) {
	s, repo := stopRepo(t)
	if err := s.updateSession("s1", func(state *sessionState) { state.TurnDiffDigest = workingDiffDigest(repo) }); err != nil {
		t.Fatal(err)
	}
	if response := stopResponse(t, s, repo, false); response["decision"] == "block" {
		t.Fatal("blocked although this turn changed nothing")
	}
}

func TestStopHonoursStopHookActive(t *testing.T) {
	s, repo := stopRepo(t)
	if response := stopResponse(t, s, repo, true); response["decision"] == "block" {
		t.Fatal("blocked during a stop-hook continuation")
	}
}

func TestPromptRecordsTurnDiffDigest(t *testing.T) {
	s, repo := stopRepo(t)
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": repo, "prompt": "keep going with the service refactor"})
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if got := s.loadSession("s1").TurnDiffDigest; got != workingDiffDigest(repo) {
		t.Fatalf("turn start not fingerprinted: %q", got)
	}
	if response := stopResponse(t, s, repo, false); response["decision"] == "block" {
		t.Fatal("changes from before this turn were reviewed")
	}
}
