package feedback

import (
	"strings"
	"testing"
)

func TestValidateDecisionRequest(t *testing.T) {
	cases := []struct {
		name string
		d    DecisionRequest
		ok   bool
	}{
		{"two options", DecisionRequest{Question: "Which auth?", Options: []string{"Session", "JWT"}}, true},
		{"one option", DecisionRequest{Question: "Which auth?", Options: []string{"Session"}}, false},
		{"duplicate options", DecisionRequest{Question: "Which auth?", Options: []string{"JWT", "JWT"}}, false},
		{"empty option", DecisionRequest{Question: "Which auth?", Options: []string{"JWT", " "}}, false},
		{"free text only", DecisionRequest{Question: "Name it?", AllowOther: true}, true},
		{"free text plus one", DecisionRequest{Question: "Name it?", Options: []string{"foo"}, AllowOther: true}, true},
		{"empty question", DecisionRequest{Options: []string{"a", "b"}}, false},
		{"long question", DecisionRequest{Question: strings.Repeat("q", 2001), Options: []string{"a", "b"}}, false},
		{"long option", DecisionRequest{Question: "q", Options: []string{"a", strings.Repeat("o", 201)}}, false},
		{"too many options", DecisionRequest{Question: "q", Options: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}}, false},
		{"bad context path", DecisionRequest{Question: "q", Options: []string{"a", "b"}, Context: &DecisionContext{Path: "../etc/passwd", Line: 1}}, false},
		{"good context", DecisionRequest{Question: "q", Options: []string{"a", "b"}, Context: &DecisionContext{Path: "internal/auth.go", Line: 12}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateDecisionRequest(tc.d); (err == nil) != tc.ok {
				t.Fatalf("ValidateDecisionRequest = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestBatchWithoutDecisionsKeepsItsDigest(t *testing.T) {
	b := sampleBatch()
	before := digest(b)
	b.Decisions = nil
	if digest(b) != before {
		t.Fatal("an empty decisions field changes existing batch identities")
	}
}
