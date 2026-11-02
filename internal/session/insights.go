package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

// Quantiles summarises a set of numbers.
type Quantiles struct {
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// ResolutionCounts splits human comments by outcome.
type ResolutionCounts struct {
	Resolved       int `json:"resolved"`
	Unresolved     int `json:"unresolved"`
	ClaimUnchanged int `json:"claim_unchanged"`
}

// RepositoryInsight counts reviews and comments of one repository.
type RepositoryInsight struct {
	Repository string `json:"repository"`
	Reviews    int    `json:"reviews"`
	Comments   int    `json:"comments"`
}

// RuleInsight says how often a learned rule was behind a comment and how
// reviewers judged its rule-check findings.
type RuleInsight struct {
	RuleID    string `json:"rule_id"`
	Directive string `json:"directive"`
	Comments  int    `json:"comments"`
	Confirmed int    `json:"confirmed"`
	Dismissed int    `json:"dismissed"`
}

// ReviewInsights describes the reviews that ended in the window.
type ReviewInsights struct {
	WindowDays       int                 `json:"window_days"`
	Reviews          int                 `json:"reviews"`
	Approved         int                 `json:"approved"`
	RoundsPerReview  Quantiles           `json:"rounds_per_review"`
	CommentsPerRound float64             `json:"comments_per_round"`
	ApproveSeconds   Quantiles           `json:"approve_seconds"`
	ByLabel          map[string]int      `json:"by_label"`
	ByResolution     ResolutionCounts    `json:"by_resolution"`
	ByRepository     []RepositoryInsight `json:"by_repository"`
	Rules            []RuleInsight       `json:"rules"`
}

// InsightSources reads the preference store; either may be nil.
type InsightSources struct {
	RulesForObservations func(ids []string) map[string]preferences.RuleRef
	RuleVerdicts         func() map[string]preferences.VerdictCount
}

type reviewTally struct {
	key, repository string
	rounds          int
	comments        int
}

// ComputeReviewInsights reads ~/.hunkyard/stats.json and the review folders
// of the reviews that ended in the last days (0 means all time).
func ComputeReviewInsights(days int, now time.Time, src InsightSources) (ReviewInsights, error) {
	out := ReviewInsights{WindowDays: days, ByLabel: map[string]int{}, ByRepository: []RepositoryInsight{}, Rules: []RuleInsight{}}
	sf, err := loadStats()
	if err != nil {
		return out, err
	}
	since := time.Time{}
	if days > 0 {
		since = now.AddDate(0, 0, -days)
	}
	tallies := map[string]*reviewTally{}
	var order []string
	approved := map[string]bool{}
	var approveSeconds []float64
	for i, rec := range sf.Sessions {
		ended, err := time.Parse(time.RFC3339, rec.EndedAt)
		if err != nil || ended.Before(since) {
			continue
		}
		key := rec.ReviewKey
		if key == "" {
			key = "record-" + strconv.Itoa(i) // an older record: count it on its own
		}
		t := tallies[key]
		if t == nil {
			t = &reviewTally{key: rec.ReviewKey}
			tallies[key] = t
			order = append(order, key)
		}
		t.rounds = max(t.rounds, rec.Rounds)
		t.comments += rec.Comments
		if rec.Approved {
			approved[key] = true
			if started, err1 := time.Parse(time.RFC3339, rec.StartedAt); err1 == nil {
				if at, err2 := time.Parse(time.RFC3339, rec.ApprovedAt); err2 == nil {
					approveSeconds = append(approveSeconds, at.Sub(started).Seconds())
				}
			}
		}
	}
	out.Reviews, out.Approved = len(order), len(approved)
	out.ApproveSeconds = quantiles(approveSeconds)
	var rounds []float64
	totalRounds, totalComments := 0, 0
	observations := []string{}
	for _, key := range order {
		t := tallies[key]
		rounds = append(rounds, float64(t.rounds))
		totalRounds += t.rounds
		totalComments += t.comments
		t.repository = "unknown"
		if t.key != "" {
			observations = append(observations, readReviewInsight(t, &out)...)
		}
	}
	out.RoundsPerReview = quantiles(rounds)
	if totalRounds > 0 {
		out.CommentsPerRound = float64(totalComments) / float64(totalRounds)
	}
	out.ByRepository = repositoryInsights(order, tallies)
	out.Rules = ruleInsights(observations, src)
	return out, nil
}

// readReviewInsight adds one review folder's human comments to out and
// returns the observation IDs its feedback captured.
func readReviewInsight(t *reviewTally, out *ReviewInsights) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	folder := filepath.Join(home, ".hunkyard", "reviews", t.key)
	var cj HunkyardJSON
	if data, err := os.ReadFile(filepath.Join(folder, "review.json")); err == nil && json.Unmarshal(data, &cj) == nil {
		if cj.CWD != "" {
			t.repository = filepath.Base(cj.CWD)
		}
		for _, c := range cj.ReviewComments {
			countHumanComment(c, out)
		}
		for _, f := range cj.Files {
			for _, c := range f.Comments {
				countHumanComment(c, out)
			}
		}
	}
	return capturedObservations(folder)
}

// countHumanComment adds one reviewer comment to the label and resolution
// counts; tool, rule and agent comments are skipped.
func countHumanComment(c Comment, out *ReviewInsights) {
	if c.Source != "" || c.AuthorKind == "agent" {
		return
	}
	label := c.Label
	if label == "" {
		label = "unlabelled"
	}
	out.ByLabel[label]++
	if c.Resolved {
		out.ByResolution.Resolved++
	} else {
		out.ByResolution.Unresolved++
	}
	if c.ClaimUnchangedRound > 0 {
		out.ByResolution.ClaimUnchanged++
	}
}

// capturedObservations lists the preference observations `feedback learn`
// captured from this review's comments.
func capturedObservations(folder string) []string {
	var state struct {
		Preferences map[string]struct {
			Outcome struct {
				ObservationID string `json:"observation_id"`
			} `json:"outcome"`
		} `json:"preferences"`
	}
	data, err := os.ReadFile(filepath.Join(folder, "feedback", "state.json"))
	if err != nil || json.Unmarshal(data, &state) != nil {
		return nil
	}
	var ids []string
	for _, p := range state.Preferences {
		if p.Outcome.ObservationID != "" {
			ids = append(ids, p.Outcome.ObservationID)
		}
	}
	return ids
}

func repositoryInsights(order []string, tallies map[string]*reviewTally) []RepositoryInsight {
	byName := map[string]*RepositoryInsight{}
	out := []RepositoryInsight{}
	for _, key := range order {
		t := tallies[key]
		r := byName[t.repository]
		if r == nil {
			r = &RepositoryInsight{Repository: t.repository}
			byName[t.repository] = r
		}
		r.Reviews++
		r.Comments += t.comments
	}
	for _, r := range byName {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reviews != out[j].Reviews {
			return out[i].Reviews > out[j].Reviews
		}
		return out[i].Repository < out[j].Repository
	})
	return out
}

func ruleInsights(observations []string, src InsightSources) []RuleInsight {
	rules := map[string]*RuleInsight{}
	if src.RulesForObservations != nil && len(observations) > 0 {
		refs := src.RulesForObservations(observations)
		for _, id := range observations {
			ref, ok := refs[id]
			if !ok {
				continue
			}
			r := rules[ref.ID]
			if r == nil {
				r = &RuleInsight{RuleID: ref.ID, Directive: ref.Directive}
				rules[ref.ID] = r
			}
			r.Comments++
		}
	}
	if src.RuleVerdicts != nil {
		for id, v := range src.RuleVerdicts() {
			r := rules[id]
			if r == nil {
				r = &RuleInsight{RuleID: id}
				rules[id] = r
			}
			r.Confirmed, r.Dismissed = v.Confirmed, v.Dismissed
		}
	}
	out := []RuleInsight{}
	for _, r := range rules {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Comments != b.Comments {
			return a.Comments > b.Comments
		}
		if a.Dismissed != b.Dismissed {
			return a.Dismissed > b.Dismissed
		}
		return a.RuleID < b.RuleID
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func quantiles(values []float64) Quantiles {
	if len(values) == 0 {
		return Quantiles{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	median := sorted[n/2]
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return Quantiles{Median: median, Max: sorted[n-1]}
}
