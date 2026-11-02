package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

// annotationServer serves a git-mode review of server.go whose diff adds
// lines 2-3.
func annotationServer(t *testing.T) (*Server, *Session) {
	t.Helper()
	sess := &Session{Mode: "git", RepoRoot: t.TempDir(), ReviewRound: 1, Files: []*FileEntry{{
		Path: "server.go", Status: "modified", FileType: "code", Content: "a\nb\nc\nd\n", Comments: []Comment{},
		DiffHunks: []vcs.DiffHunk{{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 4, Lines: []vcs.DiffLine{
			{Type: "context", Content: "a", OldNum: 1, NewNum: 1},
			{Type: "add", Content: "b", NewNum: 2},
			{Type: "add", Content: "c", NewNum: 3},
			{Type: "context", Content: "d", OldNum: 2, NewNum: 4},
		}}},
	}}}
	sess.InitTestChannels()
	s, err := NewServer(sess, frontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, sess
}

func postAnnotations(s *Server, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/annotations", strings.NewReader(body)))
	return rec
}

func TestAnnotationsReplaceAndRead(t *testing.T) {
	s, sess := annotationServer(t)
	body := `{"tool":"golangci-lint","findings":[{"path":"server.go","line":2,"end_line":2,"message":"x is unused","rule":"unused"},{"path":"server.go","line":9,"end_line":9,"message":"off the diff"}],"marks":{"server.go":[{"start":1,"end":3,"kind":"uncovered"}]}}`
	rec := postAnnotations(s, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body)
	}
	var result struct {
		Added, Kept, Removed, Dropped int
		Coverage                      map[string]Coverage
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || result.Dropped != 1 || result.Coverage["server.go"] != (Coverage{Changed: 2, Uncovered: 2}) {
		t.Fatalf("result = %+v", result)
	}
	comments := sess.GetVisibleComments()["server.go"]
	if len(comments) != 1 || comments[0].Body != "x is unused `unused`" || comments[0].Source != "tool" || comments[0].Author != "golangci-lint" {
		t.Fatalf("stored = %+v", comments)
	}
	if _, ok := sess.AddReply("server.go", comments[0].ID, "Intentional.", "human"); !ok {
		t.Fatal("reply rejected")
	}
	if rec := postAnnotations(s, body); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kept":1`) {
		t.Fatalf("second POST = %d %s", rec.Code, rec.Body)
	}
	if got := sess.GetVisibleComments()["server.go"]; len(got) != 1 || len(got[0].Replies) != 1 {
		t.Fatalf("thread lost on re-run: %+v", got)
	}

	get := httptest.NewRecorder()
	s.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/annotations", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"golangci-lint"`) {
		t.Fatalf("GET = %d %s", get.Code, get.Body)
	}
}

func TestAnnotationsRejectsBadRequests(t *testing.T) {
	s, _ := annotationServer(t)
	cases := []struct {
		name, method, body string
		want               int
	}{
		{"missing tool", http.MethodPost, `{"findings":[]}`, http.StatusBadRequest},
		{"bad json", http.MethodPost, `{`, http.StatusBadRequest},
		{"method", http.MethodDelete, ``, http.StatusMethodNotAllowed},
		{"too large", http.MethodPost, `{"tool":"x","findings":[{"message":"` + strings.Repeat("a", 11<<20) + `"}]}`, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(tc.method, "/api/annotations", strings.NewReader(tc.body)))
			if rec.Code != tc.want {
				t.Fatalf("%s = %d, want %d", tc.name, rec.Code, tc.want)
			}
		})
	}
}

func TestPublishCarriesCommentProvenance(t *testing.T) {
	s, _ := annotationServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	if rec := postAnnotations(s, `{"tool":"lint","findings":[{"path":"server.go","line":2,"end_line":2,"message":"m"}]}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"r1","disposition":"comment"}`)))
	if rec.Code != 200 {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	var tool *feedback.Comment
	for i, c := range result.Batch.Comments {
		if c.Source == "tool" {
			tool = &result.Batch.Comments[i]
		}
	}
	if tool == nil || tool.SourceRef != "lint" {
		t.Fatalf("batch lost provenance: %+v", result.Batch.Comments)
	}
}
