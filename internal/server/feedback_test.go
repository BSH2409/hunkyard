package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestPublishRejectsAStaleDisplayedFile(t *testing.T) {
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	file, ok := sess.GetFileSnapshot("test.md")
	if !ok {
		t.Fatal("fixture file missing")
	}
	for _, field := range []string{"content_id", "view_id"} {
		context := map[string]any{"path": "test.md", "content_id": file["file_hash"], "view_id": file["view_id"]}
		context[field] = "stale"
		body, err := json.Marshal(map[string]any{"request_id": field, "disposition": "approve", "displayed_files": []any{context}})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(string(body))))
		if rec.Code != 409 {
			t.Fatalf("stale %s approved: %d %s", field, rec.Code, rec.Body)
		}
	}
}

func TestLegacyRoundCompleteCannotBypassPendingFeedback(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	store, err := s.feedbackStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish("pending", feedback.Batch{Version: 1, ReviewID: "abcd1234ef01", SnapshotID: "snapshot_" + strings.Repeat("a", 64), Round: 1, Disposition: "comment", Comments: []feedback.Comment{{ID: "human", Body: "Fix this line"}}}); err != nil {
		t.Fatal(err)
	}
	for _, authenticated := range []bool{false, true} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/round-complete", nil)
		want := http.StatusForbidden
		if authenticated {
			req.Header.Set(feedback.OwnerHeader, owner.Credential())
			want = http.StatusConflict
		}
		s.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("round bypass: %d, want %d", rec.Code, want)
		}
	}
}

func TestPublishSurvivesAbsentWaiter(t *testing.T) {
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.AddComment("test.md", 1, 1, "right", "Prefer clear names", "", ""); !ok {
		t.Fatal("comment fixture rejected")
	}
	publish := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(body)))
		return rec
	}
	if rec := publish(`{}`); rec.Code != 400 {
		t.Fatalf("implicit approval accepted: %d", rec.Code)
	}
	rec := publish(`{"request_id":"synthetic-request","disposition":"request_changes","summary":"Please address these"}`)
	if rec.Code != 200 {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Batch.Comments) != 2 || result.Batch.Disposition != "request_changes" {
		t.Fatalf("published: %#v", result.Batch)
	}
	store, err := feedback.Open(s.reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/feedback/await?timeout=1ms", nil)
	req.Header.Set(feedback.OwnerHeader, owner.Credential())
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), result.Batch.ID) {
		t.Fatalf("reconnect: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feedback/await?timeout=1ms", nil))
	if rec.Code != 403 {
		t.Fatal("other consumer read queue")
	}
}

func TestSkillManifestBoundary(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/feedback/skills", strings.NewReader(`{"version":1,"skills":[]}`)))
	if rec.Code != 403 {
		t.Fatal("browser registered host skills")
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"forged","disposition":"comment","skill_ids":["forged-path"]}`)))
	if rec.Code != 400 {
		t.Fatalf("forged skill accepted: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `"path"`) {
		t.Fatal("public catalog exposed source paths")
	}
}

func TestFeedbackLearnIsOwnerBoundAndRejectsRoot(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	t.Setenv("CODEX_HOME", t.TempDir())
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	store, err := s.feedbackStore()
	if err != nil {
		t.Fatal(err)
	}
	batch, err := store.Publish("test", feedback.Batch{Version: 1, ReviewID: "abcd1234ef01", SnapshotID: "snapshot_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Round: 1, Disposition: "comment", Comments: []feedback.Comment{{ID: "human", Body: "Fix this one line"}}})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"batch_id":"` + batch.ID + `","decisions":[{"comment_id":"human","revision":1,"kind":"ignore"}]}`
	post := func(body string, authenticated bool) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/feedback/learn", strings.NewReader(body))
		if authenticated {
			req.Header.Set(feedback.OwnerHeader, owner.Credential())
		}
		s.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(body, false); rec.Code != 403 {
		t.Fatal("unauthenticated preference write accepted")
	}
	if rec := post(strings.TrimSuffix(body, "}")+`,"root":"/untrusted"}`, true); rec.Code != 400 {
		t.Fatalf("browser-supplied preference root accepted: %d", rec.Code)
	}
	if rec := post(body, true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ignored"`) {
		t.Fatalf("classify: %d %s", rec.Code, rec.Body)
	}
	if err := store.Ack(batch.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPublishIncludesHumanReplies(t *testing.T) {
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	comment, ok := sess.AddComment("test.md", 1, 1, "right", "Use clear names", "", "")
	if !ok {
		t.Fatal("comment fixture")
	}
	reply, ok := sess.AddReply("test.md", comment.ID, "Always prefer domain names", "")
	if !ok {
		t.Fatal("reply fixture")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"human-reply","disposition":"comment"}`)))
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &result) != nil || rec.Code != 200 {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	if len(result.Batch.Comments) != 2 {
		t.Fatalf("human reply omitted from classification: %#v", result.Batch.Comments)
	}
	if result.Batch.Comments[1].ParentID != comment.ID || !strings.Contains(result.Batch.Comments[1].ID, reply.ID) {
		t.Fatal("human reply lost its thread identity")
	}
}

func TestChangedSelectedSkillCanBeExplicitlyRetrusted(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	store, err := s.feedbackStore()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("original instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	refs := []feedback.SkillRef{{ID: "naming", Name: "Naming", Source: "codex", Path: path}}
	if err := store.RegisterSkills(refs); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"skill-update","disposition":"request_changes","summary":"Fix names","skill_ids":["naming"]}`)))
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	before, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(before[0].PublishedSkills) != 1 || before[0].PublishedSkills[0].Fingerprint == "" {
		t.Fatal("publication lost original skill provenance")
	}
	if err := os.WriteFile(path, []byte("reviewed new instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	await := func() map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/feedback/await?timeout=1ms", nil)
		req.Header.Set(feedback.OwnerHeader, owner.Credential())
		s.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("feedback inaccessible: %d %s", rec.Code, rec.Body.String())
		}
		var data map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	blocked := await()
	if len(blocked["blocked_skills"].(map[string]any)) != 1 || !strings.Contains(string(mustJSON(t, blocked)), "Fix names") {
		t.Fatal("changed skill not safely blocked", blocked)
	}
	body, err := json.Marshal(map[string]any{"version": 1, "skills": refs})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/feedback/skills", strings.NewReader(string(body)))
	req.Header.Set(feedback.OwnerHeader, owner.Credential())
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal("owner cannot renew trust", rec.Code, rec.Body.String())
	}
	resolved := await()
	if len(resolved["blocked_skills"].(map[string]any)) != 0 || len(resolved["selected_skills"].(map[string]any)[before[0].ID].([]any)) != 1 {
		t.Fatal("trust renewal did not unblock selection", resolved)
	}
	after, err := store.Pending()
	if err != nil || string(mustJSON(t, before)) != string(mustJSON(t, after)) {
		t.Fatal("renewal rewrote publication", err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
