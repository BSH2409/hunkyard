package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
)

func publishForTest(t *testing.T, s *Server) {
	t.Helper()
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"since","disposition":"comment"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
}

func sinceDiff(t *testing.T, s *Server) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file/diff?path=test.md&since=published", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("since diff: %d %s", rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSincePublishedShowsOnlyTheNewChange(t *testing.T) {
	s, sess := newTestServer(t)
	publishForTest(t, s)
	sess.Lock()
	sess.Files[0].Content = "line1\nline2 changed\nline3\n"
	sess.Files[0].FileHash = "sha256:changed"
	sess.Unlock()
	out := sinceDiff(t, s)
	if out["since"] != "published" || out["since_unavailable"] == true {
		t.Fatalf("not a since-published diff: %v", out)
	}
	encoded, _ := json.Marshal(out["hunks"])
	if !strings.Contains(string(encoded), `"line2 changed"`) || strings.Count(string(encoded), `"Type":"add"`) != 1 {
		t.Fatalf("want exactly the one new line added: %s", encoded)
	}
}

func TestSincePublishedWithoutPublicationFallsBack(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	out := sinceDiff(t, s)
	if out["since_unavailable"] != true {
		t.Fatalf("missing fallback flag: %v", out)
	}
	if _, ok := out["hunks"]; !ok {
		t.Fatalf("fallback lost the normal diff: %v", out)
	}
}

func TestSinceSnapshotHunks(t *testing.T) {
	snapshot := session.ReviewSnapshot{Files: []session.ReviewSnapshotFile{{Path: "a.go", Content: []byte("one\ntwo\n")}}}
	tests := []struct {
		name, path, content string
		adds, dels          int
	}{
		{"unchanged file has no hunks", "a.go", "one\ntwo\n", 0, 0},
		{"edited line", "a.go", "one\nTWO\n", 1, 1},
		{"file absent from the snapshot is all added", "new.go", "x\ny\n", 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adds, dels := 0, 0
			for _, h := range sinceSnapshotHunks(snapshot, tt.path, tt.content) {
				for _, l := range h.Lines {
					switch l.Type {
					case "add":
						adds++
					case "del":
						dels++
					}
				}
			}
			if adds != tt.adds || dels != tt.dels {
				t.Fatalf("got +%d -%d, want +%d -%d", adds, dels, tt.adds, tt.dels)
			}
		})
	}
}
