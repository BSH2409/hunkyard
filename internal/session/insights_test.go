package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func insightsHome(t *testing.T, now time.Time) string {
	t.Helper()
	home := t.TempDir()
	setHome(t, home)
	at := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	writeJSONFile(t, filepath.Join(home, ".hunkyard", "stats.json"), statsFile{Sessions: []sessionRecord{
		{StartedAt: at(3 * time.Hour), EndedAt: at(2 * time.Hour), Rounds: 3, Comments: 6, ReviewKey: "aaaaaaaaaaaa", Approved: true, ApprovedAt: at(2 * time.Hour)},
		{StartedAt: at(26 * time.Hour), EndedAt: at(25*time.Hour + 30*time.Minute), Rounds: 1, Comments: 2, ReviewKey: "bbbbbbbbbbbb"},
		{StartedAt: at(25 * time.Hour), EndedAt: at(24 * time.Hour), Rounds: 2, Comments: 1, ReviewKey: "bbbbbbbbbbbb", Approved: true, ApprovedAt: at(24 * time.Hour)},
		{StartedAt: at(60 * 24 * time.Hour), EndedAt: at(60 * 24 * time.Hour), Rounds: 9, Comments: 99, ReviewKey: "cccccccccccc"},
	}})
	review := HunkyardJSON{CWD: "/work/orion", ReviewRound: 3, Files: map[string]HunkyardJSONFile{"a.go": {Comments: []Comment{
		{ID: "c1", Label: "issue", Resolved: true, Body: "x"},
		{ID: "c2", Label: "nit", Body: "x"},
		{ID: "c3", Body: "x", ClaimUnchangedRound: 2},
		{ID: "c4", Body: "tool", Source: "tool", SourceRef: "eslint"},
		{ID: "c5", Body: "agent", AuthorKind: "agent"},
	}}}}
	writeJSONFile(t, filepath.Join(home, ".hunkyard", "reviews", "aaaaaaaaaaaa", "review.json"), review)
	writeJSONFile(t, filepath.Join(home, ".hunkyard", "reviews", "aaaaaaaaaaaa", "feedback", "state.json"), map[string]any{
		"preferences": map[string]any{
			"batch_1:x": map[string]any{"outcome": map[string]any{"comment_id": "c1", "status": "captured", "observation_id": "obs_1"}},
			"batch_1:y": map[string]any{"outcome": map[string]any{"comment_id": "c2", "status": "captured", "observation_id": "obs_2"}},
			"batch_1:z": map[string]any{"outcome": map[string]any{"comment_id": "c3", "status": "ignored"}},
		},
	})
	return home
}

func TestComputeReviewInsights(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	insightsHome(t, now)
	sources := InsightSources{
		RulesForObservations: func(ids []string) map[string]preferences.RuleRef {
			return map[string]preferences.RuleRef{"obs_1": {ID: "rule_a", Directive: "Name things well."}, "obs_2": {ID: "rule_a", Directive: "Name things well."}}
		},
		RuleVerdicts: func() map[string]preferences.VerdictCount {
			return map[string]preferences.VerdictCount{"rule_a": {Confirmed: 1, Dismissed: 2}, "rule_b": {Dismissed: 1}}
		},
	}
	got, err := ComputeReviewInsights(30, now, sources)
	if err != nil {
		t.Fatal(err)
	}
	if got.WindowDays != 30 || got.Reviews != 2 || got.Approved != 2 {
		t.Fatalf("counts = %+v", got)
	}
	if got.RoundsPerReview != (Quantiles{Median: 2.5, Max: 3}) {
		t.Fatalf("rounds = %+v", got.RoundsPerReview)
	}
	if got.CommentsPerRound != 9.0/5.0 {
		t.Fatalf("comments per round = %v", got.CommentsPerRound)
	}
	if got.ApproveSeconds != (Quantiles{Median: 3600, Max: 3600}) {
		t.Fatalf("approve seconds = %+v", got.ApproveSeconds)
	}
	if got.ByLabel["issue"] != 1 || got.ByLabel["nit"] != 1 || got.ByLabel["unlabelled"] != 1 || len(got.ByLabel) != 3 {
		t.Fatalf("labels = %+v", got.ByLabel)
	}
	if got.ByResolution != (ResolutionCounts{Resolved: 1, Unresolved: 2, ClaimUnchanged: 1}) {
		t.Fatalf("resolution = %+v", got.ByResolution)
	}
	if len(got.ByRepository) != 2 || got.ByRepository[0].Repository != "orion" || got.ByRepository[0].Reviews != 1 {
		t.Fatalf("repositories = %+v", got.ByRepository)
	}
	if len(got.Rules) != 2 || got.Rules[0] != (RuleInsight{RuleID: "rule_a", Directive: "Name things well.", Comments: 2, Confirmed: 1, Dismissed: 2}) {
		t.Fatalf("rules = %+v", got.Rules)
	}
	all, _ := ComputeReviewInsights(0, now, InsightSources{})
	if all.Reviews != 3 || all.WindowDays != 0 {
		t.Fatalf("all time = %+v", all)
	}
}

func TestComputeReviewInsightsEmpty(t *testing.T) {
	setHome(t, t.TempDir())
	got, err := ComputeReviewInsights(30, time.Now(), InsightSources{})
	if err != nil || got.Reviews != 0 || got.ByLabel == nil || got.Rules == nil || got.ByRepository == nil {
		t.Fatalf("empty = %+v %v", got, err)
	}
}

func captureStats(t *testing.T, args ...string) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })
	RunStats(args)
	_ = writer.Close()
	os.Stdout = original
	data, _ := io.ReadAll(reader)
	return string(data)
}

func TestRunStatsPrintsInsights(t *testing.T) {
	insightsHome(t, time.Now())
	text := captureStats(t, "--days", "30")
	for _, want := range []string{"Last 30 days: 2 reviews, 2 approved", "Rounds per review: median 2.5, max 3", "Comments by label: issue 1, nit 1, unlabelled 1", "orion: 1 review"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stats output lacks %q:\n%s", want, text)
		}
	}
	var decoded struct {
		Insights ReviewInsights `json:"insights"`
	}
	if err := json.Unmarshal([]byte(captureStats(t, "--json", "--days", "0")), &decoded); err != nil || decoded.Insights.Reviews != 3 {
		t.Fatalf("json insights = %+v %v", decoded.Insights, err)
	}
}
