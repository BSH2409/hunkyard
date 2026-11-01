package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActivityFileExtendsLastActivity(t *testing.T) {
	dir := t.TempDir()
	s := &Session{}
	s.SetActivityFile(filepath.Join(dir, "activity"))
	s.MarkActivity("start")
	first := s.LastActivity()
	later := first.Add(10 * time.Minute)
	if err := os.WriteFile(filepath.Join(dir, "activity"), []byte("idle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "activity"), later, later); err != nil {
		t.Fatal(err)
	}
	if got := s.LastActivity(); !got.Equal(later) {
		t.Fatalf("LastActivity = %v, want the activity file's time %v", got, later)
	}
}

func TestMarkActivityWritesFileAtMostOncePerInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity")
	s := &Session{}
	s.SetActivityFile(path)
	s.MarkActivity("click")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("activity file not written: %v", err)
	}
	past := info.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	s.MarkActivity("click")
	if again, _ := os.Stat(path); !again.ModTime().Equal(past) {
		t.Fatal("a second mark inside the interval rewrote the file")
	}
	if got := s.LastActivity(); time.Since(got) > time.Second {
		t.Fatalf("in-memory activity not updated: %v", got)
	}
}

func TestExternalCommentCountsAsActivity(t *testing.T) {
	dir := t.TempDir()
	s := &Session{
		RepoRoot: dir, ReviewRound: 1,
		Files:       []*FileEntry{{Path: "a.go", Status: "modified", FileType: "code", Comments: []Comment{}}},
		subscribers: make(map[chan SSEEvent]struct{}),
	}
	cj := HunkyardJSON{ReviewRound: 1, Files: map[string]HunkyardJSONFile{
		"a.go": {Status: "modified", Comments: []Comment{{ID: "c1", Body: "from hunkyard comment", StartLine: 1, EndLine: 1}}},
	}}
	data, _ := json.Marshal(cj)
	path := filepath.Join(dir, ".hunkyard", "review.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !s.mergeExternalHunkyardJSON() {
		t.Fatal("external comment not merged")
	}
	if time.Since(s.LastActivity()) > time.Second {
		t.Fatal("a comment written by the headless CLI did not count as activity")
	}
}
