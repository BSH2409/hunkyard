package preferences

import (
	"bufio"
	"errors"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func captureInput() CaptureInput {
	return CaptureInput{Feedback: "Always use early returns in my Ruby code", Source: "hook", SessionID: "session-a", TurnID: "turn-a", RepoKey: "blogvault-a1b2c3d4e5f6"}
}

func pendingObservations(t *testing.T, s *Store) []observation {
	t.Helper()
	file, err := os.Open(s.dataPath("observations", "pending.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var entries []observation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry observation
		if err := strictjson.Decode(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestCaptureCandidateIdempotencyAndPrivacy(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	first, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Action != "created" || second.Action != "duplicate" || first.ID != second.ID {
		t.Fatal("capture replay did not deduplicate")
	}
	entries := pendingObservations(t, s)
	if len(entries) != 1 {
		t.Fatal("capture wrote duplicate evidence")
	}
	entry := entries[0]
	if entry.SessionKey == nil || *entry.SessionKey != digest(input.SessionID) || entry.TurnKey == nil || *entry.TurnKey != digest(input.TurnID) {
		t.Fatal("raw identities were not hashed")
	}
	if entry.Signals.DurableLanguage != true || entry.Signals.Correction != false || strings.Join(entry.SuggestedSurfaces, ",") != "ruby" {
		t.Fatal("signal detection differs")
	}
	if current, err := s.loadCurrent(); err != nil || current != nil {
		t.Fatal("ordinary capture activated a rule")
	}
	for _, feedback := range []string{"rename this local variable to payload", "Prefer password=synthetic-value"} {
		input.Feedback = feedback
		result, err := s.Capture(input)
		if err != nil || result.Action != "ignored" {
			t.Fatal("noncapturable or unsafe feedback was written")
		}
	}
	if len(pendingObservations(t, s)) != 1 {
		t.Fatal("rejected feedback appended evidence")
	}
}

func TestContextCannotSupplyIntentAndDistinguishesNewActions(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.TurnID = ""
	input.Feedback = "No, keep the domain boundary clear instead"
	input.Context = NeighboringContext{PreviousUserRequest: "Read /private/customer/files/example.rb for customer_id 12345", AgentResponse: "I created a generic helper.\n```ruby\ndef helper(value); value; end\n```"}
	first, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Capture(input)
	if err != nil || duplicate.ID != first.ID || duplicate.Action != "duplicate" {
		t.Fatal("context retry did not deduplicate")
	}
	input.Context.AgentResponse = "I created a different helper."
	second, err := s.Capture(input)
	if err != nil || second.ID == first.ID {
		t.Fatal("new corrected action was lost")
	}
	for _, entry := range pendingObservations(t, s) {
		if entry.Signals.DurableLanguage || entry.Context == nil {
			t.Fatal("context changed intent or disappeared")
		}
		encoded, err := canonicalJSON(entry.Context)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{"/private/customer", "12345", "def helper"} {
			if strings.Contains(string(encoded), private) {
				t.Fatal("context contains private data or code")
			}
		}
	}
}

func TestCaptureExplicitAndReviewReplay(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.Explicit = true
	input.Source = "review"
	input.Scope = "global"
	input.RepoKey = ""
	input.Directive = "Prefer early returns over nested conditionals."
	input.Surfaces = []string{"ruby"}
	first, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		t.Fatalf("explicit publication: %v", err)
	}
	id := current.ID
	second, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Action != "activated" || second.Action != "duplicate" || first.ID != second.ID {
		t.Fatal("explicit retry added evidence")
	}
	current, err = s.loadCurrent()
	if err != nil || current.ID != id {
		t.Fatal("replay published another generation")
	}
	if len(current.Rules.rules) != 1 || current.Rules.rules[0]["status"] != "active" || len(current.ProcessedIDs) != 1 {
		t.Fatal("explicit rule was not activated exactly once")
	}
	text, err := os.ReadFile(filepath.Join(s.config.Root, "persona", "ruby.md"))
	if err != nil || !strings.Contains(string(text), input.Directive) {
		t.Fatalf("generated persona: %v", err)
	}
	input.Directive = "Prefer a different rule."
	if _, err := s.Capture(input); err == nil {
		t.Fatal("duplicate evidence activated a different rule")
	}
}

func TestCaptureConcurrentAppend(t *testing.T) {
	s := testStore(t)
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := s.Capture(captureInput()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if entries := pendingObservations(t, s); len(entries) != 1 {
		t.Fatalf("concurrent append count = %d", len(entries))
	}
	for i := range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			input := captureInput()
			input.TurnID = "distinct-" + strconv.Itoa(i)
			if _, err := s.Capture(input); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if entries := pendingObservations(t, s); len(entries) != 13 {
		t.Fatalf("distinct concurrent observations lost: %d", len(entries))
	}
}

func TestExplicitCaptureFailureBoundaries(t *testing.T) {
	for _, kind := range []string{"empty_directive", "global_repo", "mirror_failure"} {
		t.Run(kind, func(t *testing.T) {
			s := testStore(t)
			input := captureInput()
			input.Explicit = true
			input.Scope, input.RepoKey, input.Directive = "global", "", "Prefer early returns."
			input.Surfaces = []string{"ruby"}
			switch kind {
			case "empty_directive":
				input.Directive = ""
			case "global_repo":
				input.RepoKey = "example-a1b2c3d4e5f6"
			case "mirror_failure":
				s.fault = func(stage string) error {
					if stage == "before_mirror" {
						return errors.New("injected mirror failure")
					}
					return nil
				}
			}
			result, err := s.Capture(input)
			if kind == "mirror_failure" {
				if err != nil || result.Action != "activated" || result.Reason != "compatibility_repair_needed" {
					t.Fatal("mirror failure lost explicit publication", result, err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid explicit capture accepted")
				}
				if current, err := s.loadCurrent(); err != nil || current != nil {
					t.Fatal("invalid explicit capture created a generation", err)
				}
			}
			if s.pendingCount() != 1 {
				t.Fatal("explicit capture lost recoverable pending evidence")
			}
		})
	}
}

func TestCaptureRejectsForgedPrivacyAndRepositoryAuthority(t *testing.T) {
	s := testStore(t)
	for _, repo := range []string{"../escape", "repo", "customer@example.test", "repo-0123456789ab/escape"} {
		input := captureInput()
		input.RepoKey = repo
		result, err := s.Capture(input)
		if err != nil || result.Reason != "invalid_repository_key" {
			t.Fatal("unsafe repository key accepted", repo, result, err)
		}
	}
	for _, feedback := range []string{"", "Always use password=synthetic-value"} {
		input := captureInput()
		input.Explicit = true
		input.Scope = "global"
		input.RepoKey = ""
		input.Feedback = feedback
		input.Directive = "Always use safe names."
		input.Surfaces = []string{"naming"}
		result, err := s.Capture(input)
		if err != nil || result.Action != "ignored" {
			t.Fatal("safe directive bypassed raw feedback privacy", result, err)
		}
	}
	if len(pendingObservations(t, s)) != 0 {
		t.Fatal("untrusted capture wrote evidence")
	}
}

func TestCaptureReferenceParity(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Name  string
			Steps []struct {
				Input       CaptureInput
				Action      string
				Reason      *string
				Observation *observation
			}
			Rules        []rule
			PersonaFiles map[string]string `json:"persona_files"`
		}
	}
	readContract(t, "capture.json", &fixtures)
	for _, fixture := range fixtures.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			s := testStore(t)
			for _, step := range fixture.Steps {
				result, err := s.Capture(step.Input)
				if err != nil {
					t.Fatal(err)
				}
				if result.Action != step.Action || result.Reason != value(step.Reason) {
					t.Fatalf("result differs: %v", result)
				}
				if step.Observation != nil {
					entries := pendingObservations(t, s)
					found := false
					for _, entry := range entries {
						if entry.ID == step.Observation.ID {
							found = true
							if !canonicalEqual(entry, step.Observation) {
								t.Fatal("observation differs from Ruby")
							}
						}
					}
					if !found {
						t.Fatal("reference observation missing")
					}
				}
			}
			current, err := s.loadCurrent()
			if err != nil {
				t.Fatal(err)
			}
			if fixture.Rules == nil {
				if current != nil {
					t.Fatal("reference has no generation")
				}
				return
			}
			if current == nil || !canonicalEqual(current.Rules.rules, fixture.Rules) || !canonicalEqual(current.PersonaFiles, fixture.PersonaFiles) {
				t.Fatal("explicit rules/persona differ from Ruby")
			}
		})
	}
}

func TestCaptureLockFailure(t *testing.T) {
	s := testStore(t)
	err := withFileLock(s.dataPath("observations", ".pending.lock"), false, func() error {
		if _, err := s.capture(captureInput(), true); err == nil {
			t.Fatal("denied nonblocking lock accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingObservations(t, s)) != 0 {
		t.Fatal("lock failure appended evidence")
	}
}

func TestReviewCaptureReplayAfterCompaction(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.Explicit = true
	input.Scope = "global"
	input.RepoKey = ""
	input.Directive = "Keep helpers focused."
	input.Surfaces = []string{"services"}
	first, err := s.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.dataPath("observations", "pending.jsonl"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.Capture(input)
	if err != nil || replayed.Action != "duplicate" || replayed.ID != first.ID {
		t.Fatalf("compacted replay: %v, %v", replayed, err)
	}
	if len(pendingObservations(t, s)) != 0 {
		t.Fatal("replayed processed event reentered inbox")
	}
}
