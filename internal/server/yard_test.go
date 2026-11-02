package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

func yardServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HUNKYARD_PREFERENCES_HOME", root)
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
	if !preferences.SupportedPlatform {
		t.Skip(preferences.ErrUnsupportedPlatform.Error())
	}
	srv, root := yardServer(t)
	seedYardRule(t, root)
	for _, path := range []string{"/api/preferences/rules", "/api/preferences/persona", "/api/preferences/pipeline", "/api/preferences/activity?limit=5", "/api/preferences/health", "/api/preferences/loads"} {
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
	if !preferences.SupportedPlatform {
		t.Skip(preferences.ErrUnsupportedPlatform.Error())
	}
	srv, root := yardServer(t)
	id := seedYardRule(t, root)
	if w := yardRequest(srv, http.MethodGet, "/api/preferences/rules", ""); !strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("precondition: the seeded rule should be active: %s", w.Body.String())
	}
	if w := yardRequest(srv, http.MethodPost, "/api/preferences/rules/"+id+"/retire", ""); w.Code != http.StatusForbidden {
		t.Fatalf("retire without a browser origin: %d", w.Code)
	}
	if w := yardRequest(srv, http.MethodPost, "/api/preferences/rules/"+id+"/retire", "same-origin"); w.Code != http.StatusOK {
		t.Fatalf("retire from the page: %d %s", w.Code, w.Body.String())
	}
	w := yardRequest(srv, http.MethodGet, "/api/preferences/rules", "")
	if strings.Contains(w.Body.String(), `"status":"active"`) {
		t.Fatalf("rule still active after retire: %s", w.Body.String())
	}
}

func TestYardUnsupportedPlatformAnswers501(t *testing.T) {
	previous := preferences.SupportedPlatform
	preferences.SupportedPlatform = false
	t.Cleanup(func() { preferences.SupportedPlatform = previous })
	s, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/preferences/rules", nil))
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "Windows") {
		t.Fatalf("unsupported platform: %d %s", rec.Code, rec.Body)
	}
}

func TestOldYardAPIPathIsGone(t *testing.T) {
	if !preferences.SupportedPlatform {
		t.Skip(preferences.ErrUnsupportedPlatform.Error())
	}
	srv, root := yardServer(t)
	seedYardRule(t, root)
	// Control: the rule is served under the new path.
	if w := yardRequest(srv, http.MethodGet, "/api/preferences/rules", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "multiple lines") {
		t.Fatalf("/api/preferences/rules = %d %s", w.Code, w.Body)
	}
	w := yardRequest(srv, http.MethodGet, "/api/yard/rules", "")
	if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "multiple lines") {
		t.Fatalf("/api/yard/rules = %d %s, want 404 without the rules", w.Code, w.Body)
	}
}

// "This repository" in the Preferences tab means the repository under review.
func TestYardRepositoryRuleUsesTheReviewedRepository(t *testing.T) {
	if !preferences.SupportedPlatform {
		t.Skip(preferences.ErrUnsupportedPlatform.Error())
	}
	t.Setenv("HUNKYARD_PREFERENCES_HOME", t.TempDir())
	s, sess := newTestServer(t)
	if out, err := exec.Command("git", "-C", sess.RepoRoot, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/preferences/rules", strings.NewReader(`{"directive":"Keep fixtures small.","scope":"repository","surfaces":["testing"],"feedback":"Keep fixtures small."}`))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add repository rule = %d %s", w.Code, w.Body)
	}
	rules := httptest.NewRecorder()
	s.ServeHTTP(rules, httptest.NewRequest(http.MethodGet, "/api/preferences/rules", nil))
	if !strings.Contains(rules.Body.String(), `"scope_kind":"repository"`) || !strings.Contains(rules.Body.String(), filepath.Base(sess.RepoRoot)) {
		t.Errorf("repository rule not keyed to the reviewed repository: %s", rules.Body)
	}
}

func TestYardInsights(t *testing.T) {
	if !preferences.SupportedPlatform {
		t.Skip(preferences.ErrUnsupportedPlatform.Error())
	}
	srv, _ := yardServer(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cases := []struct {
		name, method, path string
		want               int
	}{
		{"default window", http.MethodGet, "/api/preferences/insights", http.StatusOK},
		{"all time", http.MethodGet, "/api/preferences/insights?days=0", http.StatusOK},
		{"bad days", http.MethodGet, "/api/preferences/insights?days=-3", http.StatusBadRequest},
		{"not a number", http.MethodGet, "/api/preferences/insights?days=week", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := yardRequest(srv, tc.method, tc.path, "same-origin")
			if rec.Code != tc.want {
				t.Fatalf("%s = %d %s", tc.name, rec.Code, rec.Body)
			}
			if tc.want == http.StatusOK && !strings.Contains(rec.Body.String(), `"window_days"`) {
				t.Fatalf("body = %s", rec.Body)
			}
		})
	}
	if rec := yardRequest(srv, http.MethodPost, "/api/preferences/insights", "same-origin"); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST insights = %d", rec.Code)
	}
}
