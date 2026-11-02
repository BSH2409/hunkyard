package session

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func askFixture(t *testing.T) *Session {
	t.Helper()
	s := NewTestSession(t)
	s.ReviewFilePath = filepath.Join(t.TempDir(), "review")
	return s
}

func TestAddDecisionAssignsIDAndRound(t *testing.T) {
	s := askFixture(t)
	s.ReviewRound = 2
	d, err := s.AddDecision(feedback.DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}, Status: "answered", Chosen: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ID) != 8 || d.ID[:2] != "d_" || d.Status != "open" || d.AskedRound != 2 || d.AskedAt == "" || d.Chosen != "" {
		t.Fatalf("decision = %+v", d)
	}
	if again, _ := s.AddDecision(feedback.DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}}); again.ID == d.ID {
		t.Fatal("asking the same question twice reused the request")
	}
	if len(s.Decisions()) != 2 {
		t.Fatalf("decisions = %d", len(s.Decisions()))
	}
}

func TestAddDecisionRejectsInvalidRequest(t *testing.T) {
	s := askFixture(t)
	if _, err := s.AddDecision(feedback.DecisionRequest{Question: "q", Options: []string{"only"}}); err == nil {
		t.Fatal("one-option question accepted")
	}
}

func TestAnswerDecision(t *testing.T) {
	s := askFixture(t)
	closed, _ := s.AddDecision(feedback.DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}})
	open, _ := s.AddDecision(feedback.DecisionRequest{Question: "Name?", AllowOther: true})
	cases := []struct {
		name, id, chosen, text string
		dismiss                bool
		want                   error
	}{
		{"unknown option", closed.ID, "OAuth", "", false, ErrInvalidAnswer},
		{"empty answer", closed.ID, "", "", false, ErrInvalidAnswer},
		{"unknown decision", "d_000000", "JWT", "", false, ErrDecisionNotFound},
		{"choose", closed.ID, "JWT", "", false, nil},
		{"answer twice", closed.ID, "Session", "", false, ErrDecisionAnswered},
		{"free text", open.ID, "", "Use the auth service", false, nil},
	}
	for _, tc := range cases {
		if _, err := s.AnswerDecision(tc.id, tc.chosen, tc.text, tc.dismiss); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	got := s.Decisions()
	if got[0].Status != "answered" || got[0].Chosen != "JWT" || got[0].AnsweredAt == "" || got[1].Text != "Use the auth service" {
		t.Fatalf("answers = %+v", got)
	}
	written, _ := s.AddDecision(feedback.DecisionRequest{Question: "Which store?", Options: []string{"Redis", "Postgres"}})
	if d, err := s.AnswerDecision(written.ID, "", "SQLite for now", false); err != nil || d.Text != "SQLite for now" {
		t.Fatalf("a written answer to an options question = %+v %v", d, err)
	}
	third, _ := s.AddDecision(feedback.DecisionRequest{Question: "Keep it?", Options: []string{"yes", "no"}})
	if d, err := s.AnswerDecision(third.ID, "", "", true); err != nil || d.Status != "dismissed" {
		t.Fatalf("dismiss = %+v %v", d, err)
	}
}

func TestOpenBlockingDecisions(t *testing.T) {
	s := askFixture(t)
	a, _ := s.AddDecision(feedback.DecisionRequest{Question: "a?", Options: []string{"x", "y"}, Blocking: true})
	_, _ = s.AddDecision(feedback.DecisionRequest{Question: "b?", Options: []string{"x", "y"}})
	if n := s.OpenBlockingDecisions(); n != 1 {
		t.Fatalf("open blocking = %d", n)
	}
	_, _ = s.AnswerDecision(a.ID, "x", "", false)
	if n := s.OpenBlockingDecisions(); n != 0 {
		t.Fatalf("open blocking after answer = %d", n)
	}
}

func TestDecisionsToPublishAndMarkPublished(t *testing.T) {
	s := askFixture(t)
	a, _ := s.AddDecision(feedback.DecisionRequest{Question: "a?", Options: []string{"x", "y"}})
	b, _ := s.AddDecision(feedback.DecisionRequest{Question: "b?", Options: []string{"x", "y"}})
	_, _ = s.AnswerDecision(a.ID, "x", "", false)
	got := s.DecisionsToPublish()
	if len(got) != 2 || got[0].ID != a.ID || got[1].Status != "open" {
		t.Fatalf("to publish = %+v", got)
	}
	s.MarkDecisionsPublished("batch_1", got)
	got = s.DecisionsToPublish()
	if len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("answered decision published twice, or open one dropped: %+v", got)
	}
}

func TestDecisionsRoundTripReviewFile(t *testing.T) {
	s := askFixture(t)
	d, _ := s.AddDecision(feedback.DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}, Context: &feedback.DecisionContext{Path: "main.go", Line: 3}})
	_, _ = s.AnswerDecision(d.ID, "JWT", "", false)
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	reloaded := askFixture(t)
	reloaded.ReviewFilePath = s.ReviewFilePath
	reloaded.loadHunkyardJSON()
	got := reloaded.Decisions()
	if len(got) != 1 || got[0].Chosen != "JWT" || got[0].Context == nil || got[0].Context.Line != 3 {
		t.Fatalf("reloaded = %+v", got)
	}
}

func TestClearAllCommentsClearsDecisions(t *testing.T) {
	s := askFixture(t)
	_, _ = s.AddDecision(feedback.DecisionRequest{Question: "a?", Options: []string{"x", "y"}})
	s.ClearAllComments()
	if len(s.Decisions()) != 0 {
		t.Fatal("decisions survived clearing")
	}
}

// A decision asked in round 2 and answered in round 3, with the daemon
// paused and resumed in between, is published once.
func TestDecisionAcrossRoundsAndResumeIsPublishedOnce(t *testing.T) {
	s := askFixture(t)
	s.ReviewRound = 2
	d, _ := s.AddDecision(feedback.DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}})
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	resumed := askFixture(t) // the daemon paused; `hunkyard --session` started a new one
	resumed.ReviewFilePath = s.ReviewFilePath
	resumed.loadHunkyardJSON()
	resumed.ReviewRound = 3
	if _, err := resumed.AnswerDecision(d.ID, "JWT", "", false); err != nil {
		t.Fatal(err)
	}
	if err := resumed.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	again := askFixture(t)
	again.ReviewFilePath = s.ReviewFilePath
	again.loadHunkyardJSON()
	first := again.DecisionsToPublish()
	if len(first) != 1 || first[0].Chosen != "JWT" || first[0].AskedRound != 2 {
		t.Fatalf("round 3 batch = %+v", first)
	}
	again.MarkDecisionsPublished("batch_round3", first)
	if err := again.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	later := askFixture(t)
	later.ReviewFilePath = s.ReviewFilePath
	later.loadHunkyardJSON()
	if got := later.DecisionsToPublish(); len(got) != 0 {
		t.Fatalf("answered decision published again after a resume: %+v", got)
	}
}
