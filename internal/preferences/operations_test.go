package preferences

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonaOperations(t *testing.T) {
	s := testStore(t)
	result, err := s.build()
	if err != nil || result.Status != "no_generation" {
		t.Fatal("build created an empty generation", err)
	}
	if _, err := os.Stat(s.config.Root); !os.IsNotExist(err) {
		t.Fatal("read-only build created preference root")
	}
	input := captureInput()
	input.Explicit = true
	input.Scope = "global"
	input.RepoKey = ""
	input.Directive = "Prefer early returns over nested conditionals."
	input.Surfaces = []string{"ruby"}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	result, err = s.build()
	if err != nil || result.Status != "no_change" {
		t.Fatal("build changed current rendered generation", err)
	}
	found, err := s.search("early returns", []string{"ruby"})
	if err != nil || len(found) != 1 || len(found[0]) != 5 {
		t.Fatal("search lost result or exposed evidence", err)
	}
	if _, err := s.search("", []string{"invented"}); err == nil {
		t.Fatal("invalid search surface accepted")
	}
	audit := s.audit(true)
	if audit.Status != "success" || len(audit.Errors) != 0 {
		t.Fatalf("healthy store failed audit: %#v", audit)
	}
	if err := os.WriteFile(filepath.Join(s.config.Root, "persona", "ruby.md"), []byte("manual change"), 0600); err != nil {
		t.Fatal(err)
	}
	audit = s.audit(false)
	if !strings.Contains(strings.Join(audit.Errors, ","), "persona_mirror_mismatch") {
		t.Fatal("audit missed mirror modification")
	}
	if err := s.repairMirrors(); err == nil {
		t.Fatal("repair overwrote a manual persona")
	}
	current, err := s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.config.Root, "persona", "ruby.md"), []byte(current.PersonaFiles["ruby.md"]), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = s.retire(textField(found[0]["id"]))
	if err != nil || result.Action == nil || *result.Action != "retired" {
		t.Fatal("retirement failed", err)
	}
	found, err = s.search("early", nil)
	if err != nil || len(found) != 0 {
		t.Fatal("retired preference remains searchable", err)
	}
}
