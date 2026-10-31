package session

import "testing"

func claimFixture(current string, replies []Reply) (*Session, *FileEntry) {
	s := &Session{ReviewRound: 1}
	f := &FileEntry{
		Path:            "app.rb",
		PreviousContent: "a\ndef x; 1; end\nc\n",
		Content:         current,
		PreviousComments: []Comment{{
			ID: "c1", StartLine: 2, EndLine: 2, Body: "multi-line please", Anchor: "def x; 1; end",
			Resolved: true, ResolvedRound: 1, Replies: replies,
		}},
	}
	return s, f
}

func TestClaimedFixedWithUnchangedCodeIsFlaggedAndReopened(t *testing.T) {
	s, f := claimFixture("a\ndef x; 1; end\nc\n", []Reply{{ID: "r1", AuthorKind: "agent", Body: "Fixed.", ReviewRound: 1}})
	s.carryForwardFileComments(f)
	c := f.Comments[0]
	if c.ClaimUnchangedRound != 1 || c.Resolved {
		t.Fatalf("unchanged claim not flagged or not reopened: %+v", c)
	}
}

func TestClaimedFixedWithChangedCodeIsNotFlagged(t *testing.T) {
	s, f := claimFixture("a\ndef x\n  1\nend\nc\n", []Reply{{ID: "r1", AuthorKind: "agent", Body: "Fixed.", ReviewRound: 1}})
	s.carryForwardFileComments(f)
	if c := f.Comments[0]; c.ClaimUnchangedRound != 0 || !c.Resolved {
		t.Fatalf("real fix flagged: %+v", c)
	}
}

func TestNoAgentReplyIsNotFlagged(t *testing.T) {
	s, f := claimFixture("a\ndef x; 1; end\nc\n", []Reply{{ID: "r1", AuthorKind: "human", Body: "still?", ReviewRound: 1}})
	s.carryForwardFileComments(f)
	if c := f.Comments[0]; c.ClaimUnchangedRound != 0 {
		t.Fatalf("flagged without an agent claim: %+v", c)
	}
}

func TestAgentReplyWithoutFixClaimIsNotFlagged(t *testing.T) {
	for _, tt := range []struct{ name, label, body string }{
		{"explanation", "", "This is intentional: the one-liner keeps the DSL readable."},
		{"question label", "question", "Fixed."},
		{"praise label", "praise", "Done."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, f := claimFixture("a\ndef x; 1; end\nc\n", []Reply{{ID: "r1", AuthorKind: "agent", Body: tt.body, ReviewRound: 1}})
			f.PreviousComments[0].Label = tt.label
			s.carryForwardFileComments(f)
			if c := f.Comments[0]; c.ClaimUnchangedRound != 0 || !c.Resolved {
				t.Fatalf("non-claim flagged: %+v", c)
			}
		})
	}
}

func TestClaimSeesRepliesPostedAfterTheRoundSnapshot(t *testing.T) {
	s, f := claimFixture("a\ndef x; 1; end\nc\n", nil)
	current := f.PreviousComments[0]
	current.Replies = []Reply{{ID: "r1", AuthorKind: "agent", Body: "Fixed.", ReviewRound: 1}}
	f.Comments = []Comment{current}
	s.carryForwardFileComments(f)
	if len(f.Comments) != 1 || f.Comments[0].ClaimUnchangedRound != 1 || len(f.Comments[0].Replies) != 1 {
		t.Fatalf("reply after the snapshot lost: %+v", f.Comments)
	}
}
