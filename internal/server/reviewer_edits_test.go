package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func publishedBatch(t *testing.T, s *Server) feedback.Batch {
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
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"edits","disposition":"comment"}`)))
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	return result.Batch
}

func editComments(batch feedback.Batch) []feedback.Comment {
	var edits []feedback.Comment
	for _, c := range batch.Comments {
		if c.Scope == "edit" {
			edits = append(edits, c)
		}
	}
	return edits
}

func TestReviewerEditBecomesBatchComment(t *testing.T) {
	s, sess := newTestServer(t)
	if err := os.WriteFile(sess.Files[0].AbsPath, []byte("line1\nline two\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edits := editComments(publishedBatch(t, s))
	if len(edits) != 1 {
		t.Fatalf("want one edit comment, got %+v", edits)
	}
	e := edits[0]
	if e.ID != "edit:test.md:2" || e.Path != "test.md" || e.StartLine != 2 || e.EndLine != 2 || e.Quote != "line2" || e.Body != "Reviewer edited these lines:\nline two" {
		t.Fatalf("edit comment: %+v", e)
	}
}

func TestUneditedReviewHasNoEditComments(t *testing.T) {
	s, _ := newTestServer(t)
	if edits := editComments(publishedBatch(t, s)); len(edits) != 0 {
		t.Fatalf("edit comments without an edit: %+v", edits)
	}
}

func TestReviewerEditBodyIsCappedAndSanitized(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "x\x1b[31m"
	}
	body := reviewerEditBody(lines)
	if strings.Contains(body, "\x1b") || strings.Count(body, "\n") != 41 || !strings.HasSuffix(body, "… 10 more lines") {
		t.Fatalf("body not capped/sanitized: %q", body)
	}
	if got := reviewerEditBody(nil); got != "Reviewer deleted these lines." {
		t.Fatalf("deletion body: %q", got)
	}
}
