package preferences

import (
	"strings"
	"testing"
	"time"
)

func TestRecordActivityAppendsAndReadsNewestFirst(t *testing.T) {
	s := testStore(t)
	for i, kind := range []string{activityLearned, activityConfirmed, activityViolated} {
		at := time.Date(2026, 8, 19, 12, i, 0, 0, time.UTC).Format(time.RFC3339)
		if err := s.recordActivity(ActivityEvent{At: at, Kind: kind, RuleID: "rule_a"}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.Activity(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != activityViolated || events[1].Kind != activityConfirmed {
		t.Fatalf("newest-first window wrong: %+v", events)
	}
}

func TestRecordActivityDefaultsTimestamp(t *testing.T) {
	s := testStore(t)
	if err := s.recordActivity(ActivityEvent{Kind: activityCaptured}); err != nil {
		t.Fatal(err)
	}
	events, _ := s.Activity(1)
	if len(events) != 1 || events[0].At != "2026-08-19T12:00:00Z" {
		t.Fatalf("timestamp not defaulted from store clock: %+v", events)
	}
}

func TestActivityDetailIsSanitized(t *testing.T) {
	s := testStore(t)
	if err := s.recordActivity(ActivityEvent{Kind: activityBlocked, Detail: "password=hunter2 in commit"}); err != nil {
		t.Fatal(err)
	}
	if err := s.recordActivity(ActivityEvent{Kind: activityBlocked, Detail: strings.Repeat("x", 500)}); err != nil {
		t.Fatal(err)
	}
	events, _ := s.Activity(2)
	if strings.Contains(events[1].Detail, "hunter2") {
		t.Fatal("secret stored in activity detail")
	}
	if len(events[0].Detail) > 240 {
		t.Fatalf("detail not capped: %d", len(events[0].Detail))
	}
}

func TestPruneRetentionDropsOldActivity(t *testing.T) {
	s := testStore(t)
	now := s.config.Clock()
	for _, age := range []int{31, 29} {
		at := now.Add(-time.Duration(age) * 24 * time.Hour).Format(time.RFC3339)
		if err := s.recordActivity(ActivityEvent{At: at, Kind: activityLearned, Detail: "age"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.pruneRetention(""); err != nil {
		t.Fatal(err)
	}
	events, _ := s.Activity(10)
	if len(events) != 1 || events[0].At != now.Add(-29*24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("retention wrong: %+v", events)
	}
}

func TestExplicitCaptureRecordsLearnedThenViolated(t *testing.T) {
	s := testStore(t)
	input := CaptureInput{Feedback: "Never use single-line method definitions", Directive: "Write method definitions over multiple lines.", Scope: "global", Surfaces: []string{"ruby"}, Explicit: true, Source: "cli", SessionID: "s1", TurnID: "t1"}
	first, err := s.Capture(input)
	if err != nil || first.RuleID == "" {
		t.Fatalf("first capture: %+v %v", first, err)
	}
	input.TurnID = "t2"
	second, err := s.Capture(input)
	if err != nil || second.RuleID != first.RuleID {
		t.Fatalf("second capture: %+v %v", second, err)
	}
	events, _ := s.Activity(10)
	if len(events) != 2 || events[1].Kind != activityLearned || events[0].Kind != activityViolated || events[0].RuleID != first.RuleID {
		t.Fatalf("want learned then violated for %s: %+v", first.RuleID, events)
	}
}

func TestProposalActivityClassifiesConfirmations(t *testing.T) {
	observations := []map[string]any{
		{"id": "obs_correction", "signals": map[string]any{"correction": true}},
		{"id": "obs_plain", "signals": map[string]any{"correction": false}},
	}
	actions := []any{
		map[string]any{"type": "create_rule", "rule_id": "rule_new", "observation_ids": []any{}},
		map[string]any{"type": "confirm_rule", "rule_id": "rule_a", "observation_ids": []any{"obs_correction"}},
		map[string]any{"type": "confirm_rule", "rule_id": "rule_b", "observation_ids": []any{"obs_plain"}},
		map[string]any{"type": "ignore_observation", "rule_id": nil, "observation_ids": []any{}},
	}
	got := proposalActivity(actions, observations)
	want := []ActivityEvent{{Kind: activityLearned, RuleID: "rule_new"}, {Kind: activityViolated, RuleID: "rule_a"}, {Kind: activityConfirmed, RuleID: "rule_b"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].RuleID != want[i].RuleID {
			t.Fatalf("event %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSanitizeIsIdempotentWhenTruncating(t *testing.T) {
	long := strings.Repeat("word ", 199) + "tail more text after the limit"
	first := sanitize(long)
	if again := sanitize(first.Excerpt); again.Excerpt != first.Excerpt {
		t.Fatalf("re-sanitizing changed the excerpt: %q -> %q", first.Excerpt[len(first.Excerpt)-10:], again.Excerpt[len(again.Excerpt)-10:])
	}
}
