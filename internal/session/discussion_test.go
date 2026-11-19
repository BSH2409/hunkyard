package session

import "testing"

func TestAgentReviewReplyPreservesProvenanceAndIdentity(t *testing.T) {
	s := &Session{reviewComments: []Comment{{ID: "r_parent", Scope: "review"}}, ReviewRound: 2}
	t.Cleanup(s.QuiesceForTest)
	first, ok := s.AddAgentReviewReply("r_parent", "rp_agent", "Naming advice", "Claude")
	if !ok || first.AuthorKind != "agent" {
		t.Fatalf("agent provenance: %+v", first)
	}
	again, ok := s.AddAgentReviewReply("r_parent", "rp_agent", "duplicate", "Claude")
	if !ok || first.ID != again.ID || len(s.reviewComments[0].Replies) != 1 {
		t.Fatal("duplicate agent reply")
	}
	if _, ok := s.AddAgentReviewReply("removed", "rp_none", "reply", "Codex"); ok {
		t.Fatal("reply attached to a removed message")
	}
}
