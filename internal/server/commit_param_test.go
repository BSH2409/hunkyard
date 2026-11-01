package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCommitParamIsValidatedAtTheBoundary(t *testing.T) {
	s, _ := newTestServer(t)
	tests := []struct {
		commit string
		want   int
	}{
		{"", http.StatusOK},
		{"__hunkyard_virtual_working_tree__", http.StatusOK},
		{"a1b2c3d", http.StatusOK},
		{"a1b2c3d..e4f5a6b", http.StatusOK},
		{"--output=/tmp/x", http.StatusBadRequest},
		{"-p", http.StatusBadRequest},
		{"HEAD~1", http.StatusBadRequest},
		{"a1b2c3d..--output=/tmp/x", http.StatusBadRequest},
	}
	for _, tt := range tests {
		for _, path := range []string{"/api/session?scope=all&commit=", "/api/file/diff?path=test.md&commit="} {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+url.QueryEscape(tt.commit), nil))
			if (rec.Code == http.StatusBadRequest) != (tt.want == http.StatusBadRequest) {
				t.Errorf("%s%q = %d, want %d", path, tt.commit, rec.Code, tt.want)
			}
		}
	}
}
