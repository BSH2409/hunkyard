package session

import (
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestLiveSessionWritesItsReviewTypeAndOrigin(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "review-key")
	s, err := NewLiveSession("http://localhost:3000", identity)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteFiles()
	cj, err := readHunkyardJSONFromDisk(identity)
	if err != nil {
		t.Fatal(err)
	}
	if cj.ReviewType != "live" || cj.Origin != "http://localhost:3000" {
		t.Fatalf("review_type = %q, origin = %q; want live and the app origin", cj.ReviewType, cj.Origin)
	}
}

func TestLiveSessionRestartKeepsDecisionsAndReviewComments(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "review-key")
	if err := saveHunkyardJSONToDisk(identity, HunkyardJSON{
		ReviewType:     "live",
		Origin:         "http://localhost:3000",
		ReviewComments: []Comment{{ID: "r1", Body: "overall"}},
		Decisions:      []feedback.DecisionRequest{{ID: "d1", Question: "Which?", Options: []string{"A", "B"}, Blocking: true, Status: "open"}},
		Files:          map[string]HunkyardJSONFile{},
	}); err != nil {
		t.Fatal(err)
	}
	s, err := NewLiveSession("http://localhost:3000", identity)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Decisions(); len(got) != 1 || got[0].ID != "d1" {
		t.Errorf("decisions after restart = %+v", got)
	}
	if got := s.GetReviewComments(); len(got) != 1 {
		t.Errorf("review comments after restart = %+v", got)
	}
}
