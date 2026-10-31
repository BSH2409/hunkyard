package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Hunkyard never checks for updates, so the page gets no release to announce.
func TestConfigReportsNoUpdateInformation(t *testing.T) {
	s, _ := newTestServer(t)
	s.currentVersion = "v1.0.0"
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var config map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"latest_version", "no_update_check"} {
		if _, found := config[key]; found {
			t.Errorf("/api/config still reports %s", key)
		}
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
