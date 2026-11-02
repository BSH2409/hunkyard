package feedback

import (
	"fmt"
	"path"
	"strings"
)

// DecisionContext points a decision request at the code it is about.
type DecisionContext struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// DecisionRequest is a question the agent asks the reviewer through
// `hunkyard ask`. The reviewer picks an option, writes an answer (when
// AllowOther), or dismisses it; the answer is published with the next batch.
// Decisions are never preference evidence.
type DecisionRequest struct {
	ID             string           `json:"id"`
	ParentID       string           `json:"parent_id,omitempty"`
	Question       string           `json:"question"`
	Options        []string         `json:"options"`
	AllowOther     bool             `json:"allow_other,omitempty"`
	Blocking       bool             `json:"blocking,omitempty"`
	Context        *DecisionContext `json:"context,omitempty"`
	Status         string           `json:"status"` // open | answered | dismissed
	Chosen         string           `json:"chosen,omitempty"`
	Text           string           `json:"text,omitempty"`
	AskedRound     int              `json:"asked_round"`
	AskedAt        string           `json:"asked_at"`
	AnsweredAt     string           `json:"answered_at,omitempty"`
	PublishedBatch string           `json:"published_batch,omitempty"`
}

const (
	maxDecisionQuestion = 2000
	maxDecisionOption   = 200
	maxDecisionOptions  = 10
)

// ValidateDecisionRequest checks what the agent may ask: a question, two to
// ten distinct options (or free text when AllowOther), and a context path
// inside the repository.
func ValidateDecisionRequest(d DecisionRequest) error {
	q := strings.TrimSpace(d.Question)
	if q == "" || len(d.Question) > maxDecisionQuestion {
		return fmt.Errorf("a decision needs a question of at most %d characters", maxDecisionQuestion)
	}
	if len(d.Options) > maxDecisionOptions {
		return fmt.Errorf("a decision offers at most %d options", maxDecisionOptions)
	}
	if len(d.Options) < 2 && !d.AllowOther {
		return fmt.Errorf("a decision needs at least two options, or --allow-other")
	}
	seen := map[string]bool{}
	for _, o := range d.Options {
		trimmed := strings.TrimSpace(o)
		if trimmed == "" || len(o) > maxDecisionOption || seen[trimmed] {
			return fmt.Errorf("decision options must be distinct, non-empty and at most %d characters", maxDecisionOption)
		}
		seen[trimmed] = true
	}
	if c := d.Context; c != nil && (!repoRelative(c.Path) || c.Line < 0) {
		return fmt.Errorf("invalid decision context")
	}
	return nil
}

// repoRelative reports whether p is a clean path inside the repository.
func repoRelative(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00")
}
