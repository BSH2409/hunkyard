package preferences

import "testing"

// A source-pack core rule that maintenance replaced or quarantined is not
// injected at session start next to its replacement.
func TestSelectCoreSkipsRulesThatAreNotLive(t *testing.T) {
	global := map[string]any{"kind": "global", "repo_key": nil}
	coreRule := func(id, directive, status string) rule {
		return rule{"id": id, "directive": directive, "scope": global, "origin": "source_pack", "load_tier": "core", "status": status, "surfaces": []any{"naming"}}
	}
	selection := selectCore([]rule{
		coreRule("rule_live", "Name services by domain operation.", "active"),
		coreRule("rule_old", "Name services after their class.", "superseded"),
		coreRule("rule_disputed", "Prefix services with Service.", "conflicted"),
	}, "")
	if len(selection.RuleIDs) != 1 || selection.RuleIDs[0] != "rule_live" {
		t.Fatalf("core selection = %v, want only the live rule", selection.RuleIDs)
	}
}
