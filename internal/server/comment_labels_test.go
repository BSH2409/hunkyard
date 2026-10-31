package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestCommentLabelsRoundTripToPublishedBatch(t *testing.T) {
	s, sess := newTestServer(t)
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/file/comments?path=test.md", strings.NewReader(body)))
		return rec
	}
	if rec := post(`{"start_line":1,"end_line":1,"body":"x","label":"shout"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown label accepted: %d", rec.Code)
	}
	rec := post(`{"start_line":1,"end_line":1,"body":"Use the shared client","label":"issue","blocking":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("labelled comment: %d %s", rec.Code, rec.Body)
	}
	comments := sess.GetComments("test.md")
	if len(comments) != 1 || comments[0].Label != "issue" || !comments[0].Blocking {
		t.Fatalf("label not stored: %+v", comments)
	}
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	published := httptest.NewRecorder()
	s.ServeHTTP(published, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"labels","disposition":"request_changes"}`)))
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if err := json.Unmarshal(published.Body.Bytes(), &result); err != nil || published.Code != 200 {
		t.Fatalf("publish: %d %s", published.Code, published.Body)
	}
	found := false
	for _, c := range result.Batch.Comments {
		found = found || c.Label == "issue" && c.Blocking
	}
	if !found {
		t.Fatalf("batch lost the label: %#v", result.Batch.Comments)
	}
}

func TestClaimUnchangedReachesPublishedBatch(t *testing.T) {
	s, sess := newTestServer(t)
	c, ok := sess.AddComment("test.md", 1, 1, "", "Fix this", "", "")
	if !ok {
		t.Fatal("comment fixture rejected")
	}
	for _, f := range sess.Files {
		for i := range f.Comments {
			if f.Comments[i].ID == c.ID {
				f.Comments[i].ClaimUnchangedRound = 1
			}
		}
	}
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"claim","disposition":"request_changes"}`)))
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	for _, published := range result.Batch.Comments {
		if published.ID == c.ID && published.ClaimUnchanged {
			return
		}
	}
	t.Fatalf("claim flag missing from batch: %s", rec.Body)
}

func TestApproveRejectedWhileBlockingCommentOpen(t *testing.T) {
	s, sess := newTestServer(t)
	c, ok := sess.AddComment("test.md", 1, 1, "", "Must fix", "", "")
	if !ok {
		t.Fatal("comment fixture rejected")
	}
	if _, ok := sess.SetCommentLabel("test.md", c.ID, "issue", true); !ok {
		t.Fatal("blocking label rejected")
	}
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"blocked","disposition":"approve"}`)))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "blocking") {
		t.Fatalf("approve with an open blocking comment: %d %s", rec.Code, rec.Body)
	}
}
