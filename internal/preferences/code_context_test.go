package preferences

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestReviewCaptureKeepsCodeContext(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.Source = "review"
	input.Feedback = "remove these comments"
	input.CodeContext = &CodeContext{Path: "app/services/sync.rb", StartLine: 10, EndLine: 12, Excerpt: "# Sync the thing\n# then return\nsync!"}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	stored := pendingObservations(t, s)[0].CodeContext
	if stored == nil || stored.Path != "app/services/sync.rb" || stored.StartLine != 10 || !strings.Contains(stored.Excerpt, "sync!") {
		t.Fatalf("code context lost: %+v", stored)
	}
	input.TurnID = "turn-abs"
	input.CodeContext = &CodeContext{Path: "/home/user/secret/app.rb", StartLine: 1, EndLine: 1, Excerpt: "api_key = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'"}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	second := pendingObservations(t, s)[1].CodeContext
	if second == nil || second.Path != "" || strings.Contains(second.Excerpt, "AAAA") {
		t.Fatalf("absolute path or secret kept: %+v", second)
	}
}

func TestBatchIncludesOldestCandidates(t *testing.T) {
	s := testStore(t)
	current, _ := s.loadCurrent()
	catalog, err := s.catalogFromGeneration(current)
	if err != nil {
		t.Fatal(err)
	}
	global := map[string]any{"kind": "global", "repo_key": nil}
	for i := range 12 {
		r := newRule(fmt.Sprintf("Candidate preference number %02d.", i), global, []string{"naming"}, "inferred", fixedNow.Add(time.Duration(i)*time.Hour))
		if err := recomputeRule(r, fixedNow.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
		catalog.rules = append(catalog.rules, r)
	}
	files, manifest := renderPersona(catalog.rules)
	if _, err := s.commitGeneration(generation{Rules: catalog.ruleSet(), ProcessedIDs: []string{}, SourcePacks: map[string]any{}, PersonaFiles: files, Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.buildMaintenanceBatch(false)
	if err != nil {
		t.Fatal(err)
	}
	review := stringValues(batch.document()["candidate_review"])
	if len(review) != 10 {
		t.Fatalf("want 10 candidates, got %d", len(review))
	}
	oldest := ruleID("Candidate preference number 00.", global, []string{"naming"})
	newest := ruleID("Candidate preference number 11.", global, []string{"naming"})
	if review[0] != oldest || strings.Contains(strings.Join(review, ","), newest) {
		t.Fatalf("not the oldest candidates: %v", review)
	}
}
