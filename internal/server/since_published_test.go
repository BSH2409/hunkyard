package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
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

func TestSincePublishedUnsupportedSelectionKeepsGitComparison(t *testing.T) {
	dir := testutil.InitTestRepo(t)
	t.Chdir(dir)
	testutil.SetHome(t, t.TempDir())
	file := filepath.Join(dir, "README.md")
	testutil.WriteFile(t, file, "# Test\ncommitted\n")
	testutil.Git(t, dir, "commit", "-am", "committed change")
	commit := testutil.Git(t, dir, "rev-parse", "HEAD")
	base := testutil.Git(t, dir, "rev-parse", "HEAD^")
	testutil.WriteFile(t, file, "# Test\ncommitted\nstaged\n")
	testutil.Git(t, dir, "add", "README.md")
	testutil.WriteFile(t, file, "# Test\ncommitted\nstaged\nunstaged\n")
	sess, err := session.NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	s, err := NewServer(sess, frontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	publishForTest(t, s)

	for _, test := range []struct {
		name, query, want string
		rangeFocus        bool
	}{
		{"staged scope", "&scope=staged", "staged", false},
		{"selected commit", "&commit=" + commit, "committed", false},
		{"range focus", "", "committed", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.rangeFocus {
				if err := sess.SetFocus(session.Focus{Kind: session.FocusRange, BaseSHA: base, HeadSHA: commit}); err != nil {
					t.Fatal(err)
				}
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/file/diff?path=README.md&since=published"+test.query, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("since diff: %d %s", rec.Code, rec.Body)
			}
			var response struct {
				Hunks                  []diff.DiffHunk
				SinceUnavailable       bool   `json:"since_unavailable"`
				SinceUnavailableReason string `json:"since_unavailable_reason"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			var changes []string
			for _, hunk := range response.Hunks {
				for _, line := range hunk.Lines {
					if line.Type == "add" && line.Content != "# Test" {
						changes = append(changes, line.Content)
					}
				}
			}
			if !response.SinceUnavailable || response.SinceUnavailableReason != "unsupported_comparison" || len(changes) != 1 || changes[0] != test.want {
				t.Fatalf("want unsupported comparison with only %q added, got %s", test.want, rec.Body)
			}
		})
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
