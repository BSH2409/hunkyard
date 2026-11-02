package session

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

var (
	ErrDecisionNotFound = errors.New("decision not found")
	ErrDecisionAnswered = errors.New("decision already answered")
	ErrInvalidAnswer    = errors.New("answer must be one of the options or a written answer")
)

// AddDecision stores a new open decision request for this round.
func (s *Session) AddDecision(d feedback.DecisionRequest) (feedback.DecisionRequest, error) {
	if err := feedback.ValidateDecisionRequest(d); err != nil {
		return feedback.DecisionRequest{}, err
	}
	s.mu.Lock()
	d.ID = randomID("d_")
	d.Status = "open"
	d.Chosen, d.Text, d.AnsweredAt, d.PublishedBatch = "", "", "", ""
	d.AskedRound = s.ReviewRound
	d.AskedAt = time.Now().UTC().Format(time.RFC3339)
	d.Options = slices.Clone(d.Options)
	s.decisions = append(s.decisions, d)
	s.scheduleWrite()
	s.mu.Unlock()
	s.notify(SSEEvent{Type: "decisions-changed"})
	return d, nil
}

// AnswerDecision records the reviewer's choice, free-text answer, or
// dismissal of an open decision.
func (s *Session) AnswerDecision(id, chosen, text string, dismiss bool) (feedback.DecisionRequest, error) {
	s.mu.Lock()
	i := slices.IndexFunc(s.decisions, func(d feedback.DecisionRequest) bool { return d.ID == id })
	if i < 0 {
		s.mu.Unlock()
		return feedback.DecisionRequest{}, ErrDecisionNotFound
	}
	d := &s.decisions[i]
	if d.Status != "open" {
		s.mu.Unlock()
		return feedback.DecisionRequest{}, ErrDecisionAnswered
	}
	text = strings.TrimSpace(text)
	switch {
	case dismiss:
		d.Status = "dismissed"
	case chosen != "" && slices.Contains(d.Options, chosen) && len(text) <= 4000:
		d.Status, d.Chosen, d.Text = "answered", chosen, text
	case chosen == "" && text != "" && len(text) <= 4000:
		// Like Claude Code's own questions, every question takes a written
		// answer as well as its options.
		d.Status, d.Text = "answered", text
	default:
		s.mu.Unlock()
		return feedback.DecisionRequest{}, ErrInvalidAnswer
	}
	d.AnsweredAt = time.Now().UTC().Format(time.RFC3339)
	answered := *d
	s.scheduleWrite()
	s.mu.Unlock()
	s.notify(SSEEvent{Type: "decisions-changed"})
	return answered, nil
}

// Decisions returns every decision request of the review, oldest first.
func (s *Session) Decisions() []feedback.DecisionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneDecisions(s.decisions)
}

// OpenBlockingDecisions counts blocking requests the reviewer has not
// answered; Approve waits for them like blocking comments.
func (s *Session) OpenBlockingDecisions() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, d := range s.decisions {
		if d.Blocking && d.Status == "open" {
			n++
		}
	}
	return n
}

// DecisionsToPublish returns the requests the next batch carries: answers
// not yet published, and every request still open.
func (s *Session) DecisionsToPublish() []feedback.DecisionRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []feedback.DecisionRequest
	for _, d := range s.decisions {
		if d.Status == "open" || d.PublishedBatch == "" {
			out = append(out, cloneDecision(d))
		}
	}
	return out
}

// MarkDecisionsPublished records that batchID carried these answers, so the
// next batch does not repeat them. Open requests stay unpublished.
func (s *Session) MarkDecisionsPublished(batchID string, published []feedback.DecisionRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range published {
		if p.Status == "open" {
			continue
		}
		for i := range s.decisions {
			if s.decisions[i].ID == p.ID {
				s.decisions[i].PublishedBatch = batchID
			}
		}
	}
	s.scheduleWrite()
}

func cloneDecision(d feedback.DecisionRequest) feedback.DecisionRequest {
	d.Options = slices.Clone(d.Options)
	if d.Context != nil {
		c := *d.Context
		d.Context = &c
	}
	return d
}

func cloneDecisions(in []feedback.DecisionRequest) []feedback.DecisionRequest {
	if len(in) == 0 {
		return nil
	}
	out := make([]feedback.DecisionRequest, len(in))
	for i, d := range in {
		out[i] = cloneDecision(d)
	}
	return out
}
