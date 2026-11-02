package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

// askServer is a review bound to an owner, as `hunkyard ask` needs.
func askServer(t *testing.T) (*Server, *Session, *feedback.Owner) {
	t.Helper()
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	return s, sess, owner
}

func serve(s *Server, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	s.ServeHTTP(rec, req)
	return rec
}

func ask(t *testing.T, s *Server, owner *feedback.Owner, body string) feedback.DecisionRequest {
	t.Helper()
	rec := serve(s, http.MethodPost, "/api/feedback/ask", body, map[string]string{feedback.OwnerHeader: owner.Credential()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("ask = %d %s", rec.Code, rec.Body)
	}
	var d feedback.DecisionRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDecisionEndpoints(t *testing.T) {
	s, _, owner := askServer(t)
	d := ask(t, s, owner, `{"question":"Which auth?","options":["Session","JWT"],"context":{"path":"test.md","line":1}}`)
	if d.ID == "" || d.Status != "open" {
		t.Fatalf("asked = %+v", d)
	}
	ownerHeader := map[string]string{feedback.OwnerHeader: owner.Credential()}
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}
	cases := []struct {
		name, method, target, body string
		header                     map[string]string
		want                       int
	}{
		{"ask without owner", http.MethodPost, "/api/feedback/ask", `{"question":"q","options":["a","b"]}`, nil, http.StatusForbidden},
		{"ask invalid", http.MethodPost, "/api/feedback/ask", `{"question":"q","options":["a"]}`, ownerHeader, http.StatusBadRequest},
		{"ask bad json", http.MethodPost, "/api/feedback/ask", `{`, ownerHeader, http.StatusBadRequest},
		{"ask method", http.MethodGet, "/api/feedback/ask", ``, ownerHeader, http.StatusMethodNotAllowed},
		{"list", http.MethodGet, "/api/decisions", ``, nil, http.StatusOK},
		{"list method", http.MethodDelete, "/api/decisions", ``, sameOrigin, http.StatusMethodNotAllowed},
		{"answer unknown", http.MethodPut, "/api/decisions/d_000000", `{"chosen":"JWT"}`, sameOrigin, http.StatusNotFound},
		{"answer invalid", http.MethodPut, "/api/decisions/" + d.ID, `{"chosen":"OAuth"}`, sameOrigin, http.StatusBadRequest},
		{"answer cross-site", http.MethodPut, "/api/decisions/" + d.ID, `{"chosen":"JWT"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"answer outside the page", http.MethodPut, "/api/decisions/" + d.ID, `{"chosen":"JWT"}`, nil, http.StatusForbidden},
		{"answer", http.MethodPut, "/api/decisions/" + d.ID, `{"chosen":"JWT"}`, sameOrigin, http.StatusOK},
		{"answer twice", http.MethodPut, "/api/decisions/" + d.ID, `{"chosen":"Session"}`, sameOrigin, http.StatusConflict},
		{"answer method", http.MethodPost, "/api/decisions/" + d.ID, `{}`, sameOrigin, http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := serve(s, tc.method, tc.target, tc.body, tc.header); rec.Code != tc.want {
				t.Fatalf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
			}
		})
	}
	list := serve(s, http.MethodGet, "/api/decisions", "", nil)
	if !strings.Contains(list.Body.String(), `"chosen":"JWT"`) {
		t.Fatalf("list = %s", list.Body)
	}
}

func TestApproveBlockedByOpenBlockingDecision(t *testing.T) {
	s, _, owner := askServer(t)
	d := ask(t, s, owner, `{"question":"Ship it?","options":["yes","no"],"blocking":true}`)
	if rec := serve(s, http.MethodPost, "/api/finish", `{"request_id":"r1","disposition":"approve"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("approve with an open blocking decision = %d %s", rec.Code, rec.Body)
	}
	serve(s, http.MethodPut, "/api/decisions/"+d.ID, `{"chosen":"yes"}`, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec := serve(s, http.MethodPost, "/api/finish", `{"request_id":"r2","disposition":"approve"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("approve after answering = %d %s", rec.Code, rec.Body)
	}
}

func TestPublishIncludesAnsweredDecisionsOnce(t *testing.T) {
	s, _, owner := askServer(t)
	answered := ask(t, s, owner, `{"question":"Which auth?","options":["Session","JWT"]}`)
	open := ask(t, s, owner, `{"question":"Rename it?","options":["yes","no"]}`)
	serve(s, http.MethodPut, "/api/decisions/"+answered.ID, `{"chosen":"JWT"}`, map[string]string{"Sec-Fetch-Site": "same-origin"})
	publish := func(id string) feedback.Batch {
		rec := serve(s, http.MethodPost, "/api/finish", `{"request_id":"`+id+`","disposition":"comment","summary":"s `+id+`"}`, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("publish %s = %d %s", id, rec.Code, rec.Body)
		}
		var result struct {
			Batch feedback.Batch `json:"batch"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		return result.Batch
	}
	first := publish("p1")
	if len(first.Decisions) != 2 || first.Decisions[0].Chosen != "JWT" || first.Decisions[1].Status != "open" {
		t.Fatalf("first batch decisions = %+v", first.Decisions)
	}
	second := publish("p2")
	if len(second.Decisions) != 1 || second.Decisions[0].ID != open.ID {
		t.Fatalf("second batch decisions = %+v", second.Decisions)
	}
}

func TestAgentApprovalRecordsStats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s, sess, _ := askServer(t)
	s.ConfigureDaemon(Config{}, sess.RepoRoot, home, s.reviewPath, nil, time.Now().Add(-time.Minute))
	if rec := serve(s, http.MethodPost, "/api/finish", `{"request_id":"approve-1","disposition":"approve"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(home, ".hunkyard", "stats.json"))
	if err != nil || !strings.Contains(string(data), `"approved": true`) {
		t.Fatalf("stats after an agent-owned approval: %s %v", data, err)
	}
	if s.ShouldRecordStatsOnShutdown() {
		t.Fatal("shutdown would record the approved review a second time")
	}
}
