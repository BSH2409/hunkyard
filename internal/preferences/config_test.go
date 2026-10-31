package preferences

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitEmptyPreferenceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LEARN_PREFERENCES_HOME", filepath.Join(home, "canonical"))
	for _, root := range []string{" ", string(filepath.Separator)} {
		if _, err := Open(Config{Root: root}); err == nil {
			t.Fatal("invalid root accepted")
		}
	}
	var out, errOut bytes.Buffer
	if exit := Run([]string{"capture", "--root", "", "--feedback", "Always prefer clear names", "--json"}, strings.NewReader(""), &out, &errOut); exit != 3 {
		t.Fatal("explicit empty CLI root fell back to personal state", exit, out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "canonical")); !os.IsNotExist(err) {
		t.Fatal("invalid explicit root wrote canonical state")
	}
}

func TestConfigRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LEARN_PREFERENCES_HOME", "")
	config, err := resolveConfig(Config{})
	if err != nil || config.Root != filepath.Join(home, ".agents") || config.Clock == nil {
		t.Fatalf("default configuration: %#v, %v", config, err)
	}
	if _, err := os.Stat(config.Root); !os.IsNotExist(err) {
		t.Fatal("configuration resolution wrote state")
	}
	t.Setenv("LEARN_PREFERENCES_HOME", filepath.Join(home, "custom"))
	config, err = resolveConfig(Config{})
	if err != nil || config.Root != filepath.Join(home, "custom") {
		t.Fatalf("LEARN_PREFERENCES_HOME precedence: %#v, %v", config, err)
	}
	t.Setenv("LEARN_PREFERENCES_HOME", "")
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	config, err = resolveConfig(Config{})
	if err != nil || config.Root != filepath.Join(home, ".agents") {
		t.Fatalf("CODEX_HOME moved the preference store: %#v, %v", config, err)
	}
	for _, root := range []string{string(filepath.Separator), "  "} {
		if _, err := resolveConfig(Config{Root: root}); err == nil {
			t.Errorf("accepted unsafe root %q", root)
		}
	}
}

func TestDefaultPreferenceRootAndPrivateModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LEARN_PREFERENCES_HOME", "")
	s, err := Open(Config{})
	if err != nil || s.config.Root != filepath.Join(home, ".agents") {
		t.Fatal("default root differs", err)
	}
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{{s.dataPath(), 0700}, {s.dataPath("observations"), 0700}, {s.dataPath("observations", "pending.jsonl"), 0600}, {s.dataPath("observations", ".pending.lock"), 0600}} {
		info, err := os.Stat(item.path)
		if err != nil || info.Mode().Perm() != item.mode {
			t.Fatalf("private mode differs for %s: %v", item.path, err)
		}
	}
	t.Setenv("LEARN_PREFERENCES_HOME", filepath.Join(home, "custom"))
	custom, err := resolveConfig(Config{})
	if err != nil || custom.Root != filepath.Join(home, "custom") {
		t.Fatal("explicit canonical root ignored", err)
	}
}

func TestBlankHomeUsesAccountHomeWithoutWriting(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("LEARN_PREFERENCES_HOME", "")
	account, err := user.Current()
	if err != nil {
		t.Skip("account home lookup unavailable")
	}
	config, err := resolveConfig(Config{})
	if err != nil || config.Root != filepath.Join(account.HomeDir, ".agents") {
		t.Fatal("blank HOME resolved incorrectly", err)
	}
}
