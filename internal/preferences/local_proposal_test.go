package preferences

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaintenanceNeverExecutesModelProviders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	s := testStore(t)
	bin := t.TempDir()
	marker := filepath.Join(bin, "provider-was-run")
	for _, name := range []string{"codex", "claude", "ruby"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf invoked > "+shellQuote(marker)+"\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for _, mode := range []string{"dry_run", "due", "force"} {
		result, err := s.maintain(mode, nil)
		if err != nil || result["status"] != "needs_proposal" {
			t.Fatal("local batch export failed", result, err)
		}
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.maintain("force", ignoredProposal(t, batch)); err != nil || result["status"] != "success" {
		t.Fatal("empty local proposal failed", result, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("maintenance invoked an installed model provider")
	}
}

func TestMaintenanceSkillContainsSafetyAndActionContract(t *testing.T) {
	data, err := os.ReadFile("../../integrations/preferences/references/local-maintenance.md")
	if err != nil {
		t.Fatal(err)
	}
	phrases := []string{"input_digest", "observation_ids", "reason_code", "target_rule_ids", "source_ref", "proposed_rule", "schema_version", "evidence", "polarity"}
	for action := range summaryActions {
		phrases = append(phrases, action)
	}
	for _, phrase := range phrases {
		if !strings.Contains(string(data), phrase) {
			t.Errorf("local maintenance skill omits %s", phrase)
		}
	}
}
