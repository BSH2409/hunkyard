package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOverlappingWakeWaitersDeliverOnce(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa"}})
	reviewWaiter = func(reviewID string, timeout time.Duration, accept func(string) bool, active func() bool) (string, string, bool) {
		if accept("batch_1") && active() {
			return "batch_1", "comment", true
		}
		return "", "", false
	}
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = reviewWake(t, s)
		}()
	}
	wg.Wait()
	woke := 0
	for _, code := range codes {
		if code == 2 {
			woke++
		}
	}
	if woke != 1 {
		t.Fatalf("one publication woke Claude %d times", woke)
	}
}

func TestReviewWakeHonoursStopHookActive(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa"}})
	called := false
	reviewWaiter = func(string, time.Duration, func(string) bool, func() bool) (string, string, bool) {
		called = true
		return "", "", false
	}
	data, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": "s1", "stop_hook_active": true})
	if code := s.hook("ReviewWake", bytes.NewReader(data), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 || called {
		t.Fatal("waiter started during a stop-hook continuation")
	}
}

func TestCheckQuotesArgvMessages(t *testing.T) {
	s := testStore(t)
	captureRule(t, s, "Commit subjects reference issues.", "devops")
	current, _ := s.loadCurrent()
	if _, err := s.addGuard("commit_message_format", textField(current.Rules.rules[0]["id"]), []string{`^Refs #\d+: `}, nil, nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Run([]string{"check", "--root", s.config.Root, "--cwd", t.TempDir(), "--", "git", "commit", "-m", "Refs #4: fix the guard"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if strings.Contains(out.String(), `"blocked":true`) {
		t.Fatalf("well-formed message reported blocked: %s", out.String())
	}
}

func TestPathSurfacesUseRepositoryRelativePaths(t *testing.T) {
	cwd := "/home/me/test/shop"
	got := pathSurfacesFor([]string{"/home/me/test/shop/app/models/user.rb"}, cwd)
	for _, unwanted := range []string{"testing"} {
		for _, surface := range got {
			if surface == unwanted {
				t.Fatalf("parent directory chose surface %q: %v", unwanted, got)
			}
		}
	}
}

func TestClaudeInterruptDetectedWhenPromptAlreadyInTranscript(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	transcript := filepath.Join(home, "projects", "repo", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0700); err != nil {
		t.Fatal(err)
	}
	prompt := "keep it inside the existing service class"
	lines := `{"type":"user","sessionId":"s1","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}
{"type":"user","sessionId":"s1","message":{"role":"user","content":"` + prompt + `"}}
`
	if err := os.WriteFile(transcript, []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "cwd": t.TempDir(), "prompt": prompt, "transcript_path": transcript})
	s.hook("UserPromptSubmit", bytes.NewReader(payload), &bytes.Buffer{}, &bytes.Buffer{})
	if observations := pendingObservations(t, s); len(observations) != 1 || !observations[0].Signals.Correction {
		t.Fatalf("interrupt before an already-recorded prompt missed: %+v", observations)
	}
}
