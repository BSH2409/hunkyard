package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServesCaptureScripts(t *testing.T) {
	s, _ := newTestServer(t)
	for _, name := range []string{"snapdom.min.js", "agent-pin-marker.js", "agent-capture.js"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+name, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
			t.Fatalf("/%s = %d %q", name, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/snapdom.min.js", nil))
	if !strings.Contains(rec.Body.String(), "window.snapdom") {
		t.Fatal("snapdom bundle does not define window.snapdom")
	}
}
