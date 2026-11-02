package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func sampleBatch() Batch {
	return Batch{Version: 1, ReviewID: "012345abcdef", SnapshotID: "snapshot_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Round: 1, Disposition: "comment", Comments: []Comment{{ID: "human-1", Body: "Prefer clear Go names", Path: "main.go", StartLine: 1, EndLine: 1}}}
}

func TestPublishDurableReplayAndRevisions(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	input := sampleBatch()
	first, err := store.Publish("request-one", input)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	batches, err := reopened.Pending()
	if err != nil || len(batches) != 1 || batches[0].ID != first.ID || batches[0].Comments[0].Revision != 1 {
		t.Fatalf("pending: %#v %v", batches, err)
	}
	replay, err := reopened.Publish("request-one", input)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay: %v", err)
	}
	input.Comments[0].Body = "Prefer shorter Go names"
	if _, err := store.Publish("request-one", input); err == nil {
		t.Fatal("changed replay accepted")
	}
	second, err := store.Publish("request-two", input)
	if err != nil || second.Comments[0].Revision != 2 {
		t.Fatalf("edited revision: %#v %v", second, err)
	}
	if err := store.transaction(func(state *journal) error {
		state.Preferences = map[string]preferenceCheckpoint{checkpointKey(first.ID, first.Comments[0]): {DecisionHash: digest("ignored"), Outcome: PreferenceOutcome{CommentID: first.Comments[0].ID, Revision: 1, Status: "ignored"}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Ack(first.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 || pending[0].ID != second.ID {
		t.Fatalf("ack: %#v %v", pending, err)
	}
	unchanged, err := store.Publish("request-three", input)
	if err != nil || len(unchanged.Comments) != 0 {
		t.Fatalf("unchanged human revision repeated: %#v %v", unchanged, err)
	}
}

func TestFeedbackTamperFailsClosed(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish("request", sampleBatch()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "feedback", "state.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pending(); err == nil {
		t.Fatal("corrupt journal accepted")
	}
}

func TestFeedbackRejectsUnknownPreferenceCheckpoint(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish("request", sampleBatch()); err != nil {
		t.Fatal(err)
	}
	state, err := store.read()
	if err != nil {
		t.Fatal(err)
	}
	state.Preferences = map[string]preferenceCheckpoint{"forged": {DecisionHash: digest("forged"), Outcome: PreferenceOutcome{CommentID: "unknown", Revision: 1, Status: "captured"}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicFile(filepath.Join(store.root, "state.json"), data); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pending(); err == nil {
		t.Fatal("checkpoint for unknown human evidence accepted")
	}
}

func TestPublishReanchoringIsNotHumanRevision(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := sampleBatch()
	if _, err := store.Publish("original", input); err != nil {
		t.Fatal(err)
	}
	input.Round = 2
	input.Comments[0].Path = "renamed.go"
	input.Comments[0].StartLine = 10
	input.Comments[0].EndLine = 10
	input.Comments[0].ViewID = "next-view"
	input.Comments[0].ContentID = "next-content"
	batch, err := store.Publish("reanchored", input)
	if err != nil || len(batch.Comments) != 0 {
		t.Fatalf("automatic anchor change counted as human feedback: %#v %v", batch, err)
	}
}

func TestPublishRedeliversChangedReviewState(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := sampleBatch()
	if _, err := store.Publish("r1", input); err != nil {
		t.Fatal(err)
	}
	input.Comments[0].ClaimUnchanged = true
	input.Comments[0].Label = "issue"
	input.Comments[0].Blocking = true
	second, err := store.Publish("r2", input)
	if err != nil || len(second.Comments) != 1 {
		t.Fatalf("changed review state not delivered: %#v %v", second, err)
	}
	if c := second.Comments[0]; c.Revision != 1 || !c.ClaimUnchanged || !c.Blocking {
		t.Fatalf("redelivered comment = %#v, want revision 1 with the new state", c)
	}
	if again, err := store.Publish("r3", input); err != nil || len(again.Comments) != 0 {
		t.Fatalf("unchanged comment repeated: %#v %v", again, err)
	}
	resolved := input
	resolved.Comments = nil
	if _, err := store.Publish("r4", resolved); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Publish("r5", input)
	if err != nil || len(reopened.Comments) != 1 || reopened.Comments[0].Revision != 1 {
		t.Fatalf("reopened comment not delivered: %#v %v", reopened, err)
	}
}

func TestPublishDeliversEverySummary(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := sampleBatch()
	input.Comments = []Comment{{ID: "review-summary", Body: "LGTM", Scope: "review"}}
	for _, request := range []string{"first", "second"} {
		batch, err := store.Publish(request, input)
		if err != nil || len(batch.Comments) != 1 {
			t.Fatalf("%s summary not delivered: %#v %v", request, batch, err)
		}
	}
}

func TestAckAcceptsARevisionClassifiedInAnEarlierBatch(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := sampleBatch()
	first, err := store.Publish("r1", input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.transaction(func(state *journal) error {
		state.Preferences = map[string]preferenceCheckpoint{checkpointKey(first.ID, first.Comments[0]): {DecisionHash: digest("ignored"), Outcome: PreferenceOutcome{CommentID: first.Comments[0].ID, Revision: 1, Status: "ignored"}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	input.Comments[0].ClaimUnchanged = true
	second, err := store.Publish("r2", input)
	if err != nil || len(second.Comments) != 1 {
		t.Fatalf("redelivery: %#v %v", second, err)
	}
	if err := store.Ack(second.ID); err != nil {
		t.Fatalf("ack of a redelivered, already classified revision: %v", err)
	}
}

func TestPublishAcceptsAFileOutsideTheRepository(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"/home/u/notes/plan.md", "C:/Users/u/notes/plan.md"} {
		input := sampleBatch()
		input.Comments[0].ID = fmt.Sprintf("outside-%d", i)
		input.Comments[0].Path = name
		if _, err := store.Publish(fmt.Sprintf("outside-%d", i), input); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for i, name := range []string{"/home/u/../etc/passwd", "../secret", "a\\b", "/home/u//x"} {
		input := sampleBatch()
		input.Comments[0].Path = name
		if _, err := store.Publish(fmt.Sprintf("bad-%d", i), input); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
