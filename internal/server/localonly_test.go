package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalServerDoesNotCheckUpdates(t *testing.T) {
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{"tag_name":"v999"}`)) }))
	defer remote.Close()
	s, _ := newTestServer(t)
	s.currentVersion = "v1.0.0"
	s.githubAPIURL = remote.URL
	s.DisableRemoteFeatures()
	s.CheckForUpdates()
	if calls != 0 {
		t.Fatalf("local build made %d update requests", calls)
	}
}

func TestLocalServerBlocksOutboundRoutesAndSetsCSP(t *testing.T) {
	s, _ := newTestServer(t)
	s.DisableRemoteFeatures()
	for _, path := range []string{"/api/agent/request"} {
		response := httptest.NewRecorder()
		s.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusForbidden {
			t.Errorf("%s returned %d", path, response.Code)
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Error("missing outbound browser resource policy")
		}
	}
	if s.prList != nil || s.agentCmd != "" {
		t.Fatal("background provider or agent command remains enabled")
	}
}
