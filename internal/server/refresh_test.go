package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshEndpoint(t *testing.T) {
	s, _ := newTestServer(t)
	get := httptest.NewRecorder()
	s.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/refresh", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/refresh = %d, want 405", get.Code)
	}
	post := httptest.NewRecorder()
	s.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/refresh", nil))
	if post.Code != http.StatusOK {
		t.Fatalf("POST /api/refresh = %d %s", post.Code, post.Body)
	}
}
