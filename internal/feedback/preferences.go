package feedback

import (
	"fmt"
	"strconv"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

type PreferenceDecision struct {
	CommentID string   `json:"comment_id"`
	Revision  int      `json:"revision"`
	Kind      string   `json:"kind"`
	Directive string   `json:"directive,omitempty"`
	Scope     string   `json:"scope,omitempty"`
	Surfaces  []string `json:"surfaces,omitempty"`
}

type PreferenceOutcome struct {
	CommentID     string `json:"comment_id"`
	Revision      int    `json:"revision"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	ObservationID string `json:"observation_id,omitempty"`
}

type PreferenceReport struct {
	BatchID  string              `json:"batch_id"`
	Outcomes []PreferenceOutcome `json:"outcomes"`
}

type preferenceCheckpoint struct {
	DecisionHash string            `json:"decision_hash"`
	Outcome      PreferenceOutcome `json:"outcome"`
}

func checkpointKey(batchID string, c Comment) string {
	return batchID + ":" + digest([]string{c.ID, strconv.Itoa(c.Revision)})
}
func completedPreference(status string) bool {
	return status == "ignored" || status == "captured" || status == "rejected"
}

//nolint:gocyclo // Each revision is captured and checkpointed in the same retryable workflow.
func (s *Store) Learn(prefs *preferences.Store, batchID string, decisions []PreferenceDecision, owner, repoRoot string) (PreferenceReport, error) {
	report := PreferenceReport{BatchID: batchID, Outcomes: []PreferenceOutcome{}}
	if prefs == nil || owner == "" || repoRoot == "" {
		return report, fmt.Errorf("missing trusted preference context")
	}
	var batch Batch
	err := s.transaction(func(state *journal) error {
		for _, p := range state.Publications {
			if p.Batch.ID == batchID {
				batch = p.Batch
				return nil
			}
		}
		return fmt.Errorf("feedback batch not found")
	})
	if err != nil {
		return report, err
	}
	if len(decisions) != len(batch.Comments) {
		return report, fmt.Errorf("classify every published human comment revision exactly once")
	}
	known := map[string]Comment{}
	for _, c := range batch.Comments {
		known[c.ID] = c
	}
	classified := map[string]PreferenceDecision{}
	for _, decision := range decisions {
		c, exists := known[decision.CommentID]
		if !exists || c.Revision != decision.Revision || classified[c.ID].Kind != "" {
			return report, fmt.Errorf("unknown, duplicate or outdated comment decision")
		}
		switch decision.Kind {
		case "ignore", "candidate":
			if decision.Directive != "" || decision.Scope != "" || len(decision.Surfaces) > 0 {
				return report, fmt.Errorf("only directive decisions may set directive, scope or surfaces")
			}
		case "directive":
			if decision.Directive == "" || (decision.Scope != "global" && decision.Scope != "repository") || len(decision.Surfaces) == 0 {
				return report, fmt.Errorf("a directive requires explicit scope and surfaces")
			}
		default:
			return report, fmt.Errorf("invalid preference decision kind")
		}
		classified[c.ID] = decision
	}
	failed := 0
	for _, c := range batch.Comments {
		decision := classified[c.ID]
		var outcome PreferenceOutcome
		err := s.transaction(func(state *journal) error {
			if state.Preferences == nil {
				state.Preferences = map[string]preferenceCheckpoint{}
			}
			key := checkpointKey(batch.ID, c)
			hash := digest(decision)
			if previous, exists := state.Preferences[key]; exists {
				if previous.DecisionHash != hash {
					return fmt.Errorf("this human revision already has a different preference classification")
				}
				if completedPreference(previous.Outcome.Status) {
					outcome = previous.Outcome
					return nil
				}
			}
			outcome = PreferenceOutcome{CommentID: c.ID, Revision: c.Revision, Status: "ignored"}
			if decision.Kind != "ignore" {
				// The identity excludes batch/round so publishing an unchanged revision cannot inflate evidence.
				turn := digest([]string{batch.ReviewID, c.ID, strconv.Itoa(c.Revision), revisionHash(c)})
				result, err := prefs.Capture(preferences.CaptureInput{Feedback: c.Body, Directive: decision.Directive, Scope: decision.Scope, Surfaces: decision.Surfaces, CWD: repoRoot, Source: "review", SessionID: owner, TurnID: turn, Explicit: decision.Kind == "directive", CodeContext: &preferences.CodeContext{Path: c.Path, StartLine: c.StartLine, EndLine: c.EndLine, Excerpt: c.Quote}})
				switch {
				case err != nil:
					outcome.Status = "failed"
					outcome.Reason = "capture_failed"
				case result.Reason == "sensitive_content":
					outcome.Status = "rejected"
					outcome.Reason = result.Reason
				case result.Action == "ignored":
					outcome.Status = "ignored"
					outcome.Reason = result.Reason
				default:
					outcome.Status = "captured"
					outcome.ObservationID = result.ID
					outcome.Reason = result.Reason
				}
				if s.beforePreferenceCheckpoint != nil {
					if err := s.beforePreferenceCheckpoint(); err != nil {
						return err
					}
				}
			}
			state.Preferences[key] = preferenceCheckpoint{DecisionHash: hash, Outcome: outcome}
			return nil
		})
		if err != nil {
			return report, err
		}
		report.Outcomes = append(report.Outcomes, outcome)
		if outcome.Status == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return report, fmt.Errorf("preference capture failed for %d comment revisions; retry the same decisions", failed)
	}
	return report, nil
}
