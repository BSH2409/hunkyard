package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestTouchOwnedReviewActivityWritesTurnMarker(t *testing.T) {
	root := t.TempDir()
	if _, err := feedback.OpenOwner(root, "abcdef012345", feedback.IdentityForSeed("claude:s1")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "reviews", "abcdef012345"), 0o700); err != nil {
		t.Fatal(err)
	}
	TouchOwnedReviewActivity(root, "claude", "s1", "turn")
	data, err := os.ReadFile(filepath.Join(root, "reviews", "abcdef012345", "activity"))
	if err != nil || string(data) != "turn\n" {
		t.Fatalf("activity file = %q, %v", data, err)
	}
	TouchOwnedReviewActivity(root, "claude", "someone-else", "idle")
	if data, _ := os.ReadFile(filepath.Join(root, "reviews", "abcdef012345", "activity")); string(data) != "turn\n" {
		t.Fatal("another host session changed this review's marker")
	}
}

func TestNoOwnedReviewTouchesNothing(t *testing.T) {
	root := t.TempDir()
	TouchOwnedReviewActivity(root, "claude", "s1", "turn")
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatalf("created %d entries without an owned review", len(entries))
	}
}

func TestStaleTurnMarkerIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity")
	s := &Session{}
	s.SetActivityFile(path)
	now := time.Now()
	write := func(content string, at time.Time) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write("turn\n", now.Add(-40*time.Minute))
	if !s.AgentTurnActive(now, 90*time.Minute) {
		t.Fatal("an agent turn that started 40 minutes ago should keep the review awake")
	}
	write("turn\n", now.Add(-2*time.Hour))
	if s.AgentTurnActive(now, 90*time.Minute) {
		t.Fatal("a turn marker older than the window should be ignored (crashed session)")
	}
	write("idle\n", now)
	if s.AgentTurnActive(now, 90*time.Minute) {
		t.Fatal("an idle marker is not an active turn")
	}
}
