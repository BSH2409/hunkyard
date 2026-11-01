package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// The review may be opened by any host it listens on (a LAN or container
// address); its page must be allowed to frame the proxy on that host.
func TestReviewPageMayFrameTheProxyOnItsOwnHost(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "172.17.0.2:8000"
	response := httptest.NewRecorder()
	s.ServeHTTP(response, req)
	csp := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-src 'self' http://172.17.0.2:*") {
		t.Errorf("review page policy = %q, want frame-src to allow the proxy on 172.17.0.2", csp)
	}
}

func TestPreviewContentLoadsItsOwnResourcesByDefault(t *testing.T) {
	s, _ := newTestServer(t)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/preview-content/", nil))
	if csp := response.Header().Get("Content-Security-Policy"); csp != "" {
		t.Errorf("preview content policy = %q, want none by default", csp)
	}
	t.Setenv("HUNKYARD_BLOCK_EXTERNAL", "1")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/preview-content/", nil))
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "connect-src 'self'") {
		t.Errorf("preview content policy with HUNKYARD_BLOCK_EXTERNAL=1 = %q, want the strict policy", csp)
	}
}
