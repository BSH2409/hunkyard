package preferences

import (
	"strings"
	"testing"
)

// A superseded source rule keeps the citation it was created from; only live
// rules must cite the current source files.
func TestSupersededSourceRuleKeepsItsOriginalCitation(t *testing.T) {
	old, current := strings.Repeat("a", 64), strings.Repeat("b", 64)
	packs := map[string]any{"code-like-him": map[string]any{"digest": current, "files": map[string]any{"reference/testing.md": current}, "refreshed_at": "2026-11-01T00:00:00Z"}}
	superseded := rule{"origin": "source_pack", "source_pack": "code-like-him", "status": "superseded", "superseded_by": "rule_new", "source_ref": map[string]any{"file": "reference/testing.md", "digest": old}}
	if err := validateSourceMetadata(packs, []rule{superseded}); err != nil {
		t.Fatalf("a superseded rule's historical citation blocked the update: %v", err)
	}
	live := rule{"origin": "source_pack", "source_pack": "code-like-him", "status": "active", "source_ref": map[string]any{"file": "reference/testing.md", "digest": old}}
	if err := validateSourceMetadata(packs, []rule{live}); err == nil {
		t.Fatal("a live rule with a stale citation was accepted")
	}
}
