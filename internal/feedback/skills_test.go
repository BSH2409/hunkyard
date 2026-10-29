package feedback

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillSelectionPinsTrustedManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: clear-names\n---\nUse clear names."), 0600); err != nil {
		t.Fatal(err)
	}
	refs, err := PinSkills([]SkillRef{{ID: "clear-names", Name: "Clear names", Path: path, Source: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := ValidateSelection(refs, []string{"clear-names", "clear-names"})
	if err != nil || len(selected) != 1 {
		t.Fatalf("selection: %#v %v", selected, err)
	}
	if _, err := ValidateSelection(refs, []string{"unregistered"}); err == nil {
		t.Fatal("unregistered skill accepted")
	}
	if err := os.WriteFile(path, []byte("Changed skill instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSelection(refs, []string{"clear-names"}); err == nil {
		t.Fatal("replaced skill accepted")
	}
}

func TestSkillCatalogCannotReplacePublishedProvenance(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "SKILL.md")
	os.WriteFile(path, []byte("Skill instructions"), 0600)
	refs, err := PinSkills([]SkillRef{{ID: "skill", Name: "Skill", Path: path, Source: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterSkills(refs); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterSkills(refs); err != nil {
		t.Fatal(err)
	}
	refs[0].Source = "untrusted-replacement"
	if err := store.RegisterSkills(refs); err == nil {
		t.Fatal("existing host manifest replaced")
	}
}
