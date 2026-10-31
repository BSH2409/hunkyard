package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookInstallerStrictMerge(t *testing.T) {
	for _, original := range []string{`{invalid`, `{"hooks":{},"hooks":{}}`, `{"hooks":null}`, `{"hooks":{"UserPromptSubmit":null}}`} {
		t.Run(original, func(t *testing.T) {
			s := testStore(t)
			if err := ensureDirectory(s.config.Root); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(s.config.Root, "hooks.json")
			if err := os.WriteFile(target, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.installHook("codex"); err == nil {
				t.Fatal("malformed hooks accepted")
			}
			data, err := regularFile(target)
			if err != nil || string(data) != original {
				t.Fatal("invalid settings were changed", err)
			}
			if _, err := os.Stat(s.dataPath("backups")); !os.IsNotExist(err) {
				t.Fatal("invalid settings backed up before validation")
			}
		})
	}
	s := testStore(t)
	if err := ensureDirectory(s.config.Root); err != nil {
		t.Fatal(err)
	}
	original := `{"other":42,"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"keep pre"}]}],"UserPromptSubmit":[{"matcher":"keep","hooks":[{"type":"command","command":"keep prompt"},{"type":"command","command":"ruby /old/learn-preferences/scripts/user_prompt_hook.rb"}]},{"hooks":[{"type":"command","command":"ruby /old/learn-preferences/scripts/user_prompt_hook.rb"}]}]}}`
	target := filepath.Join(s.config.Root, "hooks.json")
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, err := s.installHook("codex")
		if err != nil || result["trust_required"] != true {
			t.Fatal("install failed", result, err)
		}
		backup, err := regularFile(textField(result["backup_path"]))
		if err != nil || !json.Valid(backup) {
			t.Fatal("backup is not verified settings", err)
		}
	}
	data, err := regularFile(target)
	if err != nil || strings.Contains(string(data), "user_prompt_hook.rb") || strings.Count(string(data), "keep prompt") != 1 || strings.Count(string(data), "keep pre") != 1 || bytes.Contains(data, []byte(`"trusted"`)) {
		t.Fatal("merge lost unrelated handlers or synthesized trust", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil || document["other"] != float64(42) {
		t.Fatal("unrelated setting changed", err)
	}
	groups, err := mapArray(record(document["hooks"])["UserPromptSubmit"])
	if err != nil || len(groups) != 2 || groups[0]["matcher"] != "keep" {
		t.Fatal("handler merge changed matchers or duplicated the handler", err)
	}
	entries, err := mapArray(groups[1]["hooks"])
	if err != nil || len(entries) != 1 || entries[0]["timeout"] != float64(5) || entries[0]["additionalContextLimit"] != float64(1500) {
		t.Fatal("official hook configuration differs", err)
	}
}

func TestCronMergeReferenceBoundaries(t *testing.T) {
	s := testStore(t)
	t.Setenv("PATH", strings.Repeat("/unused/interactive/runtime:", 100))
	before := "# disabled\n#* * * * * curl http://127.0.0.1/watcher\n\n"
	after := "after\n"
	text := before + cronBegin + "\nold schedule\n" + cronEnd + "\n" + after
	first, err := s.renderCron(text)
	if err != nil || !strings.HasPrefix(first, before+cronBegin+"\n17 * * * *") || !strings.HasSuffix(first, cronEnd+"\n"+after) || strings.Contains(first, "/unused/interactive") || len(first) >= 1000 {
		t.Fatal("cron merge or minimal runtime path differs", err)
	}
	second, err := s.renderCron(first)
	if err != nil || second != first {
		t.Fatal("cron merge is not idempotent", err)
	}
	for _, invalid := range []string{cronBegin + "\n", cronEnd + "\n", cronBegin + "\n" + cronBegin + "\n" + cronEnd + "\n", cronBegin + "\n" + cronEnd + "\n" + cronEnd + "\n"} {
		if _, err := s.renderCron(invalid); err == nil {
			t.Fatal("malformed cron markers accepted")
		}
	}
}

func TestPreferenceHookInstallerRollback(t *testing.T) {
	s := testStore(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "claude"))
	if err := os.MkdirAll(s.config.Root, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(s.config.Root, "hooks.json")
	original := `{"other":"keep","hooks":{"UserPromptSubmit":[{"matcher":"*","hooks":[{"type":"command","command":"printf keep"}]}]}}`
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	s.fault = func(stage string) error {
		if stage == "hook_written" {
			return errors.New("injected install fault")
		}
		return nil
	}
	if _, err := s.installHook("codex"); err == nil {
		t.Fatal("installer did not surface failure")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != original {
		t.Fatal("failed install did not restore settings", err)
	}
	s.fault = nil
	for _, host := range []string{"codex", "claude"} {
		for i := 0; i < 2; i++ {
			result, err := s.installHook(host)
			if err != nil || result["trust_required"] != true {
				t.Fatal("hook install failed", err)
			}
		}
	}
	document := readJSONFile(target)
	if document["other"] != "keep" {
		t.Fatal("unrelated setting lost")
	}
	groups, _ := mapArray(record(document["hooks"])["UserPromptSubmit"])
	if len(groups) != 2 {
		t.Fatal("hook duplicated or existing hook removed")
	}
}

func TestPreferenceCronInstallAndRollback(t *testing.T) {
	s := testStore(t)
	original := "15 * * * * echo keep\n"
	text := original
	writes := 0
	read := func() (string, error) {
		if writes == 1 {
			return text + "# mismatch\n", nil
		}
		return text, nil
	}
	write := func(value string) error { text = value; writes++; return nil }
	if _, err := s.installCron(read, write); err == nil {
		t.Fatal("readback mismatch accepted")
	}
	if text != original {
		t.Fatal("cron rollback lost original")
	}
	read = func() (string, error) { return text, nil }
	result, err := s.installCron(read, write)
	if err != nil || result["schedule"] != "17 * * * *" || !strings.HasPrefix(text, original) {
		t.Fatal("cron installation failed", err)
	}
	if _, err := s.installCron(read, write); err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, cronBegin) != 1 {
		t.Fatal("cron block duplicated")
	}
	info, err := os.Stat(s.dataPath("logs", "cron.log"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cron log is not private", err)
	}
	if _, err := s.renderCron(cronBegin + "\n"); err == nil {
		t.Fatal("unmatched marker accepted")
	}
}

func TestPreferenceCronRejectsSymlinkLog(t *testing.T) {
	s := testStore(t)
	if err := ensureDirectory(s.dataPath("logs")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, s.dataPath("logs", "cron.log")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	written := false
	if _, err := s.installCron(func() (string, error) { return "", nil }, func(string) error { written = true; return nil }); err == nil || written {
		t.Fatal("cron installed with a symlink log")
	}
	data, err := regularFile(outside)
	if err != nil || string(data) != "keep" {
		t.Fatal("cron followed log symlink", err)
	}
}

func TestHookPathFollowsHostConfiguration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LEARN_PREFERENCES_HOME", "")
	tests := []struct {
		host, codexHome, claudeDir, want string
	}{
		{"codex", "", "", filepath.Join(home, ".codex", "hooks.json")},
		{"codex", filepath.Join(home, "codex-custom"), "", filepath.Join(home, "codex-custom", "hooks.json")},
		{"claude", "", "", filepath.Join(home, ".claude", "settings.json")},
		{"claude", "", filepath.Join(home, ".claude-office"), filepath.Join(home, ".claude-office", "settings.json")},
	}
	for _, tt := range tests {
		t.Setenv("CODEX_HOME", tt.codexHome)
		t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeDir)
		s, err := Open(Config{})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := s.hookPath(tt.host); err != nil || got != tt.want {
			t.Errorf("hookPath(%s) with CODEX_HOME=%q CLAUDE_CONFIG_DIR=%q = %q, %v; want %q", tt.host, tt.codexHome, tt.claudeDir, got, err, tt.want)
		}
	}
}

func TestInstallHookReplacesEarlierRootHook(t *testing.T) {
	s := testStore(t)
	if err := ensureDirectory(s.config.Root); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(s.config.Root, "hooks.json")
	original := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"'/old/hunkyard' preferences hook --root '/home/x/.codex'"},{"type":"command","command":"echo hi"}]}]}}`
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.installHook("codex"); err != nil {
		t.Fatal(err)
	}
	data, err := regularFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), " preferences hook --event UserPromptSubmit "); n != 1 {
		t.Fatalf("want exactly one preference hook, got %d: %s", n, data)
	}
	if strings.Contains(string(data), "/home/x/.codex") || !strings.Contains(string(data), "echo hi") {
		t.Fatalf("stale hook kept or unrelated hook lost: %s", data)
	}
}

func TestInstallHookWritesAllEventsPerHost(t *testing.T) {
	want := map[string][]string{"claude": {"PreToolUse", "SessionStart", "Stop", "UserPromptSubmit"}, "codex": {"PreToolUse", "SessionStart", "Stop", "UserPromptSubmit"}}
	for host, events := range want {
		t.Run(host, func(t *testing.T) {
			s := testStore(t)
			t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(s.config.Root, "claude"))
			result, err := s.installHook(host)
			if err != nil {
				t.Fatal(err)
			}
			hooks := record(readJSONFile(textField(result["path"]))["hooks"])
			for _, event := range events {
				groups, err := mapArray(hooks[event])
				if err != nil {
					t.Fatalf("%s missing: %v", event, err)
				}
				found := 0
				for _, group := range groups {
					entries, _ := mapArray(group["hooks"])
					for _, entry := range entries {
						if strings.Contains(textField(entry["command"]), " preferences hook --event "+event+" ") {
							found++
						}
					}
					if event == "PreToolUse" && textField(group["matcher"]) == "" {
						t.Fatal("PreToolUse needs a shell matcher")
					}
				}
				if found != 1 {
					t.Fatalf("%s: want one Hunkyard handler, got %d", event, found)
				}
			}
		})
	}
}
