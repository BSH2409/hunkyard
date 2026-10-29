package feedback

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

func TestReviewCandidateHonorsHostClassificationWithoutPromotion(t *testing.T) {
	store, prefs, _, repo, root := preferenceFixture(t)
	input := sampleBatch()
	input.Comments[0].Body = "Clarify responsibility at this boundary."
	batch, err := store.Publish("candidate-without-keywords", input)
	if err != nil {
		t.Fatal(err)
	}
	decisions := []PreferenceDecision{{CommentID: batch.Comments[0].ID, Revision: batch.Comments[0].Revision, Kind: "candidate"}}
	report, err := store.Learn(prefs, batch.ID, decisions, "owner", repo)
	if err != nil || report.Outcomes[0].Status != "captured" {
		t.Fatalf("host classification was discarded: %#v %v", report, err)
	}
	pending := filepath.Join(root, "learn-preferences", "observations", "pending.jsonl")
	before, err := os.ReadFile(pending)
	if err != nil {
		t.Fatal(err)
	}
	var observation map[string]any
	if json.Unmarshal(before, &observation) != nil {
		t.Fatal("candidate missing")
	}
	signals, _ := observation["signals"].(map[string]any)
	if signals["durable_language"] != false {
		t.Fatal("candidate supplied durable intent")
	}
	if _, err := os.Lstat(filepath.Join(root, "learn-preferences", "current")); !os.IsNotExist(err) {
		t.Fatal("candidate created a generation")
	}
	if _, err := store.Learn(prefs, batch.ID, decisions, "owner", repo); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(pending)
	if err != nil || string(before) != string(after) {
		t.Fatal("candidate replay duplicated evidence", err)
	}
}

func preferenceFixture(t *testing.T) (*Store, *preferences.Store, Batch, string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "reviewed-repository")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	store, err := Open(filepath.Join(root, "review"))
	if err != nil {
		t.Fatal(err)
	}
	prefsRoot := filepath.Join(root, "codex")
	prefs, err := preferences.Open(preferences.Config{Root: prefsRoot})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := store.Publish("request", sampleBatch())
	if err != nil {
		t.Fatal(err)
	}
	return store, prefs, batch, repo, prefsRoot
}

func TestReviewPreferenceDecisionsRequireEveryRevision(t *testing.T) {
	store, prefs, batch, repo, _ := preferenceFixture(t)
	if _, err := store.Learn(prefs, batch.ID, nil, "owner", repo); err == nil {
		t.Fatal("missing decision accepted")
	}
	decision := PreferenceDecision{CommentID: batch.Comments[0].ID, Revision: 1, Kind: "ignore"}
	if _, err := store.Learn(prefs, batch.ID, []PreferenceDecision{decision, decision}, "owner", repo); err == nil {
		t.Fatal("duplicate decision accepted")
	}
	decision.CommentID = "unknown"
	if _, err := store.Learn(prefs, batch.ID, []PreferenceDecision{decision}, "owner", repo); err == nil {
		t.Fatal("unknown decision accepted")
	}
	if err := store.Ack(batch.ID); err == nil {
		t.Fatal("unclassified review acknowledged")
	}
	decision.CommentID = batch.Comments[0].ID
	report, err := store.Learn(prefs, batch.ID, []PreferenceDecision{decision}, "owner", repo)
	if err != nil || report.Outcomes[0].Status != "ignored" {
		t.Fatalf("ignore: %#v %v", report, err)
	}
	if err := store.Ack(batch.ID); err != nil {
		t.Fatal(err)
	}
}

func TestReviewPreferenceReplayAndRepositoryScope(t *testing.T) {
	store, prefs, batch, repo, root := preferenceFixture(t)
	decisions := []PreferenceDecision{{CommentID: batch.Comments[0].ID, Revision: 1, Kind: "directive", Directive: "Prefer clear Go names", Scope: "repository", Surfaces: []string{"naming"}}}
	store.beforePreferenceCheckpoint = func() error { return errors.New("simulated crash after capture") }
	if _, err := store.Learn(prefs, batch.ID, decisions, "owner", repo); err == nil {
		t.Fatal("checkpoint interruption ignored")
	}
	store.beforePreferenceCheckpoint = nil
	report, err := store.Learn(prefs, batch.ID, decisions, "owner", repo)
	if err != nil || report.Outcomes[0].Status != "captured" {
		t.Fatalf("retry: %#v %v", report, err)
	}
	before, err := os.ReadFile(filepath.Join(root, "learn-preferences", "current", "rules.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "reviewed-repository-") || strings.Contains(string(before), "hunkyard-") {
		t.Fatal("preference attached to wrong repository")
	}
	if _, err := store.Learn(prefs, batch.ID, decisions, "owner", repo); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, "learn-preferences", "current", "rules.yml"))
	if err != nil || string(before) != string(after) {
		t.Fatal("replay inflated rule evidence")
	}
}

func TestReviewPreferencePrivacy(t *testing.T) {
	store, prefs, _, repo, root := preferenceFixture(t)
	input := sampleBatch()
	input.Comments[0].Body = "Always use password=synthetic-only in Go"
	batch, err := store.Publish("sensitive", input)
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Learn(prefs, batch.ID, []PreferenceDecision{{CommentID: batch.Comments[0].ID, Revision: 2, Kind: "candidate"}}, "owner", repo)
	if err != nil || report.Outcomes[0].Status != "rejected" {
		t.Fatalf("privacy: %#v %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(root, "learn-preferences", "observations", "pending.jsonl")); !os.IsNotExist(err) {
		t.Fatal("sensitive comment entered preference evidence")
	}
}

func TestReviewPreferencePartialFailureResumes(t *testing.T) {
	store, prefs, _, repo, root := preferenceFixture(t)
	input := sampleBatch()
	input.Comments[0].Body = "A one-off fix"
	input.Comments = append(input.Comments, Comment{ID: "second", Body: "Always prefer clear names", Path: "main.go"})
	batch, err := store.Publish("partial", input)
	if err != nil {
		t.Fatal(err)
	}
	decisions := []PreferenceDecision{{CommentID: "human-1", Revision: 2, Kind: "ignore"}, {CommentID: "second", Revision: 1, Kind: "candidate"}}
	pending := filepath.Join(root, "learn-preferences", "observations", "pending.jsonl")
	if err := os.MkdirAll(pending, 0700); err != nil {
		t.Fatal(err)
	}
	report, err := store.Learn(prefs, batch.ID, decisions, "owner", repo)
	if err == nil || len(report.Outcomes) != 2 || report.Outcomes[0].Status != "ignored" || report.Outcomes[1].Status != "failed" {
		t.Fatalf("partial: %#v %v", report, err)
	}
	if err := os.Remove(pending); err != nil {
		t.Fatal(err)
	}
	report, err = store.Learn(prefs, batch.ID, decisions, "owner", repo)
	if err != nil || report.Outcomes[0].Status != "ignored" || report.Outcomes[1].Status != "captured" {
		t.Fatalf("retry: %#v %v", report, err)
	}
}
