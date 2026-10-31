package preferences

import (
	"slices"
	"strings"
	"testing"
)

func TestEditedPathsClaudeAndCodex(t *testing.T) {
	if got := editedPaths("Edit", map[string]any{"file_path": "/repo/app/models/user.rb"}); !slices.Equal(got, []string{"/repo/app/models/user.rb"}) {
		t.Fatalf("claude edit: %v", got)
	}
	patch := "*** Begin Patch\n*** Update File: app/jobs/sync_job.rb\n@@\n-a\n+b\n*** Add File: spec/jobs/sync_job_spec.rb\n+it\n*** End Patch"
	if got := editedPaths("apply_patch", map[string]any{"command": []any{"apply_patch", patch}}); !slices.Equal(got, []string{"app/jobs/sync_job.rb", "spec/jobs/sync_job_spec.rb"}) {
		t.Fatalf("codex patch: %v", got)
	}
	if got := editedPaths("Read", map[string]any{"file_path": "/repo/a.rb"}); got != nil {
		t.Fatalf("read tool treated as an edit: %v", got)
	}
}

func TestPathSurfaces(t *testing.T) {
	got := pathSurfacesFor([]string{"app/jobs/sync_job.rb", "db/migrate/1_add.rb"}, "")
	for _, want := range []string{"ruby", "rails", "jobs", "database"} {
		if !slices.Contains(got, want) {
			t.Fatalf("surfaces %v missing %s", got, want)
		}
	}
}

func editContext(t *testing.T, s *Store, tool string, input map[string]any) map[string]any {
	t.Helper()
	return hookResponse(t, s, "PreToolUse", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "tool_name": tool, "tool_input": input})
}

func TestEditInjectsRubyRulesOnce(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Write Ruby method definitions over multiple lines.", "ruby")
	captureRule(t, s, "Prefer CSS variables for colours.", "frontend")
	first := editContext(t, s, "Edit", map[string]any{"file_path": "/repo/app/models/user.rb"})
	output := record(first["hookSpecificOutput"])
	context := textField(output["additionalContext"])
	if !strings.Contains(context, "multiple lines") || strings.Contains(context, "CSS variables") || output["permissionDecision"] != nil {
		t.Fatalf("edit context: %v", first)
	}
	second := editContext(t, s, "Write", map[string]any{"file_path": "/repo/app/models/account.rb"})
	if second["hookSpecificOutput"] != nil {
		t.Fatalf("same rules injected twice: %v", second)
	}
}

func TestEditUnknownToolPasses(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Write Ruby method definitions over multiple lines.", "ruby")
	if response := editContext(t, s, "Glob", map[string]any{"pattern": "**/*.rb"}); response["hookSpecificOutput"] != nil {
		t.Fatalf("non-edit tool got context: %v", response)
	}
}
