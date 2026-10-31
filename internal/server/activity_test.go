package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStateChangingRequestsMarkActivity(t *testing.T) {
	s, sess := newTestServer(t)
	idle := sess.LastActivity()
	get := httptest.NewRecorder()
	s.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if !sess.LastActivity().Equal(idle) {
		t.Fatal("a GET counted as activity")
	}
	post := httptest.NewRecorder()
	s.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/file/comments?path=test.md", strings.NewReader(`{"start_line":1,"end_line":1,"body":"x"}`)))
	if time.Since(sess.LastActivity()) > time.Second {
		t.Fatal("adding a comment did not count as activity")
	}
}

func TestActivityEndpoint(t *testing.T) {
	s, sess := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/activity", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST /api/activity = %d", rec.Code)
	}
	if time.Since(sess.LastActivity()) > time.Second {
		t.Fatal("tab activity not recorded")
	}
	get := httptest.NewRecorder()
	s.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/activity", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/activity = %d, want 405", get.Code)
	}
}
