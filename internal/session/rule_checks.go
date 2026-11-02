package session

import (
	"errors"
	"log"
	"time"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

// RuleChecker runs the learned-preference diff checks over a review's added
// lines.
type RuleChecker func(lines []preferences.AddedLine) []preferences.DiffFinding

var (
	ErrInvalidVerdict  = errors.New("verdict must be confirmed or dismissed")
	ErrCommentNotFound = errors.New("comment not found")
	ErrNotRuleFinding  = errors.New("comment is not a rule finding")
)

// SetRuleChecker installs the checker RunRuleChecks uses.
func (s *Session) SetRuleChecker(fn RuleChecker) {
	s.mu.Lock()
	s.ruleChecker = fn
	s.mu.Unlock()
}

// SetRuleVerdictRecorder installs the callback that records a reviewer's
// verdict as preference evidence.
func (s *Session) SetRuleVerdictRecorder(fn func(ruleID, verdict string) error) {
	s.mu.Lock()
	s.ruleVerdictRecorder = fn
	s.mu.Unlock()
}

// RunRuleChecks posts one comment per rule finding on the lines this review
// adds and removes findings that no longer apply. A finding the reviewer
// already answered keeps its thread, resolution and verdict. Files whose
// diff has not loaded yet are skipped until a later pass.
func (s *Session) RunRuleChecks() ReplaceResult {
	s.mu.RLock()
	checker := s.ruleChecker
	var lines []preferences.AddedLine
	for _, f := range s.Files {
		for _, h := range f.DiffHunks {
			for _, l := range h.Lines {
				if l.Type == "add" && l.NewNum > 0 {
					lines = append(lines, preferences.AddedLine{Path: f.Path, Number: l.NewNum, Text: l.Content})
				}
			}
		}
	}
	s.mu.RUnlock()
	if checker == nil {
		return ReplaceResult{}
	}
	findings := checker(lines)
	incoming := make([]MachineComment, 0, len(findings))
	for _, f := range findings {
		incoming = append(incoming, MachineComment{Path: f.Path, Line: f.Line, EndLine: f.Line, Body: "Likely violates: " + f.Directive + "\n" + f.Message, Author: "hunkyard", SourceRef: f.RuleID})
	}
	return s.ReplaceMachineComments("rule", "", incoming)
}

// SetRuleVerdict records the reviewer's verdict on a rule finding. A
// dismissal also resolves it; a confirmation leaves it open for the agent.
func (s *Session) SetRuleVerdict(filePath, id, verdict string) (Comment, error) {
	if verdict != "confirmed" && verdict != "dismissed" {
		return Comment{}, ErrInvalidVerdict
	}
	s.mu.Lock()
	f, i := s.locateFileCommentLocked(filePath, id)
	if f == nil {
		s.mu.Unlock()
		return Comment{}, ErrCommentNotFound
	}
	c := &f.Comments[i]
	if c.Source != "rule" {
		s.mu.Unlock()
		return Comment{}, ErrNotRuleFinding
	}
	c.Verdict = verdict
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if verdict == "dismissed" {
		c.Resolved = true
		c.ResolvedRound = s.ReviewRound
	}
	updated := *c
	recorder := s.ruleVerdictRecorder
	s.scheduleWrite()
	s.mu.Unlock()
	if recorder != nil {
		if err := recorder(updated.SourceRef, verdict); err != nil {
			log.Printf("Warning: recording the rule verdict: %v", err)
		}
	}
	s.notify(SSEEvent{Type: "comments-changed"})
	return updated, nil
}
