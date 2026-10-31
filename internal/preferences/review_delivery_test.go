package preferences

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func withReviews(t *testing.T, notices []ReviewNotice) {
	t.Helper()
	previousNotices, previousWaiter := reviewNotices, reviewWaiter
	reviewNotices = func(hostKind, sessionID string) []ReviewNotice {
		if hostKind != "claude" || sessionID != "s1" {
			return nil
		}
		return notices
	}
	t.Cleanup(func() { reviewNotices, reviewWaiter = previousNotices, previousWaiter })
}

func hookResponse(t *testing.T, s *Store, event string, payload map[string]any) map[string]any {
	t.Helper()
	payload["hook_event_name"] = event
	data, _ := json.Marshal(payload)
	var out bytes.Buffer
	s.hook(event, bytes.NewReader(data), &out, &bytes.Buffer{})
	var response map[string]any
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("invalid response %q", out.String())
	}
	return response
}

func TestSessionStartAnnouncesQueuedReviewFeedback(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa", Pending: 2, BatchID: "batch_2", Disposition: "request_changes"}})
	response := hookResponse(t, s, "SessionStart", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "source": "resume"})
	context := textField(record(response["hookSpecificOutput"])["additionalContext"])
	if !strings.Contains(context, "Hunkyard review aaaaaaaaaaaa has 2 published review batch(es) waiting") || !strings.Contains(context, "hunkyard feedback await --session aaaaaaaaaaaa --json") {
		t.Fatalf("queued feedback not announced: %q", context)
	}
}

func TestStopBlocksOncePerPublishedBatch(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa", Pending: 1, BatchID: "batch_1", Disposition: "comment"}})
	first := hookResponse(t, s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "stop_hook_active": false})
	if first["decision"] != "block" || !strings.Contains(textField(first["reason"]), "feedback await --session aaaaaaaaaaaa") {
		t.Fatalf("published batch not delivered at stop: %v", first)
	}
	if second := hookResponse(t, s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "stop_hook_active": false}); second["decision"] == "block" {
		t.Fatal("same batch blocked twice")
	}
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa", Pending: 2, BatchID: "batch_2", Disposition: "comment"}})
	if third := hookResponse(t, s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir(), "stop_hook_active": false}); third["decision"] != "block" {
		t.Fatal("a newer batch was not delivered")
	}
}

func TestNoOwnedReviewCostsNothing(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, nil)
	if response := hookResponse(t, s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir()}); response["decision"] == "block" {
		t.Fatal("blocked without a review")
	}
	if _, err := os.Stat(s.dataPath("sessions")); !os.IsNotExist(err) {
		t.Fatal("session state written for a session without reviews")
	}
}

func reviewWake(t *testing.T, s *Store) (int, string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": "s1", "cwd": t.TempDir()})
	var stderr bytes.Buffer
	code := s.hook("ReviewWake", bytes.NewReader(data), &bytes.Buffer{}, &stderr)
	return code, stderr.String()
}

func TestReviewWakeExitsTwoOnPublication(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, []ReviewNotice{{ReviewID: "aaaaaaaaaaaa"}})
	reviewWaiter = func(reviewID string, timeout time.Duration, accept func(string) bool, active func() bool) (string, string, bool) {
		return "batch_9", "request_changes", reviewID == "aaaaaaaaaaaa" && timeout > time.Minute && accept("batch_9") && active()
	}
	code, message := reviewWake(t, s)
	if code != 2 || !strings.Contains(message, "feedback await --session aaaaaaaaaaaa") {
		t.Fatalf("wake: %d %q", code, message)
	}
	if response := hookResponse(t, s, "Stop", map[string]any{"session_id": "s1", "cwd": t.TempDir()}); response["decision"] == "block" {
		t.Fatal("woken batch delivered a second time at stop")
	}
}

func TestReviewWakeWithoutReviewExitsZero(t *testing.T) {
	s := testStore(t)
	s.hostKind = "claude"
	withReviews(t, nil)
	called := false
	reviewWaiter = func(string, time.Duration, func(string) bool, func() bool) (string, string, bool) {
		called = true
		return "", "", false
	}
	if code, _ := reviewWake(t, s); code != 0 || called {
		t.Fatalf("idle session waited: %d %v", code, called)
	}
}

func TestInstallAddsClaudeReviewWake(t *testing.T) {
	s := testStore(t)
	t.Setenv("CLAUDE_CONFIG_DIR", s.dataPath("claude"))
	result, err := s.installHook("claude")
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := mapArray(record(readJSONFile(textField(result["path"]))["hooks"])["Stop"])
	wake := 0
	for _, group := range groups {
		entries, _ := mapArray(group["hooks"])
		for _, entry := range entries {
			if strings.Contains(textField(entry["command"]), "--event ReviewWake") {
				wake++
				if entry["asyncRewake"] != true || fmt.Sprint(entry["timeout"]) != "3600" {
					t.Fatalf("wake handler not async with a 1h timeout: %v", entry)
				}
			}
		}
	}
	if wake != 1 {
		t.Fatalf("want one review wake handler, got %d", wake)
	}
	codex, err := s.installHook("codex")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustRead(t, textField(codex["path"]))), "--event ReviewWake") {
		t.Fatal("Codex got the Claude-only wake handler")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
