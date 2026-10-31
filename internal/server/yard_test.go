package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

func yardServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LEARN_PREFERENCES_HOME", root)
	srv, err := NewServer(nil, FrontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return srv, root
}

func seedYardRule(t *testing.T, root string) string {
	t.Helper()
	store, err := preferences.Open(preferences.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Capture(preferences.CaptureInput{Feedback: "multi-line defs", Directive: "Write Ruby method definitions over multiple lines.", Scope: "global", Surfaces: []string{"ruby"}, Explicit: true, Source: "cli", SessionID: "seed", TurnID: "seed"})
	if err != nil {
		t.Fatal(err)
	}
	return result.RuleID
}

func yardRequest(srv *Server, method, path, site string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if site != "" {
		req.Header.Set("Sec-Fetch-Site", site)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestYardEndpointsWorkWithoutSession(t *testing.T) {
	srv, root := yardServer(t)
	seedYardRule(t, root)
	for _, path := range []string{"/api/yard/rules", "/api/yard/persona", "/api/yard/pipeline", "/api/yard/activity?limit=5", "/api/yard/health", "/api/yard/loads"} {
		w := yardRequest(srv, http.MethodGet, path, "")
		if w.Code != http.StatusOK || !json.Valid(w.Body.Bytes()) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := yardRequest(srv, http.MethodGet, "/api/session", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("session endpoint must stay gated: %d", w.Code)
	}
}

func TestYardMutationsRequireSameOriginBrowser(t *testing.T) {
	srv, root := yardServer(t)
	id := seedYardRule(t, root)
	if w := yardRequest(srv, http.MethodPost, "/api/yard/rules/"+id+"/retire", ""); w.Code != http.StatusForbidden {
		t.Fatalf("retire without a browser origin: %d", w.Code)
	}
	if w := yardRequest(srv, http.MethodPost, "/api/yard/rules/"+id+"/retire", "same-origin"); w.Code != http.StatusOK {
		t.Fatalf("retire from the page: %d %s", w.Code, w.Body.String())
	}
	w := yardRequest(srv, http.MethodGet, "/api/yard/rules", "")
	if strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("rule still active after retire: %s", w.Body.String())
	}
}
