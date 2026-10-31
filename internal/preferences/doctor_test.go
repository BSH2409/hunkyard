package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func doctorCheck(result map[string]any, name string) map[string]any {
	checks, _ := result["checks"].([]map[string]any)
	for _, check := range checks {
		if check["name"] == name {
			return check
		}
	}
	return nil
}

func TestDoctorReportsMissingEvents(t *testing.T) {
	s := testStore(t)
	claudeDir := filepath.Join(s.config.Root, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatal(err)
	}
	command, _ := s.hookCommand("claude", "UserPromptSubmit")
	settings := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":` + quoteJSON(command) + `}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	result := s.doctor()
	check := doctorCheck(result, "claude_hooks")
	if check == nil || check["status"] != "warn" || !strings.Contains(textField(check["detail"]), "SessionStart") || !strings.Contains(textField(check["detail"]), "PreToolUse") {
		t.Fatalf("missing events not reported: %v", check)
	}
	if result["status"] != "attention" {
		t.Fatalf("overall status: %v", result["status"])
	}
	if _, err := s.installHook("claude"); err != nil {
		t.Fatal(err)
	}
	if check := doctorCheck(s.doctor(), "claude_hooks"); check["status"] != "ok" {
		t.Fatalf("installed hooks not recognised: %v", check)
	}
	for _, name := range []string{"store", "codex_hooks", "codex_trust", "maintenance", "cron"} {
		if doctorCheck(result, name) == nil {
			t.Fatalf("check %s missing", name)
		}
	}
}

func quoteJSON(text string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), `"`, `\"`) + `"`
}
